package yq_provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/jsonrpc2"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
	"golang.org/x/mod/semver"
	"gopkg.in/yaml.v2"
)

type yqServiceClient struct {
	rpc        *jsonrpc2.Conn
	cancelFunc context.CancelFunc
	log        logr.Logger
	cmd        *exec.Cmd

	config       provider.InitConfig
	capabilities protocol.ServerCapabilities
}

type Release struct {
	Name string `json:"name"`
}

const APIVERSION = "apiVersion"
const KIND = "kind"
const GITHUB_K8S_API_URL = "https://api.github.com/repos/kubernetes/kubernetes/releases"

var default_k8s_version = "v1.28.4"

var _ provider.ServiceClient = &yqServiceClient{}

func (p *yqServiceClient) Stop() {
	p.cancelFunc()
	p.cmd.Wait()
}

func (p *yqServiceClient) Evaluate(ctx context.Context, cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
	var cond yqCondition
	err := yaml.Unmarshal(conditionInfo, &cond)
	if err != nil {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get query info")
	}
	apiVersion := cond.K8sResourceMatched.ApiVersion
	if apiVersion == "" {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get the apiVersion of k8s yaml to be matched")
	}
	kind := cond.K8sResourceMatched.Kind
	if kind == "" {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get the kind of k8s yaml to be matched")
	}
	deprecatedIn := cond.K8sResourceMatched.DeprecatedIn
	removedIn := cond.K8sResourceMatched.RemovedIn
	if deprecatedIn == "" && removedIn == "" {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get the kubernetes version in which apiVersion: %v is depreacated or removed for resource: %v", apiVersion, kind)
	}

	query := []string{APIVERSION, KIND}

	values, err := p.GetAllValuesForKey(ctx, query)
	if err != nil {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("can't find any value for query: %v, error=%v", query, err)
	}

	incidents := []provider.IncidentContext{}
	incidentsMap := make(map[string]provider.IncidentContext) // To remove duplicates

	for _, v := range values {
		targetVersion, err := p.getLatestStableKubernetesVersion()
		if err != nil {
			targetVersion = default_k8s_version
			p.log.V(5).Error(err, fmt.Sprintf("Cant get the latest k8s stable version, using the default version %s", default_k8s_version))
		}

		deprecatedComparison := p.isDeprecatedIn(targetVersion, deprecatedIn)
		removedInComparison := p.isRemovedIn(targetVersion, cond.K8sResourceMatched.RemovedIn)

		if v.ApiVersion.Value == apiVersion && v.Kind.Value == kind && (deprecatedComparison || removedInComparison) {
			u, err := uri.Parse(v.URI)
			if err != nil {
				return provider.ProviderEvaluateResponse{}, err
			}
			lineNumber, _ := strconv.Atoi(v.ApiVersion.LineNumber)
			incident := provider.IncidentContext{
				FileURI:    u,
				LineNumber: &lineNumber,
				Variables: map[string]interface{}{
					"apiVersion":      v.ApiVersion.Value,
					"kind":            v.Kind.Value,
					"deprecated-in":   deprecatedIn,
					"removed-in":      removedIn,
					"replacement-API": cond.K8sResourceMatched.ReplacementAPI,
				},
			}

			b, _ := json.Marshal(incident)

			incidentsMap[string(b)] = incident
		}
	}

	for _, incident := range incidentsMap {
		incidents = append(incidents, incident)
	}

	if len(incidents) == 0 {
		// No results were found.
		return provider.ProviderEvaluateResponse{Matched: false}, nil
	}
	return provider.ProviderEvaluateResponse{
		Matched:   true,
		Incidents: incidents,
	}, nil
}

func (p *yqServiceClient) GetAllValuesForKey(ctx context.Context, query []string) ([]k8sOutput, error) {
	var results []k8sOutput
	var wg sync.WaitGroup
	var mu sync.Mutex

	matchingYAMLFiles, err := provider.FindFilesMatchingPattern(p.config.Location, "*.yaml")
	if err != nil {
		fmt.Printf("unable to find any YAML files: %v\n", err)
	}
	matchingYMLFiles, err := provider.FindFilesMatchingPattern(p.config.Location, "*.yml")
	if err != nil {
		fmt.Printf("unable to find any YML files: %v\n", err)
	}
	matchingYAMLFiles = append(matchingYAMLFiles, matchingYMLFiles...)

	for _, file := range matchingYAMLFiles {
		wg.Add(1)
		go func(file string) {
			defer wg.Done()

			fmt.Printf("Reading YAML file: %s\n", file)

			data, err := os.ReadFile(file)
			if err != nil {
				fmt.Printf("Error reading YAML file '%s': %v\n", file, err)
				return
			}

			cmd := p.ConstructYQCommand(query)
			result, err := p.ExecuteCmd(cmd, string(data))
			if err != nil {
				p.log.V(5).Error(err, "Error running 'yq' command")
				return
			}

			mu.Lock()
			defer mu.Unlock()

			for _, output := range result {
				var currentResult k8sOutput

				result := strings.Split(strings.TrimSpace(output), "\n")
				currentResult.ApiVersion = k8skey{
					Value:      result[0],
					LineNumber: result[1],
				}

				currentResult.Kind = k8skey{
					Value:      result[2],
					LineNumber: result[3],
				}
				absPath, err := filepath.Abs(file)
				if err != nil {
					p.log.V(5).Error(err, "error getting abs path of yaml file")
				}
				fileURL := url.URL{
					Scheme: "file",
					Path:   absPath,
				}

				fileURI := fileURL.String()
				currentResult.URI = fileURI

				results = append(results, currentResult)
			}
		}(file)
	}

	wg.Wait()
	return results, nil
}

func (p *yqServiceClient) ExecuteCmd(cmd *exec.Cmd, input string) ([]string, error) {
	cmd.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("error running command= %s, error= %s, stdError= %v", cmd, err, stderr)
	}

	output := strings.Split(stdout.String(), "---")
	return output, nil
}

func (p *yqServiceClient) ConstructYQCommand(query []string) *exec.Cmd {

	yqCmd := &exec.Cmd{
		Path:   p.cmd.Path,
		Args:   append([]string(nil), p.cmd.Args...),
		Env:    append([]string(nil), p.cmd.Env...),
		Stdin:  p.cmd.Stdin,
		Stdout: p.cmd.Stdout,
		Stderr: p.cmd.Stderr,
	}

	var queryString string
	for _, q := range query {
		queryString += fmt.Sprintf(".%s, .%s | line,", q, q)
	}

	queryString = strings.TrimSuffix(queryString, ",")

	yqCmd.Args = append(yqCmd.Args, queryString)

	return yqCmd
}

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
}

func (p *yqServiceClient) getLatestStableKubernetesVersion() (string, error) {
	resp, err := httpClient.Get(GITHUB_K8S_API_URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return "", err
	}

	// Extract and filter version numbers
	var versions []string
	for _, release := range releases {
		version := strings.TrimPrefix(release.Name, "v")
		if !strings.Contains(version, "-") { // Exclude pre-releases
			versions = append(versions, version)
		}
	}

	// Sort versions in descending order
	sort.Sort(sort.Reverse(sort.StringSlice(versions)))

	if len(versions) > 0 {
		return strings.TrimSpace(strings.TrimPrefix(versions[0], "Kubernetes")), nil
	}

	return "", fmt.Errorf("no stable Kubernetes versions found")
}

func (p *yqServiceClient) isDeprecatedIn(targetVersion string, deprecatedIn string) bool {
	if !semver.IsValid(targetVersion) {
		p.log.Info(fmt.Sprintf("targetVersion %s is not valid semVer", targetVersion))
		return false
	}

	if deprecatedIn == "" {
		return false
	}

	if !semver.IsValid(deprecatedIn) {
		p.log.Info(fmt.Sprintf("deprecated version %s is not valid semVer", deprecatedIn))
		return false
	}

	comparison := semver.Compare(targetVersion, deprecatedIn)
	return comparison >= 0
}

func (p *yqServiceClient) isRemovedIn(targetVersion string, removedIn string) bool {
	if !semver.IsValid(targetVersion) {
		p.log.Info(fmt.Sprintf("targetVersion %s is not valid semVer", targetVersion))
		return false
	}

	if removedIn == "" {
		return false
	}

	if !semver.IsValid(removedIn) {
		p.log.Info(fmt.Sprintf("removed version %s is not valid semVer", removedIn))
		return false
	}

	comparison := semver.Compare(targetVersion, removedIn)
	return comparison >= 0
}

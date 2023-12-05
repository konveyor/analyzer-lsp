package generic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/konveyor/analyzer-lsp/jsonrpc2"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
	"golang.org/x/mod/semver"
	"gopkg.in/yaml.v2"
)

type genericServiceClient struct {
	rpc        *jsonrpc2.Conn
	cancelFunc context.CancelFunc
	cmd        *exec.Cmd

	config       provider.InitConfig
	capabilities protocol.ServerCapabilities
}

type Release struct {
	Name string `json:"name"`
}

const APIVERSION = "apiVersion"
const KIND = "kind"
const GithubK8sAPIURL = "https://api.github.com/repos/kubernetes/kubernetes/releases"

var _ provider.ServiceClient = &genericServiceClient{}

func (p *genericServiceClient) Stop() {
	p.cancelFunc()
	p.cmd.Wait()
}

func (p *genericServiceClient) Evaluate(ctx context.Context, cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
	var cond genericCondition
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
	if deprecatedIn == "" {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get the kubernetes version in which apiVersion: %v is depreacated for resource: %v", apiVersion, kind)
	}

	query := []string{APIVERSION, KIND}

	values := p.GetAllValuesForKey(ctx, query)

	incidents := []provider.IncidentContext{}
	incidentsMap := make(map[string]provider.IncidentContext) // To remove duplicates

	for _, v := range values {

		targetVersion, err := getLatestStableKubernetesVersion()
		if err != nil {
			return provider.ProviderEvaluateResponse{}, err
		}

		comparison := isDeprecatedIn(targetVersion, deprecatedIn)
		removedInComparison := isRemovedIn(targetVersion, cond.K8sResourceMatched.RemovedIn)

		if v.ApiVersion.Value == apiVersion && v.Kind.Value == kind && comparison {
			u, err := uri.Parse(v.URI)
			if err != nil {
				return provider.ProviderEvaluateResponse{}, err
			}
			lineNumber, _ := strconv.Atoi(v.ApiVersion.LineNumber)
			incident := provider.IncidentContext{
				FileURI:    u,
				LineNumber: &lineNumber,
				Variables: map[string]interface{}{
					"file":         v.URI,
					"apiVersion":   v.ApiVersion,
					"kind":         v.Kind,
					"deprecatedIn": deprecatedIn,
				},
			}
			if removedInComparison {
				incident.Variables["removedIn"] = cond.K8sResourceMatched.RemovedIn
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

func (p *genericServiceClient) GetAllValuesForKey(ctx context.Context, query []string) []k8sOutput {
	var results []k8sOutput

	matchingYAMLFiles, err := provider.FindFilesMatchingPattern(p.config.Location, "*.yaml")
	if err != nil {
		fmt.Printf("unable to find any YAML files: %v\n", err)
	}
	matchingYMLFiles, err := provider.FindFilesMatchingPattern(p.config.Location, "*.yml")
	if err != nil {
		fmt.Printf("unable to find any YML files: %v\n", err)
	}
	matchingYAMLFiles = append(matchingYAMLFiles, matchingYMLFiles...)

	resultCh := make(chan k8sOutput, len(matchingYAMLFiles))
	var wg sync.WaitGroup

	for _, file := range matchingYAMLFiles {
		wg.Add(1)
		go func(file string) {
			defer wg.Done()

			select {
			case <-ctx.Done():
				return
			default:
				log.Printf("Reading YAML file: %s\n", file)

				data, err := os.ReadFile(file)
				if err != nil {
					log.Printf("Error reading YAML file '%s': %v\n", file, err)
					return
				}

				cmd := p.ConstructYQCommand(query)
				result, err := ExecuteCmd(cmd, string(data))
				if err != nil {
					log.Printf("Error running 'yq' for file '%s': %v\n", file, err)
					return
				}

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
					currentResult.URI = fmt.Sprintf("file://%v", file)

					resultCh <- currentResult
				}
			}
		}(file)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	for result := range resultCh {
		results = append(results, result)
	}

	return results
}

func ExecuteCmd(cmd *exec.Cmd, input string) ([]string, error) {
	cmd.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("error running command: %v", stderr.String())
	}

	output := strings.Split(stdout.String(), "---")
	return output, nil
}

func (p *genericServiceClient) ConstructYQCommand(query []string) *exec.Cmd {
	yqCmd := p.cmd

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

func getLatestStableKubernetesVersion() (string, error) {
	resp, err := httpClient.Get(GithubK8sAPIURL)
	if err != nil {
		log.Printf("Error making HTTP request: %v\n", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("HTTP request failed with status code: %d\n", resp.StatusCode)
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		log.Printf("Error decoding JSON response: %v\n", err)
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

func isDeprecatedIn(targetVersion string, deprecatedIn string) bool {
	if !semver.IsValid(targetVersion) {
		log.Printf("targetVersion %s is not valid semVer", targetVersion)
		return false
	}

	if deprecatedIn == "" {
		return false
	}

	if !semver.IsValid(deprecatedIn) {
		log.Printf("deprecated version %s is not valid semVer", deprecatedIn)
		return false
	}

	comparison := semver.Compare(targetVersion, deprecatedIn)
	return comparison >= 0
}

func isRemovedIn(targetVersion string, removedIn string) bool {
	if !semver.IsValid(targetVersion) {
		log.Printf("targetVersion %s is not valid semVer", targetVersion)
		return false
	}

	if removedIn == "" {
		return false
	}

	if !semver.IsValid(removedIn) {
		log.Printf("removed version %s is not valid semVer", removedIn)
		return false
	}

	comparison := semver.Compare(targetVersion, removedIn)
	return comparison >= 0
}

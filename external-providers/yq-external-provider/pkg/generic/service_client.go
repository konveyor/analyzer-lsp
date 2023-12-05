package generic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
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

type genericServiceClient struct {
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

	values, err := p.GetAllValuesForKey2(ctx, query)
	if err != nil {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("can't find any value for query: %v, error=%v", query, err)
	}
	p.log.Info("VALUES FORM GetAllValuesForKey", values)

	incidents := []provider.IncidentContext{}
	incidentsMap := make(map[string]provider.IncidentContext) // To remove duplicates

	for _, v := range values {
		p.log.Info("INSIDE THE FOR LOOP OF VALUES")
		targetVersion, err := p.getLatestStableKubernetesVersion()
		if err != nil {
			return provider.ProviderEvaluateResponse{}, err
		}
		p.log.Info("STABLE KUBERNETES VERSION", targetVersion)

		comparison := p.isDeprecatedIn(targetVersion, deprecatedIn)
		removedInComparison := p.isRemovedIn(targetVersion, cond.K8sResourceMatched.RemovedIn)

		if v.ApiVersion.Value == apiVersion && v.Kind.Value == kind && comparison {
			p.log.Info("GOT A MATCHING KEY VALUE", v.ApiVersion.Value, v.Kind.Value)
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
					"apiVersion":   v.ApiVersion.Value,
					"kind":         v.Kind.Value,
					"deprecatedIn": deprecatedIn,
				},
			}
			if removedInComparison {
				incident.Variables["removedIn"] = cond.K8sResourceMatched.RemovedIn
			}

			b, _ := json.Marshal(incident)
			p.log.Info("INCIDENT", incident, b)

			incidentsMap[string(b)] = incident
		}
	}

	for _, incident := range incidentsMap {
		p.log.Info("INSIIDE THE incidentsMap FOR LOOP", incident)
		incidents = append(incidents, incident)
	}

	if len(incidents) == 0 {
		// No results were found.
		p.log.Info("DONT GET ANY INCIDENT")
		return provider.ProviderEvaluateResponse{Matched: false}, nil
	}
	return provider.ProviderEvaluateResponse{
		Matched:   true,
		Incidents: incidents,
	}, nil
}

func (p *genericServiceClient) GetAllValuesForKey1(ctx context.Context, query []string) ([]k8sOutput, error) {
	var results []k8sOutput

	matchingYAMLFiles, xerr := provider.FindFilesMatchingPattern(p.config.Location, "*.yaml")
	if xerr != nil {
		fmt.Printf("unable to find any YAML files: %v\n", xerr)
	}
	matchingYMLFiles, yerr := provider.FindFilesMatchingPattern(p.config.Location, "*.yml")
	if yerr != nil {
		fmt.Printf("unable to find any YML files: %v\n", yerr)
	}
	matchingYAMLFiles = append(matchingYAMLFiles, matchingYMLFiles...)
	if len(matchingYAMLFiles) == 0 {
		return []k8sOutput{}, fmt.Errorf("unable to find any YAML/YML files: %v, %v", xerr, yerr)
	}

	resultCh := make(chan k8sOutput)
	errCh := make(chan error)
	var wg sync.WaitGroup
	fmt.Printf("LENGTH OF matchingYAMLFiles- %v\n", len(matchingYAMLFiles))
	for _, file := range matchingYAMLFiles {
		p.log.Info("INSIDE THE matchingYAMLFiles FOR LOOP", file)
		wg.Add(1)
		go func(file string) {
			defer wg.Done()

			select {
			case <-ctx.Done():
				p.log.Info("CTX DONE 1")
				return
			default:
				fmt.Printf("Reading YAML file: %s\n", file)

				data, err := os.ReadFile(file)
				if err != nil {
					p.log.Info("ERROR READING FILE", file)
					fmt.Printf("Error reading YAML file '%s': %v\n", file, err)
					errCh <- err
					return
				}

				cmd := p.ConstructYQCommand(query)
				p.log.Info("COMMAND", cmd)
				result, err := p.ExecuteCmd(cmd, string(data))
				if err != nil {
					p.log.Info("ERROR EXECUTING COMMAND ", err)
					fmt.Printf("Error running 'yq' for file '%s': %v\n", file, err)
					// errCh <- fmt.Errorf("YQ command: s'''%v'''s ", p.cmd)
					errCh <- err
					return
				}

				fmt.Printf("LENGTH OF RESULT- %v\n", len(result))
				var count = 0
				for _, output := range result {
					count += 1
					fmt.Printf("INSIDE THE OUTPUT FOR LOOP- %v\n", output)
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

					p.log.Info("CURRENT RESULT", currentResult)
					fmt.Printf("CURRENTRESULT- %v\n", currentResult)

					fmt.Printf("RESULT COUNT= %v\n", count)

					// if resultCh
					resultCh <- currentResult
					fmt.Printf("RESULT COUNT1= %v\n", count)

				}
			}
		}(file)
	}

	go func() {
		p.log.Info("WAITING FOR FINISHING ALL GO ROUTINE")
		wg.Wait()
		close(resultCh)
		close(errCh)
		p.log.Info("EXEIT THE FO ROUTINE")
	}()

	for err := range errCh {
		p.log.Info("INSIDE THE ERRCH FOR LOOP")
		if err != nil {
			p.log.Info("ERROR FORM ERRORCHANNEL", err)
			return nil, err
		}
	}

	for result := range resultCh {
		p.log.Info("INSIDE THE RESULTCH FOR LOOP")
		results = append(results, result)
	}

	p.log.Info("ALL RESULTS", results)
	return results, nil
}

func (p *genericServiceClient) GetAllValuesForKey(ctx context.Context, query []string) ([]k8sOutput, error) {
	var results []k8sOutput

	matchingYAMLFiles, xerr := provider.FindFilesMatchingPattern(p.config.Location, "*.yaml")
	if xerr != nil {
		fmt.Printf("unable to find any YAML files: %v\n", xerr)
	}
	matchingYMLFiles, yerr := provider.FindFilesMatchingPattern(p.config.Location, "*.yml")
	if yerr != nil {
		fmt.Printf("unable to find any YML files: %v\n", yerr)
	}
	matchingYAMLFiles = append(matchingYAMLFiles, matchingYMLFiles...)
	if len(matchingYAMLFiles) == 0 {
		return []k8sOutput{}, fmt.Errorf("unable to find any YAML/YML files: %v, %v", xerr, yerr)
	}

	for _, file := range matchingYAMLFiles {
		fmt.Printf("Reading YAML file: %s\n", file)

		data, err := os.ReadFile(file)
		if err != nil {
			fmt.Printf("Error reading YAML file '%s': %v\n", file, err)
			return nil, err
		}

		cmd := p.ConstructYQCommand(query)
		result, err := p.ExecuteCmd(cmd, string(data))
		if err != nil {
			fmt.Printf("Error running 'yq' for file '%s': %v\n", file, err)
			return nil, err
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

			results = append(results, currentResult)
		}
	}

	return results, nil
}

func (p *genericServiceClient) GetAllValuesForKey2(ctx context.Context, query []string) ([]k8sOutput, error) {
	var results []k8sOutput
	var wg sync.WaitGroup
	var mu sync.Mutex

	matchingYAMLFiles, xerr := provider.FindFilesMatchingPattern(p.config.Location, "*.yaml")
	if xerr != nil {
		fmt.Printf("unable to find any YAML files: %v\n", xerr)
	}
	matchingYMLFiles, yerr := provider.FindFilesMatchingPattern(p.config.Location, "*.yml")
	if yerr != nil {
		fmt.Printf("unable to find any YML files: %v\n", yerr)
	}
	matchingYAMLFiles = append(matchingYAMLFiles, matchingYMLFiles...)
	if len(matchingYAMLFiles) == 0 {
		return []k8sOutput{}, fmt.Errorf("unable to find any YAML/YML files: %v, %v", xerr, yerr)
	}

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
				fmt.Printf("Error running 'yq' for file '%s': %v\n", file, err)
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
				currentResult.URI = fmt.Sprintf("file://%v", file)

				results = append(results, currentResult)
			}
		}(file)
	}

	wg.Wait()
	return results, nil
}

func (p *genericServiceClient) ExecuteCmd(cmd *exec.Cmd, input string) ([]string, error) {
	cmd.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("error running command= %s, error= %s, stdError= %s", cmd, err, &stderr)
	}

	output := strings.Split(stdout.String(), "---")
	fmt.Printf("OUTPUT= %v", output)
	return output, nil
}

func (p *genericServiceClient) ConstructYQCommand(query []string) *exec.Cmd {
	yqCmd := *p.cmd
	// yqCmd := exec.Command("/usr/bin/yq")

	var queryString string
	for _, q := range query {
		queryString += fmt.Sprintf(".%s, .%s | line,", q, q)
	}

	queryString = strings.TrimSuffix(queryString, ",")

	yqCmd.Args = append(yqCmd.Args, queryString)

	fmt.Printf("ConstructYQCommand- %v", yqCmd)
	return &yqCmd
}

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
}

func (p *genericServiceClient) getLatestStableKubernetesVersion() (string, error) {
	resp, err := httpClient.Get(GithubK8sAPIURL)
	if err != nil {
		fmt.Printf("Error making HTTP request: %v\n", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("HTTP request failed with status code: %d\n", resp.StatusCode)
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		fmt.Printf("Error decoding JSON response: %v\n", err)
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
		fmt.Printf("retutning the latest version= %v", versions)
		return strings.TrimSpace(strings.TrimPrefix(versions[0], "Kubernetes")), nil
	}

	return "", fmt.Errorf("no stable Kubernetes versions found")
}

func (p *genericServiceClient) isDeprecatedIn(targetVersion string, deprecatedIn string) bool {
	if !semver.IsValid(targetVersion) {
		fmt.Printf("targetVersion %s is not valid semVer", targetVersion)
		return false
	}

	if deprecatedIn == "" {
		return false
	}

	if !semver.IsValid(deprecatedIn) {
		fmt.Printf("deprecated version %s is not valid semVer", deprecatedIn)
		return false
	}

	comparison := semver.Compare(targetVersion, deprecatedIn)
	return comparison >= 0
}

func (p *genericServiceClient) isRemovedIn(targetVersion string, removedIn string) bool {
	if !semver.IsValid(targetVersion) {
		fmt.Printf("targetVersion %s is not valid semVer", targetVersion)
		return false
	}

	if removedIn == "" {
		return false
	}

	if !semver.IsValid(removedIn) {
		fmt.Printf("removed version %s is not valid semVer", removedIn)
		return false
	}

	comparison := semver.Compare(targetVersion, removedIn)
	return comparison >= 0
}

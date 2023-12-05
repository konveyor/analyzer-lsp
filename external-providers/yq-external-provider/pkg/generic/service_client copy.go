package generic

// import (
// 	"bytes"
// 	"context"
// 	"encoding/json"
// 	"fmt"
// 	"log"
// 	"net/http"
// 	"os"
// 	"os/exec"
// 	"sort"
// 	"strconv"
// 	"strings"
// 	"sync"
// 	"time"

// 	"github.com/go-logr/logr"
// 	"github.com/konveyor/analyzer-lsp/lsp/protocol"
// 	"github.com/konveyor/analyzer-lsp/provider"
// 	"go.lsp.dev/uri"
// 	"golang.org/x/mod/semver"
// 	"gopkg.in/yaml.v2"
// )

// type genericServiceClient struct {
// 	// rpc          *jsonrpc2.Conn
// 	cancelFunc   context.CancelFunc
// 	cmd          *exec.Cmd
// 	log          logr.Logger
// 	config       provider.InitConfig
// 	capabilities protocol.ServerCapabilities
// }

// type Release struct {
// 	Name string `json:"name"`
// }

// const (
// 	APIVERSION      = "apiVersion"
// 	KIND            = "kind"
// 	GithubK8sAPIURL = "https://api.github.com/repos/kubernetes/kubernetes/releases"
// )

// var _ provider.ServiceClient = &genericServiceClient{}

// func (p *genericServiceClient) Stop() {
// 	p.cancelFunc()
// 	p.cmd.Wait()
// }

// func (p *genericServiceClient) Evaluate(ctx context.Context, cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
// 	var cond genericCondition
// 	fmt.Print("Hello its under the evaluate function ")
// 	if err := yaml.Unmarshal(conditionInfo, &cond); err != nil {
// 		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get query info: %v", err)
// 	}

// 	apiVersion := cond.K8sResourceMatched.ApiVersion
// 	if apiVersion == "" {
// 		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get the apiVersion of k8s yaml to be matched")
// 	}
// 	kind := cond.K8sResourceMatched.Kind
// 	if kind == "" {
// 		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get the kind of k8s yaml to be matched")
// 	}
// 	deprecatedIn := cond.K8sResourceMatched.DeprecatedIn
// 	if deprecatedIn == "" {
// 		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get the kubernetes version in which apiVersion: %v is deprecated for resource: %v", apiVersion, kind)
// 	}

// 	query := []string{APIVERSION, KIND}

// 	values, err := p.GetAllValuesForKey(ctx, query)
// 	if err != nil {
// 		return provider.ProviderEvaluateResponse{}, fmt.Errorf("can't find any value for query: %v, error=%v", query, err)
// 	}

// 	incidents := make([]provider.IncidentContext, 0)
// 	incidentsMap := make(map[string]provider.IncidentContext) // To remove duplicates

// 	for _, v := range values {
// 		fmt.Print("IN THE FOR LOOP\n")
// 		select {
// 		case <-ctx.Done():
// 			fmt.Print("EVALUATE CONTEXT CANCELLED")
// 			return provider.ProviderEvaluateResponse{}, context.Canceled
// 		default:
// 			targetVersion, err := getLatestStableKubernetesVersionWithContext(ctx)
// 			if err != nil {
// 				return provider.ProviderEvaluateResponse{}, err
// 			}

// 			comparison := isDeprecatedIn(targetVersion, deprecatedIn)
// 			removedInComparison := isRemovedIn(targetVersion, cond.K8sResourceMatched.RemovedIn)

// 			if v.ApiVersion.Value == apiVersion && v.Kind.Value == kind && comparison {
// 				fmt.Printf("%v KEY GOT MATCHED", v.ApiVersion.Value)
// 				u, err := uri.Parse(v.URI)
// 				if err != nil {
// 					return provider.ProviderEvaluateResponse{}, err
// 				}
// 				lineNumber, _ := strconv.Atoi(v.ApiVersion.LineNumber)
// 				incident := provider.IncidentContext{
// 					FileURI:    u,
// 					LineNumber: &lineNumber,
// 					Variables: map[string]interface{}{
// 						"file":         v.URI,
// 						"apiVersion":   v.ApiVersion,
// 						"kind":         v.Kind,
// 						"deprecatedIn": deprecatedIn,
// 					},
// 				}
// 				if removedInComparison {
// 					incident.Variables["removedIn"] = cond.K8sResourceMatched.RemovedIn
// 				}

// 				b, err := json.Marshal(incident)
// 				if err != nil {
// 					fmt.Printf("Marshalling error= %v", err)
// 				}
// 				fmt.Printf("INCIDENT= %v", b)

// 				incidentsMap[string(b)] = incident
// 			}
// 		}
// 	}

// 	for _, incident := range incidentsMap {
// 		fmt.Printf("incident for loop= %v", incident)
// 		incidents = append(incidents, incident)
// 	}

// 	if len(incidents) == 0 {
// 		// No results were found.
// 		fmt.Print("NO INCIDENTS ARE FOUND\n")
// 		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get the incidents: %v is deprecated for resource: %v", apiVersion, kind)
// 	}

// 	fmt.Printf("RETURNING INCIDENTS= %v\n", incidents)

// 	return provider.ProviderEvaluateResponse{
// 		Matched:   true,
// 		Incidents: incidents,
// 	}, nil
// }

// func (p *genericServiceClient) GetAllValuesForKey(ctx context.Context, query []string) ([]k8sOutput, error) {
// 	var results []k8sOutput

// 	matchingYAMLFiles, err := provider.FindFilesMatchingPattern(p.config.Location, "*.yaml")
// 	if err != nil {
// 		return nil, fmt.Errorf("unable to find any YAML/YML files: %v", err)
// 	}

// 	resultCh := make(chan k8sOutput, len(matchingYAMLFiles))
// 	errCh := make(chan error, len(matchingYAMLFiles))
// 	var wg sync.WaitGroup

// 	for _, file := range matchingYAMLFiles {
// 		wg.Add(1)
// 		go func(file string) {
// 			defer wg.Done()

// 			select {
// 			case <-ctx.Done():
// 				return
// 			default:
// 				fmt.Printf("Reading YAML file: %s\n", file)

// 				data, err := os.ReadFile(file)
// 				if err != nil {
// 					fmt.Printf("Error reading YAML file '%s': %v\n", file, err)
// 					errCh <- err
// 					return
// 				}

// 				cmd := p.ConstructYQCommand(query)
// 				result, err := ExecuteCmdWithContext(ctx, cmd, string(data))
// 				if err != nil {
// 					fmt.Printf("Error running 'yq' for file '%s': %v\n", file, err)
// 					errCh <- err
// 					return
// 				}

// 				for _, output := range result {
// 					fmt.Print("IN THE FOR result LOOP")
// 					var currentResult k8sOutput

// 					result := strings.Split(strings.TrimSpace(output), "\n")
// 					currentResult.ApiVersion = k8skey{
// 						Value:      result[0],
// 						LineNumber: result[1],
// 					}

// 					currentResult.Kind = k8skey{
// 						Value:      result[2],
// 						LineNumber: result[3],
// 					}
// 					currentResult.URI = fmt.Sprintf("file://%v", file)

// 					fmt.Printf("CURRENT RESULT= %v", currentResult)
// 					resultCh <- currentResult
// 				}
// 			}
// 		}(file)
// 	}

// 	go func() {
// 		wg.Wait()
// 		close(resultCh)
// 		close(errCh)
// 	}()

// 	for {
// 		fmt.Printf("In the for loop of error channel and result channel %v\n", results)
// 		select {
// 		case <-ctx.Done():
// 			fmt.Print("GetAllValuesForKey context get cancelled")
// 			return nil, context.Canceled
// 		case err := <-errCh:
// 			if err != nil {
// 				return nil, err
// 			}
// 		case result, ok := <-resultCh:
// 			if !ok {
// 				return results, nil
// 			}
// 			results = append(results, result)
// 		}
// 	}

// }

// func ExecuteCmdWithContext(ctx context.Context, cmd *exec.Cmd, input string) ([]string, error) {
// 	var output []string

// 	cmd.Stdin = strings.NewReader(input)
// 	var stdout, stderr bytes.Buffer
// 	cmd.Stdout = &stdout
// 	cmd.Stderr = &stderr

// 	err := cmd.Run()
// 	if err != nil {
// 		return nil, fmt.Errorf("error running command: %v\n%s", err, stderr.String())
// 	}

// 	outLines := strings.Split(stdout.String(), "\n")
// 	for _, line := range outLines {
// 		if line != "" {
// 			output = append(output, line)
// 		}
// 	}

// 	fmt.Printf("ExecuteCmdWithContext OUTPUT= %v", output)

// 	return output, nil
// }

// func (p *genericServiceClient) ConstructYQCommand(query []string) *exec.Cmd {
// 	yqCmd := *p.cmd

// 	var queryString string
// 	for _, q := range query {
// 		queryString += fmt.Sprintf(".%s, .%s | line,", q, q)
// 	}

// 	queryString = strings.TrimSuffix(queryString, ",")
// 	yqCmd.Args = append(yqCmd.Args, queryString)

// 	return &yqCmd
// }

// var httpClient = &http.Client{
// 	Timeout: 10 * time.Second,
// }

// func getLatestStableKubernetesVersionWithContext(ctx context.Context) (string, error) {
// 	resp, err := httpClient.Get(GithubK8sAPIURL)
// 	if err != nil {
// 		fmt.Printf("Error making HTTP request: %v\n", err)
// 		return "", err
// 	}
// 	defer resp.Body.Close()

// 	select {
// 	case <-ctx.Done():
// 		return "", context.Canceled
// 	default:
// 		// Continue with execution
// 	}

// 	var releases []Release
// 	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
// 		log.Printf("Error decoding JSON response: %v\n", err)
// 		return "", err
// 	}

// 	// Extract and filter version numbers
// 	var versions []string
// 	for _, release := range releases {
// 		version := strings.TrimPrefix(release.Name, "v")
// 		if !strings.Contains(version, "-") { // Exclude pre-releases
// 			versions = append(versions, version)
// 		}
// 	}

// 	// Sort versions in descending order
// 	sort.Sort(sort.Reverse(sort.StringSlice(versions)))

// 	if len(versions) > 0 {
// 		fmt.Printf("GOT A VALID STABLE VERSION= %v", versions)
// 		return strings.TrimSpace(strings.TrimPrefix(versions[0], "Kubernetes")), nil
// 	}

// 	return "", fmt.Errorf("no stable Kubernetes versions found")
// }

// func isDeprecatedIn(targetVersion string, deprecatedIn string) bool {
// 	if !semver.IsValid(targetVersion) {
// 		fmt.Printf("targetVersion %s is not valid semVer", targetVersion)
// 		return false
// 	}

// 	if deprecatedIn == "" {
// 		return false
// 	}

// 	if !semver.IsValid(deprecatedIn) {
// 		fmt.Printf("deprecated version %s is not valid semVer", deprecatedIn)
// 		return false
// 	}

// 	comparison := semver.Compare(targetVersion, deprecatedIn)
// 	return comparison >= 0
// }

// func isRemovedIn(targetVersion string, removedIn string) bool {
// 	if !semver.IsValid(targetVersion) {
// 		fmt.Printf("targetVersion %s is not valid semVer", targetVersion)
// 		return false
// 	}

// 	if removedIn == "" {
// 		return false
// 	}

// 	if !semver.IsValid(removedIn) {
// 		fmt.Printf("removed version %s is not valid semVer", removedIn)
// 		return false
// 	}

// 	comparison := semver.Compare(targetVersion, removedIn)
// 	return comparison >= 0
// }

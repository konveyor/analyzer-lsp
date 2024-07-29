package java

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/jsonrpc2"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
)

type javaServiceClient struct {
	rpc               *jsonrpc2.Conn
	cancelFunc        context.CancelFunc
	config            provider.InitConfig
	log               logr.Logger
	cmd               *exec.Cmd
	bundles           []string
	workspace         string
	depToLabels       map[string]*depLabelItem
	isLocationBinary  bool
	mvnSettingsFile   string
	depsMutex         sync.RWMutex
	depsCache         map[uri.URI][]*provider.Dep
	depsLocationCache map[string]int
	includedPaths     []string
}

type depLabelItem struct {
	r      *regexp.Regexp
	labels map[string]interface{}
}

var _ provider.ServiceClient = &javaServiceClient{}

func (p *javaServiceClient) Evaluate(ctx context.Context, cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {

	cond := &javaCondition{}
	err := yaml.Unmarshal(conditionInfo, &cond)
	if err != nil {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get query info: %v", err)
	}

	if cond.Referenced.Pattern == "" {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("provided query pattern empty")
	}
	symbols, err := p.GetAllSymbols(ctx, cond.Referenced.Pattern, cond.Referenced.Location, cond.Referenced.Annotated)
	if err != nil {
		p.log.Error(err, "unable to get symbols", "symbols", symbols, "cap", cap, "conditionInfo", cond)
		return provider.ProviderEvaluateResponse{}, err
	}
	p.log.Info("Symbols retrieved", "symbols", len(symbols), "cap", cap, "conditionInfo", cond)

	incidents := []provider.IncidentContext{}
	switch locationToCode[strings.ToLower(cond.Referenced.Location)] {
	case 0:
		// Filter handle for type, find all the referneces to this type.
		incidents, err = p.filterDefault(symbols)
	case 1, 5:
		incidents, err = p.filterTypesInheritance(symbols)
	case 2:
		incidents, err = p.filterMethodSymbols(symbols)
	case 3:
		incidents, err = p.filterDefault(symbols)
	case 4:
		incidents, err = p.filterDefault(symbols)
	case 7:
		incidents, err = p.filterMethodSymbols(symbols)
	case 8:
		incidents, err = p.filterModulesImports(symbols)
	case 9:
		incidents, err = p.filterVariableDeclaration(symbols)
	case 10:
		incidents, err = p.filterDefault(symbols)
	case 11:
		incidents, err = p.filterDefault(symbols)
	case 12:
		incidents, err = p.filterDefault(symbols)
	case 13:
		incidents, err = p.filterDefault(symbols)
	default:

	}
	// push error up for easier printing.
	if err != nil {
		return provider.ProviderEvaluateResponse{}, err
	}

	if len(incidents) == 0 {
		return provider.ProviderEvaluateResponse{
			Matched: false,
		}, nil
	}
	return provider.ProviderEvaluateResponse{
		Matched:   true,
		Incidents: incidents,
	}, nil
}

func (p *javaServiceClient) GetAllSymbols(ctx context.Context, query, location string, annotation annotated) ([]protocol.WorkspaceSymbol, error) {
	// This command will run the added bundle to the language server. The command over the wire needs too look like this.
	// in this case the project is hardcoded in the init of the Langauge Server above
	// workspace/executeCommand '{"command": "io.konveyor.tackle.ruleEntry", "arguments": {"query":"*customresourcedefinition","project": "java"}}'
	argumentsMap := map[string]interface{}{
		"query":        query,
		"project":      "java",
		"location":     fmt.Sprintf("%v", locationToCode[strings.ToLower(location)]),
		"analysisMode": string(p.config.AnalysisMode),
	}

	if !reflect.DeepEqual(annotation, annotated{}) {
		argumentsMap["annotationQuery"] = annotation
	}

	if p.includedPaths != nil && len(p.includedPaths) > 0 {
		argumentsMap[provider.IncludedPathsConfigKey] = p.includedPaths
		p.log.V(5).Info("setting search scope by filepaths", "paths", p.includedPaths)
	}

	argumentsBytes, _ := json.Marshal(argumentsMap)
	arguments := []json.RawMessage{argumentsBytes}

	wsp := &protocol.ExecuteCommandParams{
		Command:   "io.konveyor.tackle.ruleEntry",
		Arguments: arguments,
	}

	var refs []protocol.WorkspaceSymbol
	// If it takes us 5min to complete a request, then we are in trouble
	timeOutCtx, _ := context.WithTimeout(ctx, 5*time.Minute)
	err := p.rpc.Call(timeOutCtx, "workspace/executeCommand", wsp, &refs)
	if err != nil {
		if jsonrpc2.IsRPCClosed(err) {
			p.log.Error(err, "connection to the language server is closed, language server is not running")
			return refs, fmt.Errorf("connection to the language server is closed, language server is not running")
		} else {
			p.log.Error(err, "unable to ask for Konveyor rule entry")
			return refs, fmt.Errorf("unable to ask for Konveyor rule entry")
		}
	}

	return refs, nil
}

func (p *javaServiceClient) GetAllReferences(ctx context.Context, symbol protocol.WorkspaceSymbol) []protocol.Location {
	var locationURI protocol.DocumentURI
	var locationRange protocol.Range
	switch x := symbol.Location.Value.(type) {
	case protocol.Location:
		locationURI = x.URI
		locationRange = x.Range
	case protocol.PLocationMsg_workspace_symbol:
		locationURI = x.URI
		locationRange = protocol.Range{}
	default:
		locationURI = ""
		locationRange = protocol.Range{}
	}

	if strings.Contains(locationURI, JDT_CLASS_FILE_URI_PREFIX) {
		return []protocol.Location{
			{
				URI:   locationURI,
				Range: locationRange,
			},
		}
	}
	params := &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: locationURI,
			},
			Position: locationRange.Start,
		},
	}

	res := []protocol.Location{}
	err := p.rpc.Call(ctx, "textDocument/references", params, &res)
	if err != nil {
		if jsonrpc2.IsRPCClosed(err) {
			p.log.Error(err, "connection to the language server is closed, language server is not running")
		} else {
			fmt.Printf("Error rpc: %v", err)
		}
	}
	return res
}

func (p *javaServiceClient) Stop() {
	p.cancelFunc()
	p.cmd.Wait()
}

func (p *javaServiceClient) initialization(ctx context.Context) {
	absLocation, err := filepath.Abs(p.config.Location)
	if err != nil {
		p.log.Error(err, "unable to get path to analyize")
		panic(1)
	}

	var absBundles []string
	for _, bundle := range p.bundles {
		abs, err := filepath.Abs(bundle)
		if err != nil {
			p.log.Error(err, "unable to get path to bundles")
			panic(1)
		}
		absBundles = append(absBundles, abs)

	}
	downloadSources := true
	if p.config.AnalysisMode == provider.SourceOnlyAnalysisMode {
		downloadSources = false
	}

	//TODO(shawn-hurley): add ability to parse path to URI in a real supported way
	params := &protocol.InitializeParams{}
	params.RootURI = fmt.Sprintf("file://%v", absLocation)
	params.Capabilities = protocol.ClientCapabilities{}
	params.ExtendedClientCapilities = map[string]interface{}{
		"classFileContentsSupport": true,
	}
	// See https://github.com/eclipse-jdtls/eclipse.jdt.ls/blob/1a3dd9323756113bf39cfab82746d57a2fd19474/org.eclipse.jdt.ls.core/src/org/eclipse/jdt/ls/core/internal/preferences/Preferences.java
	java8home := os.Getenv("JAVA8_HOME")
	params.InitializationOptions = map[string]interface{}{
		"bundles":          absBundles,
		"workspaceFolders": []string{fmt.Sprintf("file://%v", absLocation)},
		"settings": map[string]interface{}{
			"java": map[string]interface{}{
				"configuration": map[string]interface{}{
					"maven": map[string]interface{}{
						"userSettings": p.mvnSettingsFile,
					},
				},
				"autobuild": map[string]interface{}{
					"enabled": false,
				},
				"maven": map[string]interface{}{
					"downloadSources": downloadSources,
				},
				"import": map[string]interface{}{
					"gradle": map[string]interface{}{
						"java": map[string]interface{}{
							"home": java8home,
						},
					},
				},
			},
		},
	}

	var result protocol.InitializeResult
	for i := 0; i < 10; i++ {
		if err := p.rpc.Call(ctx, "initialize", params, &result); err != nil {
			if jsonrpc2.IsRPCClosed(err) {
				p.log.Error(err, "connection to the language server is closed, language server is not running")
			} else {
				p.log.Error(err, "initialize failed")
			}
			continue
		}
		break
	}
	if err := p.rpc.Notify(ctx, "initialized", &protocol.InitializedParams{}); err != nil {
		fmt.Printf("initialized failed: %v", err)
		p.log.Error(err, "initialize failed")
	}
	p.log.V(2).Info("java connection initialized")

}

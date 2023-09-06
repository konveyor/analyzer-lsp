package java

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/jsonrpc2"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
)

type javaServiceClient struct {
	rpc              *jsonrpc2.Conn
	ctx              context.Context
	cancelFunc       context.CancelFunc
	config           provider.InitConfig
	log              logr.Logger
	cmd              *exec.Cmd
	bundles          []string
	workspace        string
	depToLabels      map[string]*depLabelItem
	isLocationBinary bool
	mvnSettingsFile  string
	depsCache        map[uri.URI][]*provider.Dep
}

type depLabelItem struct {
	r      *regexp.Regexp
	labels map[string]interface{}
}

var _ provider.ServiceClient = &javaServiceClient{}

func (p *javaServiceClient) Evaluate(cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
	cond := &javaCondition{}
	err := yaml.Unmarshal(conditionInfo, &cond)
	if err != nil {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get query info: %v", err)
	}

	if cond.Referenced.Pattern == "" {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("provided query pattern empty")
	}

	symbols := p.GetAllSymbols(cond.Referenced.Pattern, cond.Referenced.Location)

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
		incidents, err = p.filterConstructorSymbols(symbols)
	case 4:
		incidents, err = p.filterDefault(symbols)
	case 7:
		incidents, err = p.filterMethodSymbols(symbols)
	case 8:
		incidents, err = p.filterModulesImports(symbols)
	case 9:
		incidents, err = p.filterVariableDeclaration(symbols)
	case 10:
		incidents, err = p.filterTypeReferences(symbols)
	case 11:
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

func (p *javaServiceClient) GetAllSymbols(query, location string) []protocol.WorkspaceSymbol {
	// This command will run the added bundle to the language server. The command over the wire needs too look like this.
	// in this case the project is hardcoded in the init of the Langauge Server above
	// workspace/executeCommand '{"command": "io.konveyor.tackle.ruleEntry", "arguments": {"query":"*customresourcedefinition","project": "java"}}'
	arguments := map[string]string{
		"query":        query,
		"project":      "java",
		"location":     fmt.Sprintf("%v", locationToCode[strings.ToLower(location)]),
		"analysisMode": string(p.config.AnalysisMode),
	}

	wsp := &protocol.ExecuteCommandParams{
		Command:   "io.konveyor.tackle.ruleEntry",
		Arguments: []interface{}{arguments},
	}

	var refs []protocol.WorkspaceSymbol
	err := p.rpc.Call(p.ctx, "workspace/executeCommand", wsp, &refs)
	if err != nil {
		p.log.Error(err, "unable to ask for tackle rule entry")
	}

	return refs
}

func (p *javaServiceClient) GetAllReferences(symbol protocol.WorkspaceSymbol) []protocol.Location {
	if strings.Contains(symbol.Location.URI, FILE_URI_PREFIX) {
		return []protocol.Location{
			{
				URI:   symbol.Location.URI,
				Range: symbol.Location.Range,
			},
		}
	}
	params := &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: symbol.Location.URI,
			},
			Position: symbol.Location.Range.Start,
		},
	}

	res := []protocol.Location{}
	err := p.rpc.Call(p.ctx, "textDocument/references", params, &res)
	if err != nil {
		fmt.Printf("Error rpc: %v", err)
	}
	return res
}

func (p *javaServiceClient) Stop() {
	p.cancelFunc()
	p.cmd.Wait()
}

func (p *javaServiceClient) initialization() {
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

	params := &protocol.InitializeParams{
		//TODO(shawn-hurley): add ability to parse path to URI in a real supported way
		RootURI:      fmt.Sprintf("file://%v", absLocation),
		Capabilities: protocol.ClientCapabilities{},
		ExtendedClientCapilities: map[string]interface{}{
			"classFileContentsSupport": true,
		},
		InitializationOptions: map[string]interface{}{
			"bundles":          absBundles,
			"workspaceFolders": []string{fmt.Sprintf("file://%v", absLocation)},
			"settings": map[string]interface{}{
				"java": map[string]interface{}{
					"configuration": map[string]interface{}{
						"maven": map[string]interface{}{
							"userSettings": p.mvnSettingsFile,
						},
					},
					"maven": map[string]interface{}{
						"downloadSources": downloadSources,
					},
				},
			},
		},
	}

	var result protocol.InitializeResult
	for i := 0; i < 10; i++ {
		if err := p.rpc.Call(p.ctx, "initialize", params, &result); err != nil {
			p.log.Error(err, "initialize failed")
			continue
		}
		break
	}
	if err := p.rpc.Notify(p.ctx, "initialized", &protocol.InitializedParams{}); err != nil {
		fmt.Printf("initialized failed: %v", err)
		p.log.Error(err, "initialize failed")
	}
	p.log.V(2).Info("java connection initialized")
}

package java

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/jsonrpc2"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider/lib"
	"gopkg.in/yaml.v2"
)

// Rule Location to location that the bundle understands
var locationToCode = map[string]int{
	//Type is the default.
	"type":             0,
	"inheritance":      1,
	"method_call":      2,
	"constructor_call": 3,
	"annotation":       4,
	"implements_type":  5,
	// Not Implemented
	"enum_constant": 6,
	"return_type":   7,
	"import":        8,
	"method":        9,
}

type javaProvider struct {
	config lib.Config

	bundles   []string
	workspace string

	rpc        *jsonrpc2.Conn
	ctx        context.Context
	cancelFunc context.CancelFunc
	cmd        *exec.Cmd
	once       sync.Once
}

type javaCondition struct {
	Referenced referenceCondition `yaml:'referenced'`
}

type referenceCondition struct {
	Pattern  string `yaml:"pattern"`
	Location string `yaml:"location"`
}

const BUNDLES_INIT_OPTION = "bundles"
const WORKSPACE_INIT_OPTION = "workspace"

func NewJavaProvider(config lib.Config) *javaProvider {

	// Get the provider config out for this config.

	// Getting values out of provider config
	// TODO: Eventually we will want to make this a helper so that external providers can easily ask and get config.
	bundlesString := config.ProviderSpecificConfig[BUNDLES_INIT_OPTION]
	bundles := strings.Split(bundlesString, ",")

	workspace := config.ProviderSpecificConfig[WORKSPACE_INIT_OPTION]

	return &javaProvider{
		config:    config,
		bundles:   bundles,
		workspace: workspace,
		once:      sync.Once{},
	}
}

func (p *javaProvider) Stop() {
	p.cancelFunc()
	// Ignore the error here, it stopped and we wanted it to.
	p.cmd.Wait()
}

func (p *javaProvider) Capabilities() ([]lib.Capability, error) {
	return []lib.Capability{
		{
			Name:            "referenced",
			TemplateContext: openapi3.SchemaRef{},
		},
	}, nil
}

func (p *javaProvider) Evaluate(cap string, conditionInfo []byte) (lib.ProviderEvaluateResponse, error) {
	cond := &javaCondition{}
	err := yaml.Unmarshal(conditionInfo, &cond)
	if err != nil {
		return lib.ProviderEvaluateResponse{}, fmt.Errorf("unable to get query info: %v", err)
	}

	if cond.Referenced.Pattern == "" {
		return lib.ProviderEvaluateResponse{}, fmt.Errorf("provided query pattern empty")
	}

	symbols := p.GetAllSymbols(cond.Referenced.Pattern, cond.Referenced.Location)

	incidents := []lib.IncidentContext{}
	switch locationToCode[strings.ToLower(cond.Referenced.Location)] {
	case 0:
		// Filter handle for type, find all the referneces to this type.
		incidents, err = p.filterTypeReferences(symbols)
	case 1, 5:
		incidents, err = p.filterTypesInheritance(symbols)
	case 2:
		incidents, err = p.filterMethodSymbols(symbols)
	case 3:
		incidents, err = p.filterConstructorSymbols(symbols)
	case 7:
		incidents, err = p.filterMethodSymbols(symbols)
	case 8:
		incidents, err = p.filterModulesImports(symbols)
	default:

	}

	// push error up for easier printing.
	if err != nil {
		return lib.ProviderEvaluateResponse{}, err
	}

	if len(incidents) == 0 {
		return lib.ProviderEvaluateResponse{
			Matched: false,
		}, nil
	}
	return lib.ProviderEvaluateResponse{
		Matched:   true,
		Incidents: incidents,
	}, nil
}

func symbolKindToString(symbolKind protocol.SymbolKind) string {
	switch symbolKind {
	case 1:
		return "File"
	case 2:
		return "Module"
	case 3:
		return "Namespace"
	case 4:
		return "Package"
	case 5:
		return "Class"
	case 6:
		return "Method"
	case 7:
		return "Property"
	case 8:
		return "Field"
	case 9:
		return "Constructor"
	case 10:
		return "Enum"
	case 11:
		return "Interface"
	case 12:
		return "Function"
	case 13:
		return "Variable"
	case 14:
		return "Constant"
	case 15:
		return "String"
	case 16:
		return "Number"
	case 17:
		return "Boolean"
	case 18:
		return "Array"
	case 19:
		return "Object"
	case 20:
		return "Key"
	case 21:
		return "Null"
	case 22:
		return "EnumMember"
	case 23:
		return "Struct"
	case 24:
		return "Event"
	case 25:
		return "Operator"
	case 26:
		return "TypeParameter"
	}
	return ""
}

func (p *javaProvider) Init(ctx context.Context, log logr.Logger) error {
	log = log.WithValues("provider", "java")

	var returnErr error
	ctx, cancelFunc := context.WithCancel(ctx)
	p.once.Do(func() {

		cmd := exec.CommandContext(ctx, p.config.BinaryLocation,
			"-configuration",
			"./",
			"-data",
			p.workspace,
		)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			returnErr = err
			return
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			returnErr = err
			return
		}

		p.cancelFunc = cancelFunc
		p.cmd = cmd
		go func() {
			err := cmd.Start()
			if err != nil {
				fmt.Printf("here cmd failed- %v", err)
			}
		}()
		rpc := jsonrpc2.NewConn(jsonrpc2.NewHeaderStream(stdout, stdin), log)

		go func() {
			err := rpc.Run(ctx)
			if err != nil {
				//TODO: we need to pipe the ctx further into the stream header and run.
				// basically it is checking if done, then reading. When it gets EOF it errors.
				// We need the read to be at the same level of selection to fully implment graceful shutdown
				return
			}
		}()

		p.rpc = rpc
		p.ctx = ctx
		p.initialization(ctx, log)
	})
	return returnErr
}

func (p *javaProvider) initialization(ctx context.Context, log logr.Logger) {

	absLocation, err := filepath.Abs(p.config.Location)
	if err != nil {
		log.Error(err, "unable to get path to analyize")
		panic(1)
	}

	var absBundles []string
	for _, bundle := range p.bundles {
		abs, err := filepath.Abs(bundle)
		if err != nil {
			log.Error(err, "unable to get path to bundles")
			panic(1)
		}
		absBundles = append(absBundles, abs)

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
					"maven": map[string]interface{}{
						"downloadSources": true,
					},
				},
			},
		},
	}

	var result protocol.InitializeResult
	for {
		if err := p.rpc.Call(ctx, "initialize", params, &result); err != nil {
			log.Error(err, "initialize failed")
			continue
		}
		break
	}
	if err := p.rpc.Notify(ctx, "initialized", &protocol.InitializedParams{}); err != nil {
		fmt.Printf("initialized failed: %v", err)
		log.Error(err, "initialize failed")
	}
	log.V(2).Info("java connection initialized")
}

func (p *javaProvider) GetAllSymbols(query, location string) []protocol.WorkspaceSymbol {
	// This command will run the added bundle to the language server. The command over the wire needs too look like this.
	// in this case the project is hardcoded in the init of the Langauge Server above
	// workspace/executeCommand '{"command": "io.konveyor.tackle.ruleEntry", "arguments": {"query":"*customresourcedefinition","project": "java"}}'
	arguments := map[string]string{
		"query":    query,
		"project":  "java",
		"location": fmt.Sprintf("%v", locationToCode[strings.ToLower(location)]),
	}

	wsp := &protocol.ExecuteCommandParams{
		Command:   "io.konveyor.tackle.ruleEntry",
		Arguments: []interface{}{arguments},
	}

	var refs []protocol.WorkspaceSymbol
	err := p.rpc.Call(p.ctx, "workspace/executeCommand", wsp, &refs)
	if err != nil {
		fmt.Printf("error: %v", err)
	}

	return refs
}

func (p *javaProvider) GetAllReferences(symbol protocol.WorkspaceSymbol) []protocol.Location {
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

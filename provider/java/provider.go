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
	Referenced string `yaml:'referenced'`
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
	var cond javaCondition
	err := yaml.Unmarshal(conditionInfo, &cond)
	if err != nil {
		return lib.ProviderEvaluateResponse{}, fmt.Errorf("unable to get query info")
	}

	query := cond.Referenced
	if query == "" {
		fmt.Printf("not ok did not get query info")
		return lib.ProviderEvaluateResponse{}, fmt.Errorf("unable to get query info")
	}

	symbols := p.GetAllSymbols(query)

	incidents := []lib.IncidentContext{}
	for _, s := range symbols {

		if s.Kind == protocol.Module {
			references := p.GetAllReferences(s)
			for _, ref := range references {
				// Look for things that are in the location loaded, //Note may need to filter out vendor at some point
				if strings.Contains(ref.URI, p.config.Location) {
					incidents = append(incidents, lib.IncidentContext{
						FileURI: ref.URI,
						Extras: map[string]interface{}{
							"lineNumber": ref.Range.Start.Line,
							"file":       ref.URI,
						},
					})
				}
			}
		}
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
			"bundles": absBundles,
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

func (p *javaProvider) GetAllSymbols(query string) []protocol.WorkspaceSymbol {
	// This command will run the added bundle to the language server. The command over the wire needs too look like this.
	// in this case the project is hardcoded in the init of the Langauge Server above
	// workspace/executeCommand '{"command": "io.konveyor.tackle.ruleEntry", "arguments": {"query":"*customresourcedefinition","project": "java"}}'
	arguments := map[string]string{
		"query":   query,
		"project": "java",
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

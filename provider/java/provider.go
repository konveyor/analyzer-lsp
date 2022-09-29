package java

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/shawn-hurley/jsonrpc-golang/jsonrpc2"
	"github.com/shawn-hurley/jsonrpc-golang/lsp/protocol"
	"github.com/shawn-hurley/jsonrpc-golang/provider/lib"
)

type javaProvider struct {
	config lib.Config

	bundles   []string
	workspace string

	rpc  *jsonrpc2.Conn
	ctx  context.Context
	once sync.Once
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

func (p *javaProvider) Capabilities() ([]string, error) {
	return []string{
		"referenced",
	}, nil
}

func (p *javaProvider) Evaluate(cap string, conditionInfo interface{}) (lib.ProviderEvaluateResponse, error) {
	return lib.ProviderEvaluateResponse{}, nil
}
func (p *javaProvider) Init(ctx context.Context) error {

	var returnErr error
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

		go func() {
			err := cmd.Run()
			if err != nil {
				fmt.Printf("here cmd failed- %v", err)
			}
		}()
		rpc := jsonrpc2.NewConn(jsonrpc2.NewHeaderStream(stdout, stdin))

		go func() {
			err := rpc.Run(ctx)
			if err != nil {
				fmt.Printf("connection terminated: %v", err)
			}
		}()

		p.rpc = rpc
		p.ctx = ctx
		p.initialization(ctx)
	})
	return returnErr
}

func (p *javaProvider) initialization(ctx context.Context) {

	params := &protocol.InitializeParams{
		//TODO(shawn-hurley): add ability to parse path to URI in a real supported way
		RootURI:      fmt.Sprintf("file://%v", p.config.Location),
		Capabilities: protocol.ClientCapabilities{},
		ExtendedClientCapilities: map[string]interface{}{
			"classFileContentsSupport": true,
		},
		InitializationOptions: map[string]interface{}{
			"bundles": p.bundles,
		},
	}

	var result protocol.InitializeResult
	for {
		if err := p.rpc.Call(ctx, "initialize", params, &result); err != nil {
			fmt.Printf("initialize failed: %v", err)
			continue
		}
		break
	}
	if err := p.rpc.Notify(ctx, "initialized", &protocol.InitializedParams{}); err != nil {
		fmt.Printf("initialized failed: %v", err)
	}
	fmt.Printf("\njava connection initialized: %#v", result.Capabilities.WorkspaceSymbolProvider)
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

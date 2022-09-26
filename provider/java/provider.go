package java

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/shawn-hurley/jsonrpc-golang/jsonrpc2"
	"github.com/shawn-hurley/jsonrpc-golang/lsp/protocol"
	"github.com/shawn-hurley/jsonrpc-golang/provider/lib"
)

type javaProvider struct {
	config lib.Config

	bundles   []string
	workspace string

	rpc *jsonrpc2.Conn
	ctx context.Context
}

const BUNDLES_INIT_OPTION = "bundles"
const WORKSPACE_INIT_OPTION = "workspace"

func NewJavaProvider(config lib.Config) *javaProvider {

	// Get the provider config out for this config.

	// Getting values out of provider config

	// Get
	bundlesString := config.ProviderSpecificConfig[BUNDLES_INIT_OPTION]
	bundles := strings.Split(bundlesString, ",")

	workspace := config.ProviderSpecificConfig[WORKSPACE_INIT_OPTION]

	return &javaProvider{
		config:    config,
		bundles:   bundles,
		workspace: workspace,
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

	cmd := exec.CommandContext(ctx, "/home/shurley/repos/eclipse.jdt.ls/org.eclipse.jdt.ls.product/target/repository/bin/jdtls",
		"-configuration",
		"/home/shurley/config",
		"-data",
		p.workspace,
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		fmt.Printf("HERE!!!!!! - %v", err)
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Printf("HERE!!!!!! - %v", err)
		return err
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
	return nil
}

// This can probably be shared between the two
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
	//workspace/executeCommand '{"command": "io.konveyor.tackle.ruleEntry", "arguments": {"query":"*customresourcedefinition","project": "java"}}'
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

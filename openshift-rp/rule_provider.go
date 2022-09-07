package openshiftrp

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/shawn-hurley/jsonrpc-golang/jsonrpc2"
	"github.com/shawn-hurley/jsonrpc-golang/lsp/protocol"
)

type ProviderConfig struct {
	//TODO: Should be an enum/iota
	Level string
}

type LSPConfig struct {
	Location              string
	ExtraConfigs          map[string]interface{}
	InitializationOptions map[string]interface{}
}

type golangProvider struct {
	LSPConfig
	ProviderConfig

	// TODO multiplexer
	rpc *jsonrpc2.Conn
	ctx context.Context
}

type javaProvider struct {
	LSPConfig
	ProviderConfig
	Workspace string

	rpc *jsonrpc2.Conn
	ctx context.Context
}

type Provider interface {
	Connect(ctx context.Context)
	GetAllSymbols(query string) []protocol.WorkspaceSymbol
	GetAllReferences(symbol protocol.WorkspaceSymbol) []protocol.Location
}

func NewGolangProvider(lspConfig LSPConfig, providerConfig ProviderConfig) Provider {
	return &golangProvider{
		LSPConfig:      lspConfig,
		ProviderConfig: providerConfig,
	}
}

func (p *golangProvider) Connect(ctx context.Context) {
	cmd := exec.CommandContext(ctx, "/usr/bin/gopls")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		fmt.Printf("HERE!!!!!! - %v", err)
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Printf("HERE!!!!!! - %v", err)
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

	// Lets Initiallize before returning
	p.initialization(ctx)
}

func (p *golangProvider) initialization(ctx context.Context) {
	params := &protocol.InitializeParams{
		//TODO(shawn-hurley): add ability to parse path to URI in a real supported way
		RootURI:      fmt.Sprintf("file://%v", p.LSPConfig.Location),
		Capabilities: protocol.ClientCapabilities{},
		ExtendedClientCapilities: map[string]interface{}{
			"classFileContentsSupport": true,
		},
		InitializationOptions: p.LSPConfig.InitializationOptions,
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
	fmt.Printf("\ngolang connection initialized: %#v", result.Capabilities.WorkspaceSymbolProvider)
}

func (p *golangProvider) GetAllSymbols(query string) []protocol.WorkspaceSymbol {

	wsp := &protocol.WorkspaceSymbolParams{
		Query: query,
	}

	fmt.Printf("\nQuery: %v", wsp)

	var refs []protocol.WorkspaceSymbol
	err := p.rpc.Call(p.ctx, "workspace/symbol", wsp, &refs)
	if err != nil {
		fmt.Printf("error: %v", err)
	}

	return refs
}

func (p *golangProvider) GetAllReferences(symbol protocol.WorkspaceSymbol) []protocol.Location {
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

func NewJavaProvider(lspConfig LSPConfig, providerConfig ProviderConfig, workspace string) Provider {
	return &javaProvider{
		LSPConfig:      lspConfig,
		ProviderConfig: providerConfig,
		Workspace:      workspace,
	}
}

func (p *javaProvider) Connect(ctx context.Context) {

	cmd := exec.CommandContext(ctx, "/home/shurley/repos/eclipse.jdt.ls/org.eclipse.jdt.ls.product/target/repository/bin/jdtls",
		"-configuration",
		"/home/shurley/config",
		"-data",
		p.Workspace,
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		fmt.Printf("HERE!!!!!! - %v", err)
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Printf("HERE!!!!!! - %v", err)
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
}

// This can probably be shared between the two
func (p *javaProvider) initialization(ctx context.Context) {

	params := &protocol.InitializeParams{
		//TODO(shawn-hurley): add ability to parse path to URI in a real supported way
		RootURI:      fmt.Sprintf("file://%v", p.LSPConfig.Location),
		Capabilities: protocol.ClientCapabilities{},
		ExtendedClientCapilities: map[string]interface{}{
			"classFileContentsSupport": true,
		},
		InitializationOptions: p.LSPConfig.InitializationOptions,
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

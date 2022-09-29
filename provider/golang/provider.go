package golang

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

type golangProvider struct {
	rpc *jsonrpc2.Conn
	ctx context.Context

	config lib.Config
	once   sync.Once
}

func NewGolangProvider(config lib.Config) *golangProvider {
	return &golangProvider{
		config: config,
		once:   sync.Once{},
	}
}

func (p *golangProvider) Capabilities() ([]string, error) {
	return []string{
		"referenced",
	}, nil
}

func (p *golangProvider) Evaluate(cap string, conditionInfo interface{}) (lib.ProviderEvaluateResponse, error) {

	query, ok := conditionInfo.(string)
	if !ok {
		return lib.ProviderEvaluateResponse{}, fmt.Errorf("unable to get query info")
	}

	symbols := p.GetAllSymbols(query)

	foundRefs := []map[string]string{}
	for _, s := range symbols {
		if s.Kind == protocol.Struct {
			references := p.GetAllReferences(s)
			for _, ref := range references {
				// Look for things that are in the location loaded, //Note may need to filter out vendor at some point
				if strings.Contains(ref.URI, p.config.Location) {
					foundRefs = append(foundRefs, map[string]string{
						fmt.Sprintf("location %v: %v", ref.URI, ref.Range.Start.Line): "",
					})
				}
			}
		}
	}

	if len(foundRefs) == 0 {
		return lib.ProviderEvaluateResponse{}, nil
	}
	return lib.ProviderEvaluateResponse{
		Passed:              true,
		ConditionHitContext: foundRefs,
	}, nil
}

func (p *golangProvider) Init(ctx context.Context) error {
	var returnErr error
	p.once.Do(func() {

		cmd := exec.CommandContext(ctx, "/usr/bin/gopls")
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
				fmt.Printf("cmd failed - %v", err)
				// TODO: Probably should cancel the ctx here, to shut everything down
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
	})
	return returnErr
}

func (p *golangProvider) initialization(ctx context.Context) {
	params := &protocol.InitializeParams{
		//TODO(shawn-hurley): add ability to parse path to URI in a real supported way
		RootURI:      fmt.Sprintf("file://%v", p.config.Location),
		Capabilities: protocol.ClientCapabilities{},
		ExtendedClientCapilities: map[string]interface{}{
			"classFileContentsSupport": true,
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
	fmt.Printf("\ngolang connection initialized: %#v", result.Capabilities.WorkspaceSymbolProvider)
}

func (p *golangProvider) GetAllSymbols(query string) []protocol.WorkspaceSymbol {

	wsp := &protocol.WorkspaceSymbolParams{
		Query: query,
	}

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

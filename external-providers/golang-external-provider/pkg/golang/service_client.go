package golang

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/jsonrpc2"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
)

type golangServiceClient struct {
	rpc        *jsonrpc2.Conn
	ctx        context.Context
	cancelFunc context.CancelFunc
	cmd        *exec.Cmd

	config provider.InitConfig
}

var _ provider.ServiceClient = &golangServiceClient{}

func (p *golangServiceClient) Stop() {
	p.cancelFunc()
	p.cmd.Wait()
}
func (p *golangServiceClient) Evaluate(cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
	fmt.Printf("\nHERE IN EVALUATE\n")
	var cond golangCondition
	err := yaml.Unmarshal(conditionInfo, &cond)
	if err != nil {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get query info")
	}
	query := cond.Referenced
	if query == "" {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get query info")
	}

	fmt.Printf("\nHERE IN EVALUATE get all symbols\n")
	symbols := p.GetAllSymbols(query)
	fmt.Printf("\nHERE IN EVALUATE got all symbols: %#v\n\n", symbols)

	incidents := []provider.IncidentContext{}
	for _, s := range symbols {
		if s.Kind == protocol.Struct {
			references := p.GetAllReferences(s)
			for _, ref := range references {
				// Look for things that are in the location loaded, //Note may need to filter out vendor at some point
				if strings.Contains(ref.URI, p.config.Location) {
					u, err := uri.Parse(ref.URI)
					if err != nil {
						return provider.ProviderEvaluateResponse{}, err
					}
					incidents = append(incidents, provider.IncidentContext{
						FileURI: u,
						Variables: map[string]interface{}{
							"file":       ref.URI,
							"lineNumber": ref.Range.Start.Line,
						},
					})
				}
			}
		}
	}

	if len(incidents) == 0 {
		// No results were found.
		return provider.ProviderEvaluateResponse{Matched: false}, nil
	}
	return provider.ProviderEvaluateResponse{
		Matched:   true,
		Incidents: incidents,
	}, nil
}

func (p *golangServiceClient) GetAllSymbols(query string) []protocol.WorkspaceSymbol {

	wsp := &protocol.WorkspaceSymbolParams{
		Query: query,
	}

	var refs []protocol.WorkspaceSymbol
	fmt.Printf("\nrpc call\n")
	err := p.rpc.Call(context.TODO(), "workspace/symbol", wsp, &refs)
	fmt.Printf("\nrpc called\n")
	if err != nil {
		fmt.Printf("\n\nerror: %v\n", err)
	}

	return refs
}

func (p *golangServiceClient) GetAllReferences(symbol protocol.WorkspaceSymbol) []protocol.Location {
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

func (p *golangServiceClient) initialization(ctx context.Context, log logr.Logger) {
	// Get abosulte path of location.
	abs, err := filepath.Abs(p.config.Location)
	if err != nil {
		log.Error(err, "unable to get path to analyize")
		panic(1)
	}

	params := &protocol.InitializeParams{
		//TODO(shawn-hurley): add ability to parse path to URI in a real supported way
		RootURI:      fmt.Sprintf("file://%v", abs),
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
	fmt.Printf("golang connection initialized")
	log.V(2).Info("golang connection initialized")
}

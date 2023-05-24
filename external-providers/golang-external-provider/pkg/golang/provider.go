package golang

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
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
)

// TODO(shawn-hurley): Pipe the logger through
// Determine how and where external providers will add the logs to make the logs viewable in a single location.
type golangProvider struct {
	rpc        *jsonrpc2.Conn
	ctx        context.Context
	cancelFunc context.CancelFunc
	cmd        *exec.Cmd

	config provider.InitConfig
	once   sync.Once
}

var _ provider.Client = &golangProvider{}

func (p *golangProvider) Stop() {
	// Ignore the error here, it stopped and we wanted it to.
	p.cancelFunc()
	p.cmd.Wait()
}

func NewGolangProvider() *golangProvider {
	return &golangProvider{
		once: sync.Once{},
	}
}

func (p *golangProvider) Capabilities() []provider.Capability {
	return []provider.Capability{
		{
			Name:            "referenced",
			TemplateContext: openapi3.SchemaRef{},
		},
		{
			Name:            "dependency",
			TemplateContext: openapi3.SchemaRef{},
		},
	}
}

type golangCondition struct {
	Referenced string `yaml:"referenced"`
}

func (p *golangProvider) Evaluate(cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
	var cond golangCondition
	err := yaml.Unmarshal(conditionInfo, &cond)
	if err != nil {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get query info")
	}
	query := cond.Referenced
	if query == "" {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get query info")
	}

	symbols := p.GetAllSymbols(query)

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

func (p *golangProvider) Init(_ context.Context, log logr.Logger, c provider.InitConfig) (int, error) {
	ctx := context.Background()
	ctx, cancelFunc := context.WithCancel(ctx)
	log = log.WithValues("provider", "golang")
	var returnErr error
	p.once.Do(func() {
		p.config = c
		cmd := exec.CommandContext(ctx, p.config.LSPServerPath)
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
				fmt.Printf("cmd failed - %v", err)
				// TODO: Probably should cancel the ctx here, to shut everything down
				return
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

		// Lets Initiallize before returning
		p.initialization(ctx, log)
	})
	return 0, returnErr
}

func (p *golangProvider) initialization(ctx context.Context, log logr.Logger) {
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

func (p *golangProvider) GetAllSymbols(query string) []protocol.WorkspaceSymbol {

	wsp := &protocol.WorkspaceSymbolParams{
		Query: query,
	}

	var refs []protocol.WorkspaceSymbol
	err := p.rpc.Call(context.TODO(), "workspace/symbol", wsp, &refs)
	if err != nil {
		fmt.Printf("\n\nerror: %v\n", err)
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

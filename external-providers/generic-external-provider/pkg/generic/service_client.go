package generic

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
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

type genericServiceClient struct {
	rpc        *jsonrpc2.Conn
	ctx        context.Context
	cancelFunc context.CancelFunc
	cmd        *exec.Cmd

	config provider.InitConfig
}

var _ provider.ServiceClient = &genericServiceClient{}

func (p *genericServiceClient) Stop() {
	p.cancelFunc()
	p.cmd.Wait()
}
func (p *genericServiceClient) Evaluate(cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
	var cond genericCondition
	err := yaml.Unmarshal(conditionInfo, &cond)
	if err != nil {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get query info")
	}
	query := cond.Referenced.Pattern
	if query == "" {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get query info")
	}

	symbols := p.GetAllSymbols(query)

	incidents := []provider.IncidentContext{}
	for _, s := range symbols {
		references := p.GetAllReferences(s.Location)
		for _, ref := range references {
			// Look for things that are in the location loaded, //Note may need to filter out vendor at some point
			if strings.Contains(ref.URI, p.config.Location) {
				u, err := uri.Parse(ref.URI)
				if err != nil {
					return provider.ProviderEvaluateResponse{}, err
				}
				lineNumber := int(ref.Range.Start.Line)
				incidents = append(incidents, provider.IncidentContext{
					FileURI:    u,
					LineNumber: &lineNumber,
					Variables: map[string]interface{}{
						"file": ref.URI},
				})
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

func (p *genericServiceClient) GetAllSymbols(query string) []protocol.WorkspaceSymbol {
	logger, err := os.OpenFile("golang-provider.log",
		os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println(err)
	}
	defer logger.Close()

	wsp := &protocol.WorkspaceSymbolParams{
		Query: query,
	}

	var symbols []protocol.WorkspaceSymbol
	fmt.Printf("\nrpc call\n")
	logger.WriteString("\nrpc call\n")
	err = p.rpc.Call(context.TODO(), "workspace/symbol", wsp, &symbols)
	fmt.Printf("\nrpc called\n")
	if err != nil {
		fmt.Printf("\n\nerror: %v\n", err)
		logger.WriteString(err.Error() + "\n")
	}
	logger.WriteString(fmt.Sprintf("First try: %+v\n", symbols))
	regex, err := regexp.Compile(query)
	if err != nil {
		logger.WriteString(err.Error())
		// Not a valid regex, can't do anything more
		return symbols
	}
	if len(symbols) == 0 {
		// Run empty string query and manually search using the query as a regex
		var allSymbols []protocol.WorkspaceSymbol
		err = p.rpc.Call(context.TODO(), "workspace/symbol", &protocol.WorkspaceSymbolParams{Query: ""}, &allSymbols)
		if err != nil {
			fmt.Printf("\n\nerror: %v\n", err)
		}
		for _, s := range allSymbols {
			if regex.MatchString(s.Name) {
				symbols = append(symbols, s)
			}
		}
	}
	logger.WriteString(fmt.Sprintf("Second try: %+v\n", symbols))
	if len(symbols) == 0 {
		var positions []protocol.TextDocumentPositionParams
		// Fallback to manually searching for an occurrence and performing a GotoDefinition call
		visit := func(path string, f os.FileInfo, err error) error {
			logger.WriteString(fmt.Sprintf("Visiting %s\n", path))
			if f.Mode().IsRegular() {
				content, err := os.ReadFile(path)
				if err != nil {
					logger.WriteString(err.Error() + "\n")
					return err
				}
				if regex.Match(content) {
					logger.WriteString("Found a match\n")
					// Find the exact location
					scanner := bufio.NewScanner(strings.NewReader(string(content)))
					lineNumber := 0
					for scanner.Scan() {
						matchLocations := regex.FindAllStringIndex(scanner.Text(), -1)
						logger.WriteString(fmt.Sprintf("Locations: %+v\n", matchLocations))
						for _, loc := range matchLocations {
							logger.WriteString(fmt.Sprintf("%s, %d, %+v\n", path, lineNumber, loc))
							positions = append(positions, protocol.TextDocumentPositionParams{
								TextDocument: protocol.TextDocumentIdentifier{
									URI: fmt.Sprintf("file://%s", path),
								},
								Position: protocol.Position{
									Line:      float64(lineNumber),
									Character: float64(loc[1]),
								},
							})
						}
						lineNumber++
					}
				}
			}
			return nil
		}
		err = filepath.Walk(p.config.Location, visit)
		if err != nil {
			logger.WriteString(err.Error() + "\n")
			// return err
		}
		for _, position := range positions {
			fmt.Println(position)
			logger.WriteString(fmt.Sprintf("%+v\n", position))
			res := []protocol.Location{}
			err := p.rpc.Call(p.ctx, "textDocument/definition", position, &res)
			if err != nil {
				fmt.Printf("Error rpc: %v", err)
				logger.WriteString(err.Error() + "\n")
			}
			logger.WriteString(fmt.Sprintf("%+v\n", res))
			for _, r := range res {
				symbols = append(symbols, protocol.WorkspaceSymbol{Location: r})
			}
			logger.WriteString(fmt.Sprintf("%+v\n", symbols))
		}
	}
	return symbols
}

func (p *genericServiceClient) GetAllReferences(location protocol.Location) []protocol.Location {
	params := &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: location.URI,
			},
			Position: location.Range.Start,
		},
	}

	res := []protocol.Location{}
	err := p.rpc.Call(p.ctx, "textDocument/references", params, &res)
	if err != nil {
		fmt.Printf("Error rpc: %v", err)
	}
	return res
}

func (p *genericServiceClient) initialization(ctx context.Context, log logr.Logger) {
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
	fmt.Printf("provider connection initialized")
	log.V(2).Info("provider connection initialized")
}

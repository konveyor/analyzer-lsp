package generic

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

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

func processFile(path string, regex *regexp.Regexp, positionsChan chan<- protocol.TextDocumentPositionParams, wg *sync.WaitGroup) {
	defer wg.Done()

	content, err := os.ReadFile(path)
	if err != nil {
		return
	}

	if regex.Match(content) {
		scanner := bufio.NewScanner(strings.NewReader(string(content)))
		lineNumber := 0
		for scanner.Scan() {
			matchLocations := regex.FindAllStringIndex(scanner.Text(), -1)
			for _, loc := range matchLocations {
				absPath, err := filepath.Abs(path)
				if err != nil {
					return
				}
				positionsChan <- protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{
						URI: fmt.Sprintf("file://%s", absPath),
					},
					Position: protocol.Position{
						Line:      float64(lineNumber),
						Character: float64(loc[1]),
					},
				}
			}
			lineNumber++
		}
	}
}

func parallelWalk(location string, regex *regexp.Regexp) ([]protocol.TextDocumentPositionParams, error) {
	var positions []protocol.TextDocumentPositionParams
	positionsChan := make(chan protocol.TextDocumentPositionParams)
	wg := &sync.WaitGroup{}

	go func() {
		err := filepath.Walk(location, func(path string, f os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if f.Mode().IsRegular() {
				wg.Add(1)
				go processFile(path, regex, positionsChan, wg)
			}

			return nil
		})

		if err != nil {
			return
		}

		wg.Wait()
		close(positionsChan)
	}()

	for pos := range positionsChan {
		positions = append(positions, pos)
	}

	return positions, nil
}

func (p *genericServiceClient) GetAllSymbols(query string) []protocol.WorkspaceSymbol {
	wsp := &protocol.WorkspaceSymbolParams{
		Query: query,
	}

	var symbols []protocol.WorkspaceSymbol
	fmt.Printf("\nrpc call\n")
	err := p.rpc.Call(context.TODO(), "workspace/symbol", wsp, &symbols)
	fmt.Printf("\nrpc called\n")
	if err != nil {
		fmt.Printf("\n\nerror: %v\n", err)
	}
	regex, err := regexp.Compile(query)
	if err != nil {
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
	if len(symbols) == 0 {
		var positions []protocol.TextDocumentPositionParams
		// Fallback to manually searching for an occurrence and performing a GotoDefinition call
		positions, err := parallelWalk(p.config.Location, regex)
		if err != nil {
			fmt.Printf("\n\nerror: %v\n", err)
			return nil
		}
		for _, position := range positions {
			fmt.Println(position)
			res := []protocol.Location{}
			err := p.rpc.Call(p.ctx, "textDocument/definition", position, &res)
			if err != nil {
				fmt.Printf("Error rpc: %v", err)
			}
			for _, r := range res {
				symbols = append(symbols, protocol.WorkspaceSymbol{Location: r})
			}
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

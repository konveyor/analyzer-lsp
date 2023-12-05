package generic

import (
	"bufio"
	"context"
	"encoding/json"
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
	cancelFunc context.CancelFunc
	cmd        *exec.Cmd

	config       provider.InitConfig
	capabilities protocol.ServerCapabilities
}

var _ provider.ServiceClient = &genericServiceClient{}

func (p *genericServiceClient) Stop() {
	p.cancelFunc()
	p.cmd.Wait()
}

func (p *genericServiceClient) Evaluate(ctx context.Context, cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
	var cond genericCondition
	err := yaml.Unmarshal(conditionInfo, &cond)
	if err != nil {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get query info")
	}
	query := cond.Referenced.Pattern
	if query == "" {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get query info")
	}

	symbols := p.GetAllSymbols(ctx, query)

	incidents := []provider.IncidentContext{}
	incidentsMap := make(map[string]provider.IncidentContext) // To remove duplicates

	for _, s := range symbols {
		references := p.GetAllReferences(ctx, s.Location.Value.(protocol.Location))
		for _, ref := range references {
			// Look for things that are in the location loaded,
			// Note may need to filter out vendor at some point
			if strings.Contains(ref.URI, p.config.Location) {

				var referencedOutputIgnoreContains []interface{}
				if p.config.ProviderSpecificConfig["referencedOutputIgnoreContains"] != nil {
					referencedOutputIgnoreContains = p.config.ProviderSpecificConfig["referencedOutputIgnoreContains"].([]interface{})
				}

				foundSubstr := false
				for _, x := range referencedOutputIgnoreContains {
					substr := x.(string)
					if substr == "" {
						continue
					}

					if strings.Contains(ref.URI, substr) {
						foundSubstr = true
						break
					}
				}
				if foundSubstr {
					continue
				}

				u, err := uri.Parse(ref.URI)
				if err != nil {
					return provider.ProviderEvaluateResponse{}, err
				}
				lineNumber := int(ref.Range.Start.Line)
				incident := provider.IncidentContext{
					FileURI:    u,
					LineNumber: &lineNumber,
					Variables: map[string]interface{}{
						"file": ref.URI,
					},
				}
				b, err := json.Marshal(incident)
				if err != nil {
					fmt.Printf("Marshalling error= %v", err)
				}
				fmt.Printf("INCIDENT= %v", b)
				incidentsMap[string(b)] = incident
			}
		}
	}

	for _, incident := range incidentsMap {
		incidents = append(incidents, incident)
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
						Line:      uint32(lineNumber),
						Character: uint32(loc[1]),
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

// Returns all symbols for the given query.
// NOTE: Only returns definitions when server does not supoprt workspace/symbol.
// Is is intended behavior?
// TODO: Change protocol.WorkspaceSymbol to protocol.SymbolInformation
func (p *genericServiceClient) GetAllSymbols(ctx context.Context, query string) []protocol.WorkspaceSymbol {
	wsp := &protocol.WorkspaceSymbolParams{
		Query: query,
	}

	var symbols []protocol.WorkspaceSymbol
	var err error

	regex, regexErr := regexp.Compile(query)

	// Client may or may not support the "workspace/symbol" method, so we must
	// check before calling.

	if p.capabilities.Supports("workspace/symbol") {
		err := p.rpc.Call(ctx, "workspace/symbol", wsp, &symbols)
		if err != nil {
			fmt.Printf("error: %v\n", err)
		}
	}

	if regexErr != nil {
		// Not a valid regex, can't do anything more
		return symbols
	}

	if p.capabilities.Supports("workspace/symbol") && len(symbols) == 0 {
		// Run empty string query and manually search using the query as a regex
		var allSymbols []protocol.WorkspaceSymbol
		err = p.rpc.Call(ctx, "workspace/symbol", &protocol.WorkspaceSymbolParams{Query: ""}, &allSymbols)
		if err != nil {
			fmt.Printf("error: %v\n", err)
		}
		for _, s := range allSymbols {
			if regex.MatchString(s.Name) {
				symbols = append(symbols, s)
			}
		}
	}

	if p.capabilities.Supports("textDocument/definition") && len(symbols) == 0 {
		var positions []protocol.TextDocumentPositionParams
		symbolMap := make(map[string]protocol.WorkspaceSymbol) // To avoid repeats

		// Fallback to manually searching for an occurrence and performing a
		// GotoDefinition call

		// Lambda function to support switch to workspace folders
		walkFiles := func(locations []string) error {
			for _, location := range locations {
				if location == "" {
					continue
				}

				result, err := parallelWalk(location, regex)
				if err != nil {
					return fmt.Errorf("error: %v", err)
				}

				positions = append(positions, result...)
			}

			return nil
		}

		err := walkFiles([]string{p.config.Location})
		if err != nil {
			fmt.Printf("%s\n", err.Error())
			return nil
		}

		// Leaving this in here until we determine whether we can use workspace
		// folders

		// err = walkFiles(p.Config.WorkspaceFolders)
		// if err != nil {
		// 	fmt.Printf("%s\n", err.Error())
		// 	return nil
		// }
		// err = walkFiles(p.Config.DependencyFolders)
		// if err != nil {
		// 	fmt.Printf("%s\n", err.Error())
		// 	return nil
		// }

		for _, position := range positions {
			res := []protocol.Location{}
			err := p.rpc.Call(ctx, "textDocument/definition", position, &res)
			if err != nil {
				fmt.Printf("Error rpc: %v", err)
			}

			for _, r := range res {
				out, _ := json.Marshal(r)
				symbolMap[string(out)] = protocol.WorkspaceSymbol{
					Location: protocol.OrPLocation_workspace_symbol{
						Value: r,
					},
				}
			}
		}

		for _, ws := range symbolMap {
			symbols = append(symbols, ws)
		}
	}

	return symbols
}

func (p *genericServiceClient) GetAllReferences(ctx context.Context, location protocol.Location) []protocol.Location {
	params := &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: location.URI,
			},
			Position: location.Range.Start,
		},
		Context: protocol.ReferenceContext{
			// pylsp has trouble with always returning declarations
			IncludeDeclaration: false,
		},
	}

	res := []protocol.Location{}
	err := p.rpc.Call(ctx, "textDocument/references", params, &res)
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

	//TODO(shawn-hurley): add ability to parse path to URI in a real supported way
	params := &protocol.InitializeParams{}
	params.RootURI = fmt.Sprintf("file://%v", abs)

	// Leaving this in here until we determine whether we can use workspace
	// folders

	// var allWorkspaceFolders []string
	// copy(allWorkspaceFolders, p.Config.WorkspaceFolders)
	// allWorkspaceFolders = append(allWorkspaceFolders, p.Config.DependencyFolders...)

	// for _, path := range allWorkspaceFolders {
	// 	abs, err := filepath.Abs(path)
	// 	if err != nil {
	// 		log.Error(err, "unable to get path to analyize")
	// 		panic(1)
	// 	}

	// 	params.WorkspaceFolders = append(params.WorkspaceFolders, protocol.WorkspaceFolder{
	// 		URI:  abs,
	// 		Name: abs,
	// 	})
	// }

	params.Capabilities = protocol.ClientCapabilities{}
	params.ExtendedClientCapilities = map[string]interface{}{
		"classFileContentsSupport": true,
	}

	var result protocol.InitializeResult
	for {
		err := p.rpc.Call(ctx, "initialize", params, &result)
		if err == nil {
			break
		}

		// Code to attempt to fix broken initialize responses. There is probably a
		// better way than shown here.

		rpcerr, ok := err.(*jsonrpc2.RPCUnmarshalError)
		if !ok {
			fmt.Printf("initialized failed: %v\n", err)
			continue
		}

		fix := NaiveFixResponse(p.config.ProviderSpecificConfig["name"].(string), rpcerr.Json)

		err = json.Unmarshal([]byte(fix), &result)
		if err != nil {
			fmt.Printf("initialized failed: %v", err)
			continue
		}

		break
	}

	p.capabilities = result.Capabilities

	if err := p.rpc.Notify(ctx, "initialized", &protocol.InitializedParams{}); err != nil {
		fmt.Printf("initialized failed: %v", err)
	}
	fmt.Printf("provider connection initialized\n")
	log.V(2).Info("provider connection initialized\n")
}

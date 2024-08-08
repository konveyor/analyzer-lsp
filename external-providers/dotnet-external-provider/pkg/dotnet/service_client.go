package dotnet

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
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
)

type dotnetServiceClient struct {
	rpc        jsonrpc2.Conn
	ctx        context.Context
	cancelFunc context.CancelFunc
	cmd        *exec.Cmd
	log        logr.Logger

	config provider.InitConfig
}

var _ provider.ServiceClient = &dotnetServiceClient{}

func (d *dotnetServiceClient) Stop() {
	d.cancelFunc()
	d.cmd.Wait()
}

func (d *dotnetServiceClient) Evaluate(ctx context.Context, cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
	var cond dotnetCondition
	err := yaml.Unmarshal(conditionInfo, &cond)
	if err != nil {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("failed to unmarshal condition")
	}

	query := cond.Referenced.Pattern
	if query == "" {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get query info")
	}

	namespace := cond.Referenced.Namespace
	if namespace == "" {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get namespace for query")
	}

	symbols := d.GetAllSymbols(query)
	incidents := []provider.IncidentContext{}
	for _, s := range symbols {
		if s.Kind == protocol.SymbolKindMethod {
			references := d.GetAllReferences(s)
			for _, ref := range references {
				if strings.Contains(ref.URI.Filename(), d.config.Location) {
					lineNumber := int(ref.Range.Start.Line)
					incidents = append(incidents, provider.IncidentContext{
						FileURI:    ref.URI,
						LineNumber: &lineNumber,
						Variables: map[string]interface{}{
							"file": ref.URI.Filename(),
						},
						CodeLocation: &provider.Location{
							StartPosition: provider.Position{Line: float64(lineNumber)},
							EndPosition:   provider.Position{Line: float64(lineNumber)},
						},
					})
				}
			}
		}
	}

	if len(incidents) == 0 {
		// Fallback to manually searching for an occurrence and performing a GotoDefinition call
		d.log.Info("falling back to manual search for query string")
		regex, err := regexp.Compile(query)
		if err != nil {
			// Not a valid regex, can't do anything more
			return provider.ProviderEvaluateResponse{Matched: false}, nil
		}
		var positions []interface{}
		positions, err = parallelWalk(d.config.Location, regex)
		if err != nil {
			d.log.Error(err, "failed parallel walk")
			return provider.ProviderEvaluateResponse{Matched: false}, nil
		}
		for _, position := range positions {
			fmt.Println("%#v", position)
			res := []protocol.Location{}
			switch position.(type) {
			case protocol.ReferenceParams:
				_, err := d.rpc.Call(d.ctx, protocol.MethodTextDocumentReferences, position, &res)
				if err != nil {
					d.log.Error(err, "failed to get references")
				}
			case protocol.TextDocumentPositionParams:
				_, err := d.rpc.Call(d.ctx, "textDocument/definition", position, &res)
				if err != nil {
					d.log.Error(err, "problem getting definition")
					continue
				}
				if len(res) == 0 || len(res) > 1 {
					d.log.Error(fmt.Errorf("only expect one result"), "too many, or not enough, results")
					continue
				}
			}

			for _, r := range res {
				switch filename := string(res[0].URI); {
				case strings.HasPrefix(filename, "csharp"):
					// As best I understand, this would require the definition to be defined
					// outside the project...like a third-party dep.

					// "csharp:/metadata/projects/NerdDinner/assemblies/System.Web.Mvc/symbols/System.Web.Mvc.Controller.cs"
					split := strings.Split(filename, "assemblies/")
					if strings.HasPrefix(split[1], namespace) {
						lineNumber := int(r.Range.Start.Line)
						incidents = append(incidents, provider.IncidentContext{
							FileURI:    r.URI,
							LineNumber: &lineNumber,
							Variables: map[string]interface{}{
								"file": string(r.URI),
							},
							CodeLocation: &provider.Location{
								StartPosition: provider.Position{Line: float64(lineNumber)},
								EndPosition:   provider.Position{Line: float64(lineNumber)},
							},
						})
					}
				case strings.HasPrefix(filename, "file"):
					// TODO(djzager): do we even need to handle these?
					d.log.Error(fmt.Errorf("not implemented"), "don't know how to handle file URI")
					continue
				}
			}

		}
	}
	return provider.ProviderEvaluateResponse{
		Matched:   len(incidents) > 0,
		Incidents: incidents,
	}, nil
}

func processFile(path string, regex *regexp.Regexp, positionsChan chan<- interface{}, wg *sync.WaitGroup) {
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
				if strings.Contains(scanner.Text(), "using") {
					positionsChan <- protocol.ReferenceParams{
						TextDocumentPositionParams: protocol.TextDocumentPositionParams{
							TextDocument: protocol.TextDocumentIdentifier{
								URI: uri.New(fmt.Sprintf("file:///%s", absPath)),
							},
							Position: protocol.Position{
								Line:      uint32(lineNumber),
								Character: uint32(loc[1]),
							},
						},
					}
				} else {
					positionsChan <- protocol.TextDocumentPositionParams{
						TextDocument: protocol.TextDocumentIdentifier{
							URI: uri.New(fmt.Sprintf("file:///%s", absPath)),
						},
						Position: protocol.Position{
							Line:      uint32(lineNumber),
							Character: uint32(loc[1]),
						},
					}
				}
			}
			lineNumber++
		}
	}
}

func parallelWalk(location string, regex *regexp.Regexp) ([]interface{}, error) {
	var positions []interface{}
	positionsChan := make(chan interface{})
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

func (d *dotnetServiceClient) GetAllSymbols(query string) []protocol.SymbolInformation {
	wsp := &protocol.WorkspaceSymbolParams{
		Query: query,
	}

	var refs []protocol.SymbolInformation
	_, err := d.rpc.Call(context.TODO(), protocol.MethodWorkspaceSymbol, wsp, &refs)
	if err != nil {
		d.log.Error(err, "failed to get workspace symbols")
	}

	return refs
}

func (d *dotnetServiceClient) GetAllReferences(symbol protocol.SymbolInformation) []protocol.Location {
	params := &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: symbol.Location.URI,
			},
			Position: symbol.Location.Range.Start,
		},
	}

	res := []protocol.Location{}
	_, err := d.rpc.Call(d.ctx, protocol.MethodTextDocumentReferences, params, &res)
	if err != nil {
		d.log.Error(err, "failed to get references")
	}
	return res
}

func (d *dotnetServiceClient) GetDependencies(ctx context.Context) (map[uri.URI][]*provider.Dep, error) {
	return map[uri.URI][]*provider.Dep{}, nil
}

func (d *dotnetServiceClient) GetDependenciesDAG(ctx context.Context) (map[uri.URI][]provider.DepDAGItem, error) {
	return map[uri.URI][]provider.DepDAGItem{}, nil
}

// this is a struct for providing the server that lives on the client side
// of the connection to properly respond to requests sent server -> client
type handler struct {
	log *logr.Logger
	ch  chan int
}

func (h *handler) replyHandler(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	method := req.Method()
	h.log.Info("Got request for " + method)
	switch method {
	case protocol.MethodClientRegisterCapability:
		err := reply(ctx, nil, nil)
		return err
	case protocol.MethodWorkspaceConfiguration:
		err := reply(ctx, nil, nil)
		return err
	case protocol.MethodWindowShowMessage:
		var showMessageParams protocol.ShowMessageParams
		if err := json.Unmarshal(req.Params(), &showMessageParams); err != nil {
			return reply(ctx, nil, err)
		}
		err := reply(ctx, nil, nil)
		if strings.HasSuffix(showMessageParams.Message, "project files loaded") ||
			strings.HasSuffix(showMessageParams.Message, "project file(s) loaded") ||
			strings.Contains(showMessageParams.Message, "finished loading solution") {
			h.ch <- 1
		}
		return err
	case protocol.MethodProgress:
		var methodProgressParams protocol.ProgressParams
		if err := json.Unmarshal(req.Params(), &methodProgressParams); err != nil {
			return reply(ctx, nil, err)
		}
		h.log.Info("Progress message", "params", methodProgressParams)
		err := reply(ctx, nil, nil)
		valueMap, _ := methodProgressParams.Value.(map[string]interface{})
		if message, ok := valueMap["message"]; ok {
			h.log.Info("Extracted message", "message", message)
			if strings.HasSuffix(message.(string), "project files loaded") ||
				strings.HasSuffix(message.(string), "project file(s) loaded") ||
				strings.Contains(message.(string), "finished loading solution") {
				h.ch <- 3
			}
		} else {
			err = fmt.Errorf("Something terrible has happened loading progress message")
		}
		return err
	case protocol.MethodWorkDoneProgressCreate, protocol.MethodWorkDoneProgressCancel:
		err := reply(ctx, nil, nil)
		return err
	}

	h.log.Info("I don't know what to do with this", "request", req)
	return jsonrpc2.MethodNotFoundHandler(ctx, reply, req)
}

package base

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
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-logr/logr"
	jsonrpc2 "github.com/konveyor/analyzer-lsp/jsonrpc2_v2"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
)

// Cannot have generic function type aliases, but this is still better than typing out
// the entire function type definition
type LSPServiceClientFunc[T HasLSPServiceClientBase] func(T, context.Context, string, []byte) (provider.ProviderEvaluateResponse, error)

// Almost the same struct that we return to `analyzer-lsp` but with the added
// `Fn` field to reduce code duplication. We can use this struct for the
// Evaluator struct to call the appropriate method when queried.
type LSPServiceClientCapability struct {
	Name            string
	TemplateContext openapi3.SchemaRef
	Fn              interface{}
}

// The base service client configs that all subsequent configs must embed
type LSPServiceClientConfig struct {
	// The name of the server. Think `yaml_language_server` not `yaml`
	LspServerName string `yaml:"lspServerName,omitempty"`

	// Where the binary of the server is. Not a URI. Passed to exec.CommandContext
	LspServerPath string `yaml:"lspServerPath,omitempty"`

	// The args of the lsp server. Passed to exec.CommandContext.
	LspServerArgs []string `yaml:"lspServerArgs,omitempty"`

	// JSON string that can get sent to the initialize request instead of the
	// computed options in the service client. Each service client can implement
	// this differently. Must be a string due to grpc not allowing nested structs.
	LspServerInitializationOptions string `yaml:"lspServerInitializationOptions,omitempty"`

	// Full URI of the workspace folders under consideration
	WorkspaceFolders []string `yaml:"workspaceFolders,omitempty"`
	// Full URI of the dependency folders under consideration. Used for ignoring
	// results from things like `referenced`
	DependencyFolders []string `yaml:"dependencyFolders,omitempty"`

	// Path to a simple binary that lists the dependencies for a given language.
	DependencyProviderPath string `yaml:"dependencyProviderPath,omitempty"`
}

// Provides a generic `Evaluate` method, that calls the associated method found
// in the struct's FuncMap field. T should be a service client pointer
type LSPServiceClientEvaluator[T HasLSPServiceClientBase] struct {
	Parent  T
	FuncMap map[string]LSPServiceClientFunc[T]
}

// While you could implement the `Evaluate` method yourself as a massive
// switch-case block, it gets unwieldy after the number of capabilities grows.
// Additionally, whenever you add new capabilities, you have to modify things in
// three places: the function itself, the array of capabilities that gets
// advertised to the analyzer-lsp, and the `Evaluate` method. Embedding this
// struct knocks that down to two. (Theoretically, you could knock it down to
// one by defining the methods right in the array, but then you can't they can't
// reference each other). This is also why the LSPServiceClientCapability struct
// has a `Fn` field - specifically for this evaluator.
func NewLspServiceClientEvaluator[T HasLSPServiceClientBase](
	parent T, capabilities []LSPServiceClientCapability,
) (*LSPServiceClientEvaluator[T], error) {
	evaluator := LSPServiceClientEvaluator[T]{}

	// Load all the capabilities into the `evaluate` map
	evaluator.Parent = parent
	evaluator.FuncMap = make(map[string]LSPServiceClientFunc[T])
	for _, capability := range capabilities {
		fn, ok := capability.Fn.(LSPServiceClientFunc[T])
		if !ok {
			return nil, fmt.Errorf("couldn't convert function to LSPServiceClientFunc[%T]. capability: %s", parent, capability.Name)
		}
		evaluator.FuncMap[capability.Name] = fn
	}

	return &evaluator, nil
}

// The evaluate method. Looks in the FuncMap and sees if `cap` matches. Executes
// the function if it does.
func (sc *LSPServiceClientEvaluator[T]) Evaluate(ctx context.Context, cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
	if fn, ok := sc.FuncMap[cap]; ok {
		return fn(sc.Parent, ctx, cap, conditionInfo)
	}

	return provider.ProviderEvaluateResponse{}, fmt.Errorf("capability '%s' not supported", cap)
}

// For the generic methods we must convert the T to a *LSPServiceClientBase[T],
// so we need to make sure that it has this method. Kind of a hack
type HasLSPServiceClientBase interface {
	GetLSPServiceClientBase() *LSPServiceClientBase
}

// Almost everything implemented to satisfy the protocol.ServiceClient
// interface. The only thing that's not is `Evaluate`, intentionally.
//
// TODO(jsussman): Evaluate the merits of creating a separate ServiceClientBase
// that this inherits so we can talk to non-lsp things as well (for example,
// `yq` for yaml). This would involve doing something like extracting out
// `NewCmdDialer` (incorrect phraseology). Server & Client handler, look at
// jsonrpc2's serve_test.go
type LSPServiceClientBase struct {
	Ctx        context.Context
	CancelFunc context.CancelFunc
	Log        logr.Logger

	BaseConfig LSPServiceClientConfig

	Dialer *CmdDialer
	Conn   *jsonrpc2.Connection

	// Will call this handler's Handle function first. If it returns an
	// ErrMethodNotFound or ErrNotHandled we use the LSPServiceClientBase's Handle
	// method.
	//
	// TODO(jsussman): Determine which errors are "acceptable" for us to continue
	// using the base's Handle function
	// ChainHandler *jsonrpc2.Handler

	// There are some concerns about cache inconsistency when using AwaitCache, so
	// for simplicity, we should probably only get diagnostics for each file
	// exactly once.
	PublishDiagnosticsCache *AwaitCache[string, []protocol.Diagnostic]

	ServerCapabilities protocol.ServerCapabilities
	ServerInfo         *protocol.PServerInfoMsg_initialize

	TempDir string
}

func NewLSPServiceClientBase(
	ctx context.Context, log logr.Logger, c provider.InitConfig,
	initializeHandler jsonrpc2.Handler,
	initializeParams protocol.InitializeParams,
) (*LSPServiceClientBase, error) {
	sc := LSPServiceClientBase{}

	// Load the provider / service client specific config. Transforming from
	// map[string]any -> yaml string -> ServiceClient
	b, _ := yaml.Marshal(c.ProviderSpecificConfig)
	err := yaml.Unmarshal(b, &sc.BaseConfig)
	if err != nil {
		return nil, fmt.Errorf("base config unmarshal error: %w", err)
	}

	if sc.BaseConfig.LspServerPath == "" {
		return nil, fmt.Errorf("must provide lspServerPath")
	}

	if sc.BaseConfig.LspServerName == "" {
		sc.BaseConfig.LspServerName = "generic"
	}

	if initializeParams.RootURI == "" && len(initializeParams.WorkspaceFolders) == 0 {
		TempDir, err := os.MkdirTemp("", "tmp")
		if err != nil {
			return nil, fmt.Errorf("tmp dir error: %w", err)
		}

		sc.TempDir = TempDir
		initializeParams.RootURI = "file://" + TempDir
	}

	if !strings.HasPrefix(initializeParams.RootURI, "file://") && len(initializeParams.WorkspaceFolders) == 0 {
		initializeParams.RootURI = "file://" + initializeParams.RootURI
	}

	if initializeParams.ProcessID == 0 {
		initializeParams.ProcessID = int32(os.Getpid())
	}

	// Create the ctx, cancelFunc, and log
	sc.Ctx, sc.CancelFunc = context.WithCancel(ctx)
	sc.Log = log.WithValues("provider", sc.BaseConfig.LspServerName)

	// launch the lsp command
	sc.Dialer, err = NewCmdDialer(
		sc.Ctx, sc.BaseConfig.LspServerPath, sc.BaseConfig.LspServerArgs...,
	)
	if err != nil {
		return nil, fmt.Errorf("new cmd dialer error: %w", err)
	}

	time.Sleep(5 * time.Second)

	// Create the caches for the various handler stuffs
	sc.PublishDiagnosticsCache = NewAwaitCache[string, []protocol.Diagnostic]()

	// Create a connection to the lsp server
	sc.Conn, err = jsonrpc2.Dial(
		sc.Ctx, sc.Dialer, jsonrpc2.ConnectionOptions{
			Handler: NewChainHandler(&sc, initializeHandler),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("jsonrpc2.Dial error: %w", err)
	}

	var result json.RawMessage
	err = sc.Conn.Call(sc.Ctx, "initialize", initializeParams).Await(sc.Ctx, &result)
	if err != nil {
		b, _ := json.Marshal(initializeParams)
		return nil, fmt.Errorf("initialize request error: %w, result: %s, initializeParams: %s, Dialer: %v", err, string(result), string(b), sc.Dialer)
	}

	fmt.Printf("%s\n", string(result))

	initializeResult := protocol.InitializeResult{}
	err = json.Unmarshal(result, &initializeResult)
	if err != nil {
		return nil, fmt.Errorf("initialize result unmarshal error: %w", err)
	}

	sc.ServerCapabilities = initializeResult.Capabilities
	sc.ServerInfo = initializeResult.ServerInfo

	err = sc.Conn.Notify(sc.Ctx, "initialized", protocol.InitializeParams{})
	if err != nil {
		return nil, fmt.Errorf("initialized notification error: %w", err)
	}

	fmt.Printf("provider connection initialized\n")
	sc.Log.V(2).Info("provider connection initialized\n")

	return &sc, nil
}

// Method exists so that we can do generic capabilities. See
// `base_capabilities.go` for examples
func (sc *LSPServiceClientBase) GetLSPServiceClientBase() *LSPServiceClientBase {
	return sc
}

// Shut down any spawned helper processes
func (sc *LSPServiceClientBase) Stop() {
	sc.CancelFunc()
	sc.Conn.Close()

	if sc.TempDir != "" {
		os.RemoveAll(sc.TempDir)
	}
}

// This GetDependencies method was the one that was present in the
// generic-external-provider before I got my hands on it. Not too sure what it's
// used for. I didn't want to break anything so I just made it the default
// implementation.
func (sc *LSPServiceClientBase) GetDependencies(ctx context.Context) (map[uri.URI][]*provider.Dep, error) {
	cmdStr := sc.BaseConfig.DependencyProviderPath
	if cmdStr == "" {
		return nil, fmt.Errorf("dependency provider path not set")
	}
	// Expects dependency provider to output provider.Dep structs to stdout
	cmd := exec.Command(cmdStr)
	cmd.Dir = sc.BaseConfig.WorkspaceFolders[0][7:]
	dataR, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	data := string(dataR)
	if len(data) == 0 {
		return nil, nil
	}
	m := map[uri.URI][]*provider.Dep{}
	err = json.Unmarshal([]byte(data), &m)
	if err != nil {
		return nil, err
	}
	return m, err
}

// We don't have dependencies
func (sc *LSPServiceClientBase) GetDependenciesDAG(ctx context.Context) (map[uri.URI][]provider.DepDAGItem, error) {
	return nil, nil
}

func (sc *LSPServiceClientBase) Handle(ctx context.Context, req *jsonrpc2.Request) (result interface{}, err error) {
	// fmt.Printf("Base Handler!\n")

	switch req.Method {
	case "textDocument/publishDiagnostics":
		var res protocol.PublishDiagnosticsParams
		err := json.Unmarshal(req.Params, &res)
		if err != nil {
			return nil, err
		}

		// fmt.Printf("Fake wait.\n")
		// time.Sleep(3 * time.Second)

		sc.PublishDiagnosticsCache.Set(res.URI, res.Diagnostics)

		return nil, nil
	}

	return nil, jsonrpc2.ErrNotHandled
}

// ---

// Returns all top-level declaration symbols for the given query.
//
// gopls's `workspace/symbol` only returns the *top-level declarations* in each
// file (see [^1]). Each LSP server has different semantics for handling
// queries.
//
// - gopls: https://github.com/golang/tools/blob/master/gopls/doc/features.md#symbol-queries
// - pylsp: https://jedi.readthedocs.io/en/latest/docs/api.html#jedi.Project.search
//
// [^1]: https://github.com/golang/tools/blob/ecbfa885b278478686e8b8efb52535e934c53ec5/gopls/internal/lsp/cache/symbols.go#L72
func (sc *LSPServiceClientBase) GetAllDeclarations(ctx context.Context, workspaceFolders []string, query string) []protocol.WorkspaceSymbol {
	// TODO(jsussman) Should we change protocol.WorkspaceSymbol to
	// protocol.SymbolInformation?

	var symbols []protocol.WorkspaceSymbol

	regex, regexErr := regexp.Compile(query)

	// Client may or may not support the "workspace/symbol" method, so we must
	// check before calling.

	if sc.ServerCapabilities.Supports("workspace/symbol") {
		params := protocol.WorkspaceSymbolParams{
			Query: query,
		}

		err := sc.Conn.Call(ctx, "workspace/symbol", params).Await(ctx, &symbols)
		if err != nil {
			fmt.Printf("error: %v\n", err)
		}
	}

	if regexErr != nil {
		// Not a valid regex, can't do anything more
		return symbols
	}

	// if p.capabilities.Supports("workspace/symbol") && len(symbols) == 0 {
	// 	// Run empty string query and manually search using the query as a regex
	// 	var allSymbols []protocol.WorkspaceSymbol
	// 	err = p.rpc.Call(ctx, "workspace/symbol", &protocol.WorkspaceSymbolParams{Query: ""}, &allSymbols)
	// 	if err != nil {
	// 		fmt.Printf("error: %v\n", err)
	// 	}
	// 	for _, s := range allSymbols {
	// 		if regex.MatchString(s.Name) {
	// 			symbols = append(symbols, s)
	// 		}
	// 	}
	// }

	if sc.ServerCapabilities.Supports("textDocument/definition") && len(symbols) == 0 {
		// if p.capabilities.Supports("textDocument/declaration") && len(symbols) == 0 {
		var positions []protocol.TextDocumentPositionParams
		symbolMap := make(map[string]protocol.WorkspaceSymbol) // To avoid repeats

		// Fallback to manually searching for an occurrence and performing a
		// GotoDefinition call

		// Lambda function to support switch to workspace folders
		walkFiles := func(locations []string) error {
			for _, location := range locations {
				location = strings.TrimPrefix(location, "file://")

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

		err := walkFiles(workspaceFolders)
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
			err := sc.Conn.Call(ctx, "textDocument/definition", position).Await(ctx, &res)
			// err := p.rpc.Call(ctx, "textDocument/declaration", position, &res)
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

func (sc *LSPServiceClientBase) GetAllReferences(ctx context.Context, location protocol.Location) []protocol.Location {
	params := &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: location.URI,
			},
			Position: location.Range.Start,
		},
		Context: protocol.ReferenceContext{
			IncludeDeclaration: false,
		},
	}

	res := []protocol.Location{}
	err := sc.Conn.Call(ctx, "textDocument/references", params).Await(ctx, &res)
	if err != nil {
		fmt.Printf("Error rpc: %v", err)
	}

	return res
}

// ---

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

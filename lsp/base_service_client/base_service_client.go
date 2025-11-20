package base

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	provider.Capability
	Fn interface{}
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
	Conn   provider.RPCClient
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

	// symbolCache is used to cache the document symbols for the workspace.
	symbolCache *SymbolCache
	// symbolSearchHelper is used to provide logic to work with symbol cache of the generic provider.
	symbolSearchHelper SymbolSearchHelper
	// symbolCacheUpdateChan is a channel to send file URIs to update the symbol cache.
	symbolCacheUpdateChan chan uri.URI
	// symbolCacheUpdateWaitGroup wait group to wait for all symbol cache updates to complete.
	symbolCacheUpdateWaitGroup sync.WaitGroup
	// allConditions list of all conditions we are mainting symbols for
	allConditions []provider.ConditionsByCap

	ServerCapabilities protocol.ServerCapabilities
	ServerInfo         *protocol.PServerInfoMsg_initialize

	TempDir string
	handler jsonrpc2.Handler
}

// SymbolSearchHelper is used by the generic service client to work with symbols
// Each language client using generic service client can change the search logic
// There are two stages where helper functions are called:
// 1. Prepare() - To prepare symbol cache ahead of time
// 2. Evaluate() - To perform the actual matching of symbols in the cache
type SymbolSearchHelper interface {
	// GetDocumentUris given a set of queries, this function should return the final
	// list of document URIs to search symbols in. The search will be made using a
	// combination of text search and textDocument/documentSymbol requests.
	GetDocumentUris(quries ...provider.ConditionsByCap) []uri.URI
	// GetLanguageID returns the language ID for a given URI. Required in didOpen() notification
	GetLanguageID(uri string) string

	// MatchFileContentByConditions given a content and list of all conditions available for the provider
	// returns the positions of the matched queries in the content. Used in Prepare() to find
	// all locations in a file which match any of our conditions.
	MatchFileContentByConditions(content string, queries ...provider.ConditionsByCap) [][]int
	// MatchSymbolByConditions given a workspace symbol and a list of all conditions available
	// returns true if symbol matches any of the conditions. Used in Prepare() to determine which
	// symbols we should be storing in the symbol cache.
	MatchSymbolByConditions(symbol WorkspaceSymbolDefinitionsPair, conditions ...provider.ConditionsByCap) bool

	// MatchSymbolByPatterns is used to determine if a symbol matches either one of the queries.
	// This is so that different languages can have different FQN semantics to match. Used in Evaluate().
	MatchSymbolByPatterns(symbol WorkspaceSymbolDefinitionsPair, patterns ...string) bool
}

func NewLSPServiceClientBase(
	ctx context.Context, log logr.Logger, c provider.InitConfig,
	initializeHandler jsonrpc2.Handler,
	initializeParams protocol.InitializeParams,
	symbolCacheHelper SymbolSearchHelper,
) (*LSPServiceClientBase, error) {
	sc := LSPServiceClientBase{}

	// Load the provider / service client specific config. Transforming from
	// map[string]any -> yaml string -> ServiceClient
	b, _ := yaml.Marshal(c.ProviderSpecificConfig)
	err := yaml.Unmarshal(b, &sc.BaseConfig)
	if err != nil {
		return nil, fmt.Errorf("base config unmarshal error: %w", err)
	}

	if sc.BaseConfig.LspServerPath == "" && c.RPC == nil {
		return nil, fmt.Errorf("must provide lspServerPath when RPC connection is not provided")
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

	sc.handler = NewChainHandler(initializeHandler)
	if c.RPC == nil {
		// launch the lsp command
		sc.Dialer, err = NewCmdDialer(
			sc.Ctx, sc.BaseConfig.LspServerPath, sc.BaseConfig.LspServerArgs...,
		)
		if err != nil {
			return nil, fmt.Errorf("new cmd dialer error: %w", err)
		}
		sc.Conn, err = jsonrpc2.Dial(
			sc.Ctx, sc.Dialer, jsonrpc2.ConnectionOptions{
				Handler: &sc,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("jsonrpc2.Dial error: %w", err)
		}
		time.Sleep(5 * time.Second)

	} else {
		sc.Log.Info("using provided connection", "conn", c.RPC)
		sc.Conn = c.RPC
		sc.ServerCapabilities = protocol.ServerCapabilities{
			AssumeWorks: true,
		}
	}
	// Create the caches for the various handler stuffs
	sc.PublishDiagnosticsCache = NewAwaitCache[string, []protocol.Diagnostic]()
	sc.symbolCache = NewDocumentSymbolCache()
	sc.symbolSearchHelper = symbolCacheHelper
	if sc.symbolSearchHelper == nil {
		sc.symbolSearchHelper = NewDefaultSymbolCacheHelper(sc.Log, c)
	}
	sc.symbolCacheUpdateChan = make(chan uri.URI, 10)
	go sc.symbolCacheUpdateHandler()
	// Create a connection to the lsp server
	if !c.Initialized {
		var result json.RawMessage
		err = sc.Conn.Call(sc.Ctx, "initialize", initializeParams).Await(sc.Ctx, &result)
		if err != nil {
			b, _ := json.Marshal(initializeParams)
			return nil, fmt.Errorf("initialize request error: %w, result: %s, initializeParams: %s, Dialer: %v", err, string(result), string(b), sc.Dialer)
		}

		sc.Log.V(7).Info(fmt.Sprintf("%s\n", string(result)))

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
		sc.Log.Info("provider connection initialized\n")
	}

	sc.Log.V(2).Info("provider connection established\n")

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

// NotifyFileChanges when a workspace file is modified, we invalidate the previous symbols we stored in the cache and query new symbols
func (sc *LSPServiceClientBase) NotifyFileChanges(ctx context.Context, changes ...provider.FileChange) error {
	if sc.symbolCache == nil {
		return nil
	}
	for _, change := range changes {
		if change.Path == "" {
			continue
		}
		fileURI, err := toURI(change.Path)
		if err != nil {
			sc.Log.Error(err, "unable to parse file change path", "path", change.Path)
			continue
		}
		sc.symbolCache.Invalidate(fileURI)
		sc.symbolCacheUpdateWaitGroup.Add(1)
		sc.symbolCacheUpdateChan <- fileURI
	}
	return nil
}

// Prepare is called before Evaluate() with all rules. We prepare the symbol cache during this step.
func (sc *LSPServiceClientBase) Prepare(ctx context.Context, conditionsByCap []provider.ConditionsByCap) error {
	sc.allConditions = conditionsByCap
	sc.symbolCacheUpdateWaitGroup.Add(1)
	go func() {
		defer sc.symbolCacheUpdateWaitGroup.Done()
		uris := sc.symbolSearchHelper.GetDocumentUris(conditionsByCap...)
		sc.symbolCacheUpdateWaitGroup.Add(len(uris))
		for _, uri := range uris {
			sc.symbolCacheUpdateChan <- uri
		}
	}()
	return nil
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

	if sc.handler != nil {
		res, err := sc.handler.Handle(ctx, req)
		if err == nil {
			return res, err
		}
	}

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
func (sc *LSPServiceClientBase) GetAllDeclarations(ctx context.Context, query string) []protocol.WorkspaceSymbol {
	var symbols []protocol.WorkspaceSymbol

	// prefer actual workspace/symbol request if supported
	if sc.ServerCapabilities.Supports("workspace/symbol") {
		params := protocol.WorkspaceSymbolParams{
			Query: query,
		}

		if err := sc.Conn.Call(ctx, "workspace/symbol", params).Await(ctx, &symbols); err != nil {
			sc.Log.Error(err, "workspace/symbol request failed", "query", query)
		}

		if len(symbols) > 0 {
			return symbols
		}
	}

	// wait until pending symbol cache update calls are complete
	sc.symbolCacheUpdateWaitGroup.Wait()

	symbolsDefinitionPairs := sc.symbolCache.GetAllWorkspaceSymbols()

	filteredSymbols := []protocol.WorkspaceSymbol{}
	if sc.symbolSearchHelper != nil {
		for _, symbol := range symbolsDefinitionPairs {
			if sc.symbolSearchHelper.MatchSymbolByPatterns(symbol, query) {
				filteredSymbols = append(filteredSymbols, symbol.WorkspaceSymbol)
			}
		}
	}

	return filteredSymbols
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
	if err := sc.Conn.Call(ctx, "textDocument/references", params).Await(ctx, &res); err != nil {
		fmt.Printf("Error rpc: %v", err)
	}

	return res
}

// populateDocumentSymbolCache is called to populate the document symbol cache for a given set of URIs
// For each URI, we perform a text search to find all positions that match *any* of the conditions passed to Prepare()
// For each position found, we perform a textDocument/definition request to find the symbol's definition
// For each definition, we perform textDocument/documentSymbol on the URI to get actual symbols in that file
// We then find out the actual symbol for that definition by looking up the symbol tree of that file
// Finally, we store the original match found as well as the definition as workspace symbols in the cache
// This info is later used in EvaluateReferenced() to search symbols for a query
func (sc *LSPServiceClientBase) populateDocumentSymbolCache(ctx context.Context, uris []uri.URI) {
	if sc.symbolCache == nil {
		return
	}

	queryDocumentSymbol := func(ctx context.Context, uri uri.URI, content []byte) ([]protocol.DocumentSymbol, error) {
		if symbols, exists := sc.symbolCache.GetDocumentSymbols(uri); exists {
			return symbols, nil
		}

		var symbols []struct {
			protocol.DocumentSymbol
			Location *protocol.Location `json:"location,omitempty"`
		}
		params := protocol.DocumentSymbolParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: string(uri),
			},
		}
		if err := sc.didOpen(ctx, string(uri), sc.symbolSearchHelper.GetLanguageID(string(uri)), content); err != nil {
			sc.Log.Error(err, "didOpen request failed", "uri", uri)
		}
		if err := sc.Conn.Call(ctx, "textDocument/documentSymbol", params).Await(ctx, &symbols); err != nil {
			return nil, err
		}
		if err := sc.didClose(ctx, string(uri)); err != nil {
			sc.Log.Error(err, "didClose request failed", "uri", uri)
		}
		documentSymbols := []protocol.DocumentSymbol{}
		for _, symbol := range symbols {
			if symbol.Location != nil {
				symbol.DocumentSymbol.Range = symbol.Location.Range
			}
			documentSymbols = append(documentSymbols, symbol.DocumentSymbol)
		}
		sc.symbolCache.SetDocumentSymbols(uri, documentSymbols)
		return documentSymbols, nil
	}

	workspaceSymbolKey := func(symbol protocol.WorkspaceSymbol) string {
		return fmt.Sprintf("%s:%d:%d", symbol.Location.Value.(protocol.Location).URI, symbol.Location.Value.(protocol.Location).Range.Start.Line, symbol.Location.Value.(protocol.Location).Range.Start.Character)
	}

	for _, fileURI := range uris {
		if ctx.Err() != nil {
			return
		}
		if _, exists := sc.symbolCache.GetWorkspaceSymbols(fileURI); exists {
			continue
		}
		workspaceSymbols := map[string]WorkspaceSymbolDefinitionsPair{}
		content, err := os.ReadFile(fileURI.Filename())
		if err != nil {
			sc.Log.Error(err, "unable to read file", "uri", fileURI)
			continue
		}
		// perform a text search to find all positions in the doc that match rules
		// for each position, get definition of the symbol at that position. From
		// the found definitions, store the actual position where text match as
		// workspace symbol and store any definitions found for that symbol. If a
		// definition is found, the symbol will be used as a reference symbol.
		positions := sc.getMatchingPositions(string(content), fileURI.Filename())
		for _, position := range positions {
			definitions := sc.getDefinitionForPosition(ctx, fileURI, position)
			wsSymbolsForDefinitions := map[string]protocol.WorkspaceSymbol{}
			for _, definition := range definitions {
				uri, err := toURI(definition.URI)
				if err != nil {
					sc.Log.Error(err, "unable to parse definition URI", "uri", definition.URI)
					continue
				}
				content, err := os.ReadFile(uri.Filename())
				if err != nil {
					sc.Log.Error(err, "unable to read file", "uri", uri)
					continue
				}
				documentSymbols, err := queryDocumentSymbol(ctx, uri, content)
				if err != nil {
					sc.Log.Error(err, "documentSymbol request failed", "uri", uri)
					continue
				}
				for _, symbol := range findDocumentSymbolsAtLocation(uri, documentSymbols, definition) {
					wsSymbolsForDefinitions[workspaceSymbolKey(symbol)] = symbol
				}
			}
			definitionSymbols := []protocol.WorkspaceSymbol{}
			for _, symbol := range wsSymbolsForDefinitions {
				definitionSymbols = append(definitionSymbols, symbol)
			}
			// attach all definitions found with the original match, so that
			// we can determine it as a referenced symbol
			pair := WorkspaceSymbolDefinitionsPair{
				WorkspaceSymbol: protocol.WorkspaceSymbol{
					Location: protocol.OrPLocation_workspace_symbol{
						Value: protocol.Location{
							URI: protocol.DocumentURI(fileURI),
							Range: protocol.Range{
								Start: position.Position,
								End:   position.Position,
							},
						},
					},
				},
				Definitions: definitionSymbols,
			}
			workspaceSymbols[workspaceSymbolKey(pair.WorkspaceSymbol)] = pair
		}
		workspaceSymbolsList := []WorkspaceSymbolDefinitionsPair{}
		for _, pair := range workspaceSymbols {
			workspaceSymbolsList = append(workspaceSymbolsList, pair)
		}
		sc.symbolCache.SetWorkspaceSymbols(fileURI, workspaceSymbolsList)
	}
}

func (sc *LSPServiceClientBase) symbolCacheUpdateHandler() {
	if sc.symbolCacheUpdateChan == nil {
		return
	}
	for {
		select {
		case <-sc.Ctx.Done():
			sc.drainPendingSymbolCacheUpdates()
			return
		case fileURI := <-sc.symbolCacheUpdateChan:
			sc.processSymbolCacheUpdate(fileURI)
		}
	}
}

func (sc *LSPServiceClientBase) processSymbolCacheUpdate(fileURI uri.URI) {
	defer sc.symbolCacheUpdateWaitGroup.Done()
	if sc.symbolCache == nil || fileURI == "" {
		return
	}
	filename := fileURI.Filename()
	if filename == "" {
		return
	}
	if _, err := os.Stat(filename); err != nil {
		if os.IsNotExist(err) {
			sc.Log.V(5).Info("skipping symbol cache update; file does not exist", "uri", fileURI)
			return
		}
		sc.Log.Error(err, "unable to stat file for symbol cache update", "uri", fileURI)
		return
	}
	sc.populateDocumentSymbolCache(sc.Ctx, []uri.URI{fileURI})
}

func (sc *LSPServiceClientBase) drainPendingSymbolCacheUpdates() {
	for {
		select {
		case fileURI := <-sc.symbolCacheUpdateChan:
			sc.symbolCacheUpdateWaitGroup.Done()
			sc.Log.V(6).Info("dropping pending symbol cache update", "uri", fileURI)
		default:
			return
		}
	}
}

func (sc *LSPServiceClientBase) didOpen(ctx context.Context, uri string, langID string, text []byte) error {
	params := protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        uri,
			LanguageID: langID,
			Version:    0,
			Text:       string(text),
		},
	}
	// typescript server seems to throw "No project" error without notification
	// perhaps there's a better way to do this
	return sc.Conn.Notify(ctx, "textDocument/didOpen", params)
}

func (sc *LSPServiceClientBase) didClose(ctx context.Context, uri string) error {
	params := protocol.DidCloseTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{
			URI: uri,
		},
	}
	return sc.Conn.Notify(ctx, "textDocument/didClose", params)
}

func toURI(path string) (uri.URI, error) {
	if strings.HasPrefix(path, "file://") {
		return uri.Parse(path)
	}

	absPath := path
	if !filepath.IsAbs(absPath) {
		var err error
		absPath, err = filepath.Abs(absPath)
		if err != nil {
			return "", err
		}
	}

	return uri.File(absPath), nil
}

func (sc *LSPServiceClientBase) getMatchingPositions(content string, path string) []protocol.TextDocumentPositionParams {
	positions := []protocol.TextDocumentPositionParams{}
	if sc.symbolSearchHelper != nil {
		scanner := bufio.NewScanner(strings.NewReader(string(content)))
		lineNumber := 0
		for scanner.Scan() {
			matchLocations := sc.symbolSearchHelper.MatchFileContentByConditions(scanner.Text(), sc.allConditions...)
			for _, loc := range matchLocations {
				absPath, err := filepath.Abs(path)
				if err != nil {
					return positions
				}
				positions = append(positions, protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{
						URI: string(uri.File(absPath)),
					},
					Position: protocol.Position{
						Line:      uint32(lineNumber),
						Character: uint32(loc[1]),
					},
				})
			}
			lineNumber++
		}
	}
	return positions
}

func (sc *LSPServiceClientBase) getDefinitionForPosition(ctx context.Context, uri uri.URI, position protocol.TextDocumentPositionParams) []protocol.Location {
	res := []protocol.Location{}
	if sc.ServerCapabilities.Supports("textDocument/definition") {
		content, err := os.ReadFile(uri.Filename())
		if err != nil {
			sc.Log.Error(err, "unable to read file", "uri", uri)
			return res
		}
		if err := sc.didOpen(ctx, string(position.TextDocument.URI), sc.symbolSearchHelper.GetLanguageID(string(position.TextDocument.URI)), content); err != nil {
			sc.Log.Error(err, "didOpen request failed", "uri", position.TextDocument.URI)
		}
		if err := sc.Conn.Call(ctx, "textDocument/definition", position).Await(ctx, &res); err != nil {
			sc.Log.Error(err, "textDocument/definition request failed", "position", position)
		}
		if err := sc.didClose(ctx, string(position.TextDocument.URI)); err != nil {
			sc.Log.Error(err, "didClose request failed", "uri", position.TextDocument.URI)
		}
	}
	return res
}

func findDocumentSymbolsAtLocation(docURI uri.URI, symbols []protocol.DocumentSymbol, defLoc protocol.Location) []protocol.WorkspaceSymbol {
	var out []protocol.WorkspaceSymbol
	findSymbolsAtLocationRecursive(docURI, symbols, defLoc, "", &out)
	return out
}

func findSymbolsAtLocationRecursive(docURI uri.URI, symbols []protocol.DocumentSymbol, defLoc protocol.Location, containerName string, out *[]protocol.WorkspaceSymbol) {
	for _, symbol := range symbols {
		symRange := preferredRange(symbol)
		if rangeOverlaps(symRange, defLoc.Range) {
			ws := protocol.WorkspaceSymbol{
				BaseSymbolInformation: protocol.BaseSymbolInformation{
					Name:          symbol.Name,
					Kind:          symbol.Kind,
					Tags:          symbol.Tags,
					ContainerName: containerName,
				},
				Location: protocol.OrPLocation_workspace_symbol{
					Value: protocol.Location{
						URI:   protocol.DocumentURI(docURI),
						Range: symRange,
					},
				},
			}
			*out = append(*out, ws)
		}
		// Traverse children
		if len(symbol.Children) > 0 {
			findSymbolsAtLocationRecursive(docURI, symbol.Children, defLoc, symbol.Name, out)
		}
	}
}

func rangeOverlaps(r1, r2 protocol.Range) bool {
	start1 := r1.Start
	end1 := r1.End
	start2 := r2.Start
	end2 := r2.End

	if positionLessEqual(start1, end2) && positionLessEqual(start2, end1) {
		return true
	}
	return false
}

func positionLessEqual(p1, p2 protocol.Position) bool {
	if p1.Line < p2.Line {
		return true
	} else if p1.Line == p2.Line {
		return p1.Character <= p2.Character
	}
	return false
}

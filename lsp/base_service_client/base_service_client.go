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
	"sync/atomic"
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
	openedFiles                map[uri.URI]bool
	openedFilesMutex           sync.Mutex
	// allConditions list of all conditions we are mainting symbols for
	allConditions      []provider.ConditionsByCap
	allConditionsMutex sync.RWMutex

	ServerCapabilities protocol.ServerCapabilities
	ServerInfo         *protocol.PServerInfoMsg_initialize

	TempDir string
	handler jsonrpc2.Handler

	// Progress reporting for Prepare() phase
	progressReporter     provider.PrepareProgressReporter
	totalFilesToProcess  int32
	filesProcessed       atomic.Int32
	lastProgressReport   time.Time
	progressMutex        sync.Mutex

	// Progress event streaming for GRPC providers
	progressEventChan    chan *provider.PrepareProgressEvent
	progressStreamActive atomic.Bool
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
	GetLanguageID(uri uri.URI) string

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

	// Populate WorkspaceFolders from initialize params
	if len(initializeParams.WorkspaceFolders) > 0 {
		for _, folder := range initializeParams.WorkspaceFolders {
			sc.BaseConfig.WorkspaceFolders = append(sc.BaseConfig.WorkspaceFolders, folder.URI)
		}
	} else if initializeParams.RootURI != "" {
		sc.BaseConfig.WorkspaceFolders = append(sc.BaseConfig.WorkspaceFolders, initializeParams.RootURI)
	}

	if initializeParams.ProcessID == 0 {
		initializeParams.ProcessID = int32(os.Getpid())
	}

	// Create the ctx, cancelFunc, and log
	sc.Ctx, sc.CancelFunc = context.WithCancel(ctx)
	sc.Log = log.WithValues("provider", sc.BaseConfig.LspServerName)

	// Store progress reporter if provided
	sc.progressReporter = c.PrepareProgressReporter

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
	sc.openedFilesMutex.Lock()
	sc.openedFiles = make(map[uri.URI]bool)
	sc.openedFilesMutex.Unlock()

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
		if err := sc.didClose(ctx, fileURI); err != nil {
			sc.Log.Error(err, "didClose request failed", "uri", fileURI)
		}
		sc.symbolCache.Invalidate(fileURI)
		sc.symbolCacheUpdateWaitGroup.Add(1)
		sc.symbolCacheUpdateChan <- fileURI
	}
	return nil
}

// Prepare is called before Evaluate() with all rules. We prepare the symbol cache during this step.
func (sc *LSPServiceClientBase) Prepare(ctx context.Context, conditionsByCap []provider.ConditionsByCap) error {
	sc.allConditionsMutex.Lock()
	sc.allConditions = conditionsByCap
	sc.allConditionsMutex.Unlock()
	sc.symbolCacheUpdateWaitGroup.Add(1)
	go func() {
		defer sc.symbolCacheUpdateWaitGroup.Done()
		uris := sc.symbolSearchHelper.GetDocumentUris(conditionsByCap...)

		// Initialize progress tracking
		sc.filesProcessed.Store(0)
		atomic.StoreInt32(&sc.totalFilesToProcess, int32(len(uris)))
		sc.lastProgressReport = time.Now()

		sc.symbolCacheUpdateWaitGroup.Add(len(uris))
		for _, uri := range uris {
			sc.symbolCacheUpdateChan <- uri
		}
	}()

	// Wait for all file processing to complete
	sc.symbolCacheUpdateWaitGroup.Wait()

	// Close progress stream if it was started
	// This signals to the server that no more progress events will be sent
	if sc.progressStreamActive.Load() {
		sc.StopProgressStream()
	}

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
	if len(sc.BaseConfig.WorkspaceFolders) == 0 {
		return nil, fmt.Errorf("no workspace folders configured")
	}
	// Expects dependency provider to output provider.Dep structs to stdout
	cmd := exec.Command(cmdStr)
	workspaceURI := sc.BaseConfig.WorkspaceFolders[0]
	// Remove file:// prefix if present
	if strings.HasPrefix(workspaceURI, "file://") {
		cmd.Dir = workspaceURI[7:]
	} else {
		cmd.Dir = workspaceURI
	}
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
func (sc *LSPServiceClientBase) GetAllDeclarations(ctx context.Context, query string, useWorkspaceSymbol bool) []protocol.WorkspaceSymbol {
	var symbols []protocol.WorkspaceSymbol

	// prefer actual workspace/symbol request if supported
	if useWorkspaceSymbol && sc.ServerCapabilities.Supports("workspace/symbol") {
		params := protocol.WorkspaceSymbolParams{
			Query: query,
		}

		if err := sc.Conn.Call(ctx, "workspace/symbol", params).Await(ctx, &symbols); err != nil {
			sc.Log.Error(err, "workspace/symbol request failed", "query", query)
		}

		if len(symbols) > 0 {
			sc.Log.V(7).Info("workspace/symbol request returned symbols", "totalSymbols", len(symbols), "query", query)
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
	sc.Log.V(7).Info("Returning symbols from symbol cache", "totalSymbols", len(filteredSymbols), "query", query)
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
		sc.Log.Error(err, "textDocument/references request failed", "uri", location.URI)
		return nil
	}
	sc.Log.V(7).Info("textDocument/references request returned locations", "totalLocations", len(res), "uri", location.URI)
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

	workspaceSymbolKey := func(symbol protocol.WorkspaceSymbol) string {
		return fmt.Sprintf("%s:%d:%d",
			symbol.Location.Value.(protocol.Location).URI,
			symbol.Location.Value.(protocol.Location).Range.Start.Line,
			symbol.Location.Value.(protocol.Location).Range.Start.Character)
	}

	for _, fileURI := range uris {
		if ctx.Err() != nil {
			return
		}
		if _, exists := sc.symbolCache.GetWorkspaceSymbols(fileURI); exists {
			sc.Log.V(9).Info("Skipping symbol cache update; symbols already exist for file", "uri", fileURI)
			continue
		}
		workspaceSymbols := map[string]WorkspaceSymbolDefinitionsPair{}
		content, err := os.ReadFile(fileURI.Filename())
		if err != nil {
			sc.Log.Error(err, "unable to read file", "uri", fileURI)
			continue
		}
		// perform a text search to find all matchedSymbols in the doc that match rules
		// for each position, get definition of the symbol at that position. From
		// the found definitions, store the actual position where text match as
		// workspace symbol and store any definitions found for that symbol. If a
		// definition is found, the symbol will be used as a reference symbol.
		matchedSymbols := sc.searchContentForWorkspaceSymbols(ctx, string(content), fileURI)
		sc.Log.V(9).Info("Found matched symbol by text search", "totalMatchedSymbols", len(matchedSymbols), "uri", fileURI)

		// get definitions for matched symbols in parallel
		type defResult struct {
			matchedSymbol protocol.WorkspaceSymbol
			definitions   []protocol.Location
		}
		results := make(chan defResult, len(matchedSymbols))
		var wg sync.WaitGroup
		sem := make(chan struct{}, 10) // limit concurrency to 10
		for _, matchedSymbol := range matchedSymbols {
			wg.Add(1)
			go func(ms protocol.WorkspaceSymbol) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				location, ok := ms.Location.Value.(protocol.Location)
				if !ok {
					sc.Log.V(7).Info("unable to get location from workspace symbol", "workspace symbol", ms)
					return
				}
				defs := sc.getDefinitionForPosition(ctx, fileURI, location)
				if len(defs) > 0 {
					results <- defResult{matchedSymbol: ms, definitions: defs}
				}
			}(matchedSymbol)
		}
		go func() {
			wg.Wait()
			close(results)
		}()

		for res := range results {
			matchedSymbol := res.matchedSymbol
			definitions := res.definitions
			sc.Log.V(11).Info("Found definitions for matched symbol", "totalDefinitions", len(definitions), "uri", fileURI, "matchedSymbol", matchedSymbol)
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
				documentSymbols, err := sc.queryDocumentSymbol(ctx, uri, content)
				if err != nil {
					sc.Log.Error(err, "documentSymbol request failed", "uri", uri)
					continue
				}
				wsForDefinition := protocol.WorkspaceSymbol{
					Location: protocol.OrPLocation_workspace_symbol{
						Value: definition,
					},
					BaseSymbolInformation: protocol.BaseSymbolInformation{
						Name: matchedSymbol.Name,
					},
				}
				if symbol, ok := sc.findDocumentSymbolAtLocation(uri, documentSymbols, wsForDefinition); ok {
					wsSymbolsForDefinitions[workspaceSymbolKey(symbol)] = symbol
				} else {
					wsSymbolsForDefinitions[workspaceSymbolKey(wsForDefinition)] = wsForDefinition
				}
			}
			definitionSymbols := []protocol.WorkspaceSymbol{}
			for _, symbol := range wsSymbolsForDefinitions {
				definitionSymbols = append(definitionSymbols, symbol)
			}
			// attach all definitions found with the original match
			pair := WorkspaceSymbolDefinitionsPair{
				WorkspaceSymbol: protocol.WorkspaceSymbol{
					Location: protocol.OrPLocation_workspace_symbol{
						Value: protocol.Location{
							URI:   protocol.DocumentURI(fileURI),
							Range: matchedSymbol.Location.Value.(protocol.Location).Range,
						},
					},
					BaseSymbolInformation: protocol.BaseSymbolInformation{
						Name:          matchedSymbol.Name,
						Kind:          matchedSymbol.Kind,
						Tags:          matchedSymbol.Tags,
						ContainerName: matchedSymbol.ContainerName,
					},
				},
				Definitions: definitionSymbols,
			}
			sc.Log.V(11).Info("Preparing workspace symbol pair", "workspaceSymbol", pair.WorkspaceSymbol, "uri", fileURI, "definitions", pair.Definitions)
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
	defer sc.reportProgress()

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

// reportProgress reports progress to the progress reporter if one is configured.
// It throttles updates to avoid excessive reporting (max once per 500ms).
// It also sends events to the progress stream channel if streaming is active.
func (sc *LSPServiceClientBase) reportProgress() {
	// Increment files processed counter
	processed := sc.filesProcessed.Add(1)
	total := atomic.LoadInt32(&sc.totalFilesToProcess)

	// Throttle progress updates to once per 500ms
	sc.progressMutex.Lock()
	now := time.Now()
	timeSinceLastReport := now.Sub(sc.lastProgressReport)

	// Always report on first file, last file, or if 500ms have elapsed
	shouldReport := processed == 1 || processed == total || timeSinceLastReport >= 500*time.Millisecond

	if shouldReport {
		sc.lastProgressReport = now
		sc.progressMutex.Unlock()

		// Call the progress reporter if configured (in-process)
		if sc.progressReporter != nil {
			sc.progressReporter.ReportProgress(
				sc.BaseConfig.LspServerName,
				int(processed),
				int(total),
			)
		}

		// Send to progress stream channel if streaming is active (GRPC)
		if sc.progressStreamActive.Load() && sc.progressEventChan != nil {
			event := &provider.PrepareProgressEvent{
				ProviderName:   sc.BaseConfig.LspServerName,
				FilesProcessed: int(processed),
				TotalFiles:     int(total),
			}
			// Non-blocking send to avoid deadlocks
			select {
			case sc.progressEventChan <- event:
			default:
				// Channel full or closed, skip this event
			}
		}
	} else {
		sc.progressMutex.Unlock()
	}
}

// StartProgressStream creates and returns a channel for streaming progress events.
// This is used by GRPC providers to stream progress back to the client.
// The returned channel will receive progress events during Prepare() phase.
// The caller should close the channel when done by calling StopProgressStream().
func (sc *LSPServiceClientBase) StartProgressStream() <-chan *provider.PrepareProgressEvent {
	sc.progressEventChan = make(chan *provider.PrepareProgressEvent, 100)
	sc.progressStreamActive.Store(true)
	return sc.progressEventChan
}

// StopProgressStream stops the progress stream and closes the channel.
func (sc *LSPServiceClientBase) StopProgressStream() {
	sc.progressStreamActive.Store(false)
	if sc.progressEventChan != nil {
		close(sc.progressEventChan)
		sc.progressEventChan = nil
	}
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

func (sc *LSPServiceClientBase) didOpen(ctx context.Context, uri uri.URI, text []byte) error {
	sc.openedFilesMutex.Lock()
	if _, exists := sc.openedFiles[uri]; exists {
		sc.openedFilesMutex.Unlock()
		return nil
	}
	sc.openedFiles[uri] = true
	sc.openedFilesMutex.Unlock()
	params := protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        string(uri),
			LanguageID: sc.symbolSearchHelper.GetLanguageID(uri),
			Version:    0,
			Text:       string(text),
		},
	}
	// typescript server seems to throw "No project" error without notification
	// perhaps there's a better way to do this
	return sc.Conn.Notify(ctx, "textDocument/didOpen", params)
}

func (sc *LSPServiceClientBase) didClose(ctx context.Context, uri uri.URI) error {
	sc.openedFilesMutex.Lock()
	if _, exists := sc.openedFiles[uri]; !exists {
		sc.openedFilesMutex.Unlock()
		return nil
	}
	delete(sc.openedFiles, uri)
	sc.openedFilesMutex.Unlock()
	params := protocol.DidCloseTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{
			URI: string(uri),
		},
	}
	return sc.Conn.Notify(ctx, "textDocument/didClose", params)
}

func (sc *LSPServiceClientBase) queryDocumentSymbol(ctx context.Context, uri uri.URI, content []byte) ([]protocol.DocumentSymbol, error) {
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
	if err := sc.didOpen(ctx, uri, content); err != nil {
		sc.Log.Error(err, "didOpen request failed", "uri", uri)
	}

	const maxAttempts = 2
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		symbols = nil
		err := sc.Conn.Call(ctx, "textDocument/documentSymbol", params).Await(ctx, &symbols)
		if err != nil {
			lastErr = err
		} else if len(symbols) == 0 {
			lastErr = fmt.Errorf("textDocument/documentSymbol returned zero symbols")
		} else {
			documentSymbols := make([]protocol.DocumentSymbol, 0, len(symbols))
			// typescript-language-server seems to return workspaceSymbol types even for document symbols
			// we need to normalize them back into document symbol types by copying the range
			for _, symbol := range symbols {
				if symbol.Location != nil {
					symbol.DocumentSymbol.Range = symbol.Location.Range
				}
				documentSymbols = append(documentSymbols, symbol.DocumentSymbol)
			}
			sc.symbolCache.SetDocumentSymbols(uri, documentSymbols)
			sc.Log.V(11).Info("Returning document symbols", "uri", uri, "totalDocumentSymbols", len(documentSymbols))
			return documentSymbols, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if attempt < maxAttempts {
			time.Sleep(100 * time.Millisecond)
		}
	}
	sc.Log.V(7).Info("textDocument/documentSymbol failed", "uri", uri, "error", lastErr)
	return nil, nil
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

func (sc *LSPServiceClientBase) searchContentForWorkspaceSymbols(ctx context.Context, content string, fileURI uri.URI) []protocol.WorkspaceSymbol {
	positions := []protocol.WorkspaceSymbol{}
	symbols := []protocol.DocumentSymbol{}
	var symbolsMutex sync.RWMutex

	if sc.symbolSearchHelper != nil {
		scanner := bufio.NewScanner(strings.NewReader(string(content)))
		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}

		type matchResult struct {
			locs       [][]int
			lineNumber int
		}
		results := make(chan matchResult, len(lines))
		var wg sync.WaitGroup
		sem := make(chan struct{}, 20)

		for i, line := range lines {
			wg.Add(1)
			go func(line string, lineNumber int) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				sc.allConditionsMutex.RLock()
				allConditions := sc.allConditions
				sc.allConditionsMutex.RUnlock()

				matchLocations := sc.symbolSearchHelper.MatchFileContentByConditions(line, allConditions...)
				if len(matchLocations) > 0 {
					results <- matchResult{locs: matchLocations, lineNumber: lineNumber}
				}
			}(line, i)
		}

		go func() {
			wg.Wait()
			close(results)
		}()

		dsCalled := false
		for res := range results {
			matchLocations := res.locs
			lineNumber := res.lineNumber

			matchLocationKey := func(loc []int) string {
				return fmt.Sprintf("%d:%d:%d", lineNumber, loc[0], loc[1])
			}
			dedupedMatchLocations := map[string]bool{}
			if len(matchLocations) > 0 {
				symbolsMutex.Lock()
				if !dsCalled {
					ds, err := sc.queryDocumentSymbol(ctx, fileURI, []byte(content))
					if err != nil {
						sc.Log.Error(err, "queryDocumentSymbol request failed", "uri", fileURI)
					}
					symbols = ds
					dsCalled = true
				}
				currentSymbols := symbols
				symbolsMutex.Unlock()

				for _, loc := range matchLocations {
					key := matchLocationKey(loc)
					if _, ok := dedupedMatchLocations[key]; ok {
						continue
					}
					dedupedMatchLocations[key] = true
					absPath, err := filepath.Abs(fileURI.Filename())
					if err != nil {
						sc.Log.Error(err, "unable to get absolute path for file", "uri", fileURI)
						continue
					}
					wsForMatch := protocol.WorkspaceSymbol{
						BaseSymbolInformation: protocol.BaseSymbolInformation{
							Name: lines[lineNumber][loc[0]:loc[1]],
						},
						Location: protocol.OrPLocation_workspace_symbol{
							Value: protocol.Location{
								URI: string(uri.File(absPath)),
								Range: protocol.Range{
									Start: protocol.Position{
										Line:      uint32(lineNumber),
										Character: uint32(loc[0]),
									},
									End: protocol.Position{
										Line:      uint32(lineNumber),
										Character: uint32(loc[1]),
									},
								},
							},
						},
					}
					if symbol, ok := sc.findDocumentSymbolAtLocation(fileURI, currentSymbols, wsForMatch); ok {
						positions = append(positions, symbol)
					} else {
						positions = append(positions, wsForMatch)
					}
				}
			}
		}
	}
	return positions
}

func (sc *LSPServiceClientBase) getDefinitionForPosition(ctx context.Context, uri uri.URI, location protocol.Location) []protocol.Location {
	unmarshalLocations := func(raw json.RawMessage) ([]protocol.Location, bool) {
		var links []protocol.LocationLink
		if err := json.Unmarshal(raw, &links); err == nil {
			if len(links) == 0 {
				return []protocol.Location{}, true
			}
			if links[0].TargetURI != "" {
				locs := make([]protocol.Location, len(links))
				for i, link := range links {
					locs[i] = protocol.Location{
						URI:   link.TargetURI,
						Range: link.TargetRange,
					}
				}
				return locs, true
			}
		}
		var loc protocol.Location
		if err := json.Unmarshal(raw, &loc); err == nil && loc.URI != "" {
			return []protocol.Location{loc}, true
		}
		var locs []protocol.Location
		if err := json.Unmarshal(raw, &locs); err == nil {
			return locs, true
		}
		return nil, false
	}
	if sc.ServerCapabilities.Supports("textDocument/definition") {
		position := protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: string(uri),
			},
			Position: location.Range.End,
		}
		content, err := os.ReadFile(uri.Filename())
		if err != nil {
			sc.Log.Error(err, "unable to read file for getting definitions", "uri", uri)
			return nil
		}
		if err := sc.didOpen(ctx, uri, content); err != nil {
			sc.Log.Error(err, "didOpen request failed", "uri", uri)
		}
		const maxAttempts = 2
		var lastErr error
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			var tmp json.RawMessage
			err := sc.Conn.Call(ctx, "textDocument/definition", position).Await(ctx, &tmp)
			if err != nil {
				lastErr = err
			} else if len(tmp) == 0 {
				lastErr = fmt.Errorf("textDocument/definition returned zero locations")
			} else {
				locations, ok := unmarshalLocations(tmp)
				if ok && len(locations) > 0 {
					return locations
				}
			}
			if ctx.Err() != nil {
				return nil
			}
			if attempt < maxAttempts {
				time.Sleep(100 * time.Millisecond)
			}
		}
		if lastErr != nil {
			sc.Log.Error(lastErr, "textDocument/definition request failed", "uri", uri)
		}
	}
	return nil
}

func (sc *LSPServiceClientBase) findDocumentSymbolAtLocation(docURI uri.URI, symbols []protocol.DocumentSymbol, defSymbol protocol.WorkspaceSymbol) (protocol.WorkspaceSymbol, bool) {
	var bestSymbol protocol.WorkspaceSymbol
	var bestLength uint64
	found := false

	var traverse func([]protocol.DocumentSymbol, string)
	traverse = func(symbols []protocol.DocumentSymbol, containerName string) {
		for _, symbol := range symbols {
			symRange := preferredRange(symbol)
			defLoc, ok := defSymbol.Location.Value.(protocol.Location)
			if !ok {
				continue
			}
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
			if rangeOverlaps(symRange, defLoc.Range) && sc.symbolSearchHelper.MatchSymbolByPatterns(WorkspaceSymbolDefinitionsPair{
				WorkspaceSymbol: ws,
			}, defSymbol.Name) {
				length := rangeLength(symRange)
				if !found || length < bestLength {
					bestSymbol = ws
					bestLength = length
					found = true
				}
			}
			if len(symbol.Children) > 0 {
				traverse(symbol.Children, symbol.Name)
			}
		}
	}

	traverse(symbols, "")
	return bestSymbol, found
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

func rangeLength(r protocol.Range) uint64 {
	lineDiff := int64(r.End.Line) - int64(r.Start.Line)
	if lineDiff < 0 {
		lineDiff = 0
	}
	charDiff := int64(r.End.Character) - int64(r.Start.Character)
	if charDiff < 0 {
		charDiff = 0
	}
	return (uint64(lineDiff) << 32) | uint64(charDiff)
}

package base

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
)

// DocumentSymbolCache stores document symbols keyed by URI and exposes helpers
// to retrieve flattened workspace symbols that match a regex. This will be used
// by the provider to cache the document symbol results for URIs.
type DocumentSymbolCache struct {
	mu     sync.RWMutex
	lookup map[uri.URI][]protocol.DocumentSymbol
}

func NewDocumentSymbolCache() *DocumentSymbolCache {
	return &DocumentSymbolCache{
		lookup: make(map[uri.URI][]protocol.DocumentSymbol),
	}
}

func (c *DocumentSymbolCache) Get(u uri.URI) ([]protocol.DocumentSymbol, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	symbols, ok := c.lookup[u]
	return symbols, ok
}

func (c *DocumentSymbolCache) Set(u uri.URI, symbols []protocol.DocumentSymbol) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lookup[u] = symbols
}

func (c *DocumentSymbolCache) Invalidate(u uri.URI) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.lookup, u)
}

func (c *DocumentSymbolCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lookup = make(map[uri.URI][]protocol.DocumentSymbol)
}

type SymbolMatcherFunc func(symbol protocol.WorkspaceSymbol, query ...string) bool

// GetAllWorkspaceSymbols returns workspace symbols from the cache
func (c *DocumentSymbolCache) GetAllWorkspaceSymbols() []protocol.WorkspaceSymbol {
	if c == nil {
		return nil
	}

	snapshot := c.snapshot()
	results := make([]protocol.WorkspaceSymbol, 0)

	for u, symbols := range snapshot {
		traverseDocumentSymbolTree(u, symbols, &results, "")
	}

	return results
}

func (c *DocumentSymbolCache) snapshot() map[uri.URI][]protocol.DocumentSymbol {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[uri.URI][]protocol.DocumentSymbol, len(c.lookup))
	for k, v := range c.lookup {
		result[k] = v
	}
	return result
}

func traverseDocumentSymbolTree(docURI uri.URI, symbols []protocol.DocumentSymbol, out *[]protocol.WorkspaceSymbol, containerName string) {
	for _, symbol := range symbols {
		*out = append(*out, protocol.WorkspaceSymbol{
			BaseSymbolInformation: protocol.BaseSymbolInformation{
				Name:          symbol.Name,
				Kind:          symbol.Kind,
				Tags:          symbol.Tags,
				ContainerName: containerName,
			},
			Location: protocol.OrPLocation_workspace_symbol{
				Value: protocol.Location{
					URI:   protocol.DocumentURI(docURI),
					Range: preferredRange(symbol),
				},
			},
		})
		if len(symbol.Children) > 0 {
			traverseDocumentSymbolTree(docURI, symbol.Children, out, symbol.Name)
		}
	}
}

func preferredRange(symbol protocol.DocumentSymbol) protocol.Range {
	if symbol.SelectionRange.Start.Line != 0 ||
		symbol.SelectionRange.Start.Character != 0 ||
		symbol.SelectionRange.End.Line != 0 ||
		symbol.SelectionRange.End.Character != 0 {
		return symbol.SelectionRange
	}
	return symbol.Range
}

type defaultSymbolCacheHelper struct {
	log    logr.Logger
	config provider.InitConfig
}

func (h *defaultSymbolCacheHelper) GetDocumentUris(conditionsByCap []provider.ConditionsByCap) []uri.URI {
	included, excluded := []string{}, []string{}
	for _, conditions := range conditionsByCap {
		if conditions.Cap == "referenced" {
			for _, condition := range conditions.Conditions {
				var cond ReferencedCondition
				err := yaml.Unmarshal(condition, &cond)
				if err != nil {
					h.log.Error(err, "error unmarshaling referenced condition")
					continue
				}
				condIncluded, condExcluded := cond.ProviderContext.GetScopedFilepaths()
				included = append(included, condIncluded...)
				excluded = append(excluded, condExcluded...)
			}
		}
	}
	searcher := provider.FileSearcher{
		BasePath:        h.config.Location,
		AdditionalPaths: h.config.ProviderSpecificConfig["workspaceFolders"].([]string),
		ProviderConfigConstraints: provider.IncludeExcludeConstraints{
			ExcludePathsOrPatterns: []string{
				filepath.Join(h.config.Location, "node_modules"),
				filepath.Join(h.config.Location, "vendor"),
				filepath.Join(h.config.Location, ".git"),
				filepath.Join(h.config.Location, "dist"),
				filepath.Join(h.config.Location, "build"),
				filepath.Join(h.config.Location, "target"),
				filepath.Join(h.config.Location, ".venv"),
				filepath.Join(h.config.Location, "venv"),
			},
		},
		RuleScopeConstraints: provider.IncludeExcludeConstraints{
			IncludePathsOrPatterns: included,
			ExcludePathsOrPatterns: excluded,
		},
	}
	files, err := searcher.Search(provider.SearchCriteria{})
	if err != nil {
		h.log.Error(err, "error searching for files")
		return nil
	}
	uris := []uri.URI{}
	for _, file := range files {
		uris = append(uris, uri.File(file))
	}
	return uris
}

func (h *defaultSymbolCacheHelper) GetLanguageID(uri string) string {
	languageIDMap := map[string]string{
		".ts":   "typescript",
		".js":   "javascript",
		".jsx":  "javascriptreact",
		".tsx":  "typescriptreact",
		".json": "json",
		".yaml": "yaml",
		".yml":  "yaml",
		".go":   "go",
	}
	return languageIDMap[filepath.Ext(filepath.Base(uri))]
}

func (h *defaultSymbolCacheHelper) MatchSymbol(symbol protocol.WorkspaceSymbol, query ...string) bool {
	for _, q := range query {
		regex, err := compileSymbolQueryRegex(q)
		if err != nil {
			h.log.Error(err, "error compiling symbol query regex")
			continue
		}
		if regex.MatchString(symbol.Name) {
			return true
		}
	}
	return false
}

func NewDefaultSymbolCacheHelper(log logr.Logger, config provider.InitConfig) SymbolCacheHelper {
	return &defaultSymbolCacheHelper{
		log:    log,
		config: config,
	}
}

func compileSymbolQueryRegex(query string) (*regexp.Regexp, error) {
	if query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}
	pattern := query
	if !hasRegexMeta(query) {
		pattern = "^" + regexp.QuoteMeta(query) + "$"
	}
	return regexp.Compile("(?i:" + pattern + ")")
}

func hasRegexMeta(value string) bool {
	for _, ch := range value {
		switch ch {
		case '.', '*', '?', '+', '[', ']', '(', ')', '{', '}', '|', '^', '$', '\\':
			return true
		}
	}
	return false
}

package base

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
)

// SymbolCache stores document symbols keyed by URI and exposes helpers
// to retrieve flattened workspace symbols that match a regex. This will be used
// by the provider to cache the document symbol results for URIs.
type SymbolCache struct {
	wsMutex sync.RWMutex
	dsMutex sync.RWMutex
	ws      map[uri.URI][]WorkspaceSymbolDefinitionsPair
	ds      map[uri.URI][]protocol.DocumentSymbol
}

type WorkspaceSymbolDefinitionsPair struct {
	WorkspaceSymbol protocol.WorkspaceSymbol
	Definitions     []protocol.WorkspaceSymbol
}

func NewDocumentSymbolCache() *SymbolCache {
	return &SymbolCache{
		ws: make(map[uri.URI][]WorkspaceSymbolDefinitionsPair),
		ds: make(map[uri.URI][]protocol.DocumentSymbol),
	}
}

func (c *SymbolCache) GetWorkspaceSymbols(u uri.URI) ([]WorkspaceSymbolDefinitionsPair, bool) {
	c.wsMutex.RLock()
	defer c.wsMutex.RUnlock()
	symbols, ok := c.ws[u]
	return symbols, ok
}

func (c *SymbolCache) SetWorkspaceSymbols(u uri.URI, symbols []WorkspaceSymbolDefinitionsPair) {
	c.wsMutex.Lock()
	defer c.wsMutex.Unlock()
	c.ws[u] = symbols
}

func (c *SymbolCache) GetDocumentSymbols(u uri.URI) ([]protocol.DocumentSymbol, bool) {
	c.dsMutex.RLock()
	defer c.dsMutex.RUnlock()
	symbols, ok := c.ds[u]
	return symbols, ok
}

func (c *SymbolCache) SetDocumentSymbols(u uri.URI, symbols []protocol.DocumentSymbol) {
	c.dsMutex.Lock()
	defer c.dsMutex.Unlock()
	c.ds[u] = symbols
}

func (c *SymbolCache) Invalidate(u uri.URI) {
	c.wsMutex.Lock()
	defer c.wsMutex.Unlock()
	delete(c.ws, u)
	c.dsMutex.Lock()
	defer c.dsMutex.Unlock()
	delete(c.ds, u)
}

func (c *SymbolCache) InvalidateAll() {
	c.wsMutex.Lock()
	defer c.wsMutex.Unlock()
	c.ws = make(map[uri.URI][]WorkspaceSymbolDefinitionsPair)
}

type SymbolMatcherFunc func(symbol protocol.WorkspaceSymbol, query ...string) bool

// GetAllWorkspaceSymbols returns workspace symbols from the cache
func (c *SymbolCache) GetAllWorkspaceSymbols() []WorkspaceSymbolDefinitionsPair {
	if c == nil {
		return nil
	}

	snapshot := c.snapshotWorkspaceSymbols()
	results := make([]WorkspaceSymbolDefinitionsPair, 0)

	for _, symbols := range snapshot {
		results = append(results, symbols...)
	}

	return results
}

func (c *SymbolCache) snapshotWorkspaceSymbols() map[uri.URI][]WorkspaceSymbolDefinitionsPair {
	c.wsMutex.RLock()
	defer c.wsMutex.RUnlock()

	result := make(map[uri.URI][]WorkspaceSymbolDefinitionsPair, len(c.ws))
	for k, v := range c.ws {
		result[k] = v
	}
	return result
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

// defaultSymbolSearchHelper is a default search implementation used for querying document symbols.
type defaultSymbolSearchHelper struct {
	log    logr.Logger
	config provider.InitConfig
}

// GetDocumentUris returns the URIs of all documents in workspace with some common excluded paths
func (h *defaultSymbolSearchHelper) GetDocumentUris(conditionsByCap ...provider.ConditionsByCap) []uri.URI {
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
	primaryPath := h.config.Location
	if after, ok := strings.CutPrefix(primaryPath, fmt.Sprintf("%s://", uri.FileScheme)); ok {
		primaryPath = after
	}
	additionalPaths := []string{}
	if val, ok := h.config.ProviderSpecificConfig["workspaceFolders"].([]string); ok {
		for _, path := range val {
			if after, prefixOk := strings.CutPrefix(path, fmt.Sprintf("%s://", uri.FileScheme)); prefixOk {
				path = after
			}
			if primaryPath == "" {
				primaryPath = path
				continue
			}
			additionalPaths = append(additionalPaths, path)
		}
	}
	searcher := provider.FileSearcher{
		BasePath:        primaryPath,
		AdditionalPaths: additionalPaths,
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

func (h *defaultSymbolSearchHelper) GetLanguageID(uri string) string {
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

func (h *defaultSymbolSearchHelper) MatchSymbolByPatterns(symbol WorkspaceSymbolDefinitionsPair, patterns ...string) bool {
	for _, p := range patterns {
		regex, err := compileSymbolQueryRegex(p)
		if err != nil {
			h.log.Error(err, "error compiling symbol query regex")
			continue
		}
		if regex.MatchString(symbol.WorkspaceSymbol.Name) {
			return true
		}
	}
	return false
}

func (h *defaultSymbolSearchHelper) MatchFileContentByConditions(content string, conditionsByCap ...provider.ConditionsByCap) [][]int {
	matches := [][]int{}
	for _, condition := range conditionsByCap {
		if condition.Cap == "referenced" {
			for _, condition := range condition.Conditions {
				var cond ReferencedCondition
				err := yaml.Unmarshal(condition, &cond)
				if err != nil {
					h.log.Error(err, "error unmarshaling referenced condition")
					continue
				}
				regex, err := compileSymbolQueryRegex(cond.Referenced.Pattern)
				if err != nil {
					h.log.Error(err, "error compiling symbol query regex")
					continue
				}
				matches = append(matches, regex.FindAllStringIndex(content, -1)...)
			}
		}
	}
	return matches
}

func (h *defaultSymbolSearchHelper) MatchSymbolByConditions(symbol WorkspaceSymbolDefinitionsPair, conditions ...provider.ConditionsByCap) bool {
	for _, condition := range conditions {
		if condition.Cap == "referenced" {
			for _, condition := range condition.Conditions {
				var cond ReferencedCondition
				err := yaml.Unmarshal(condition, &cond)
				if err != nil {
					h.log.Error(err, "error unmarshaling referenced condition")
					continue
				}
				if h.MatchSymbolByPatterns(symbol, cond.Referenced.Pattern) {
					return true
				}
			}
		}
	}
	return false
}

func NewDefaultSymbolCacheHelper(log logr.Logger, config provider.InitConfig) SymbolSearchHelper {
	return &defaultSymbolSearchHelper{
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

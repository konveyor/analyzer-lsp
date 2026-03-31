package nodejs_external_provider

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	base "github.com/konveyor/analyzer-lsp/lsp/base_service_client"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
)

// Evolved from pre-split generic server_configurations (nodejs).
// for nodejs-external-provider (implementation plan Step 4).

type nodejsSymbolSearchHelper struct {
	base.SymbolSearchHelper
	log    logr.Logger
	config provider.InitConfig
}

func (h *nodejsSymbolSearchHelper) GetDocumentUris(conditionsByCap ...provider.ConditionsByCap) []uri.URI {
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
			if path == primaryPath {
				continue
			}
			additionalPaths = append(additionalPaths, path)
		}
	}

	searcher := provider.FileSearcher{
		BasePath:        primaryPath,
		AdditionalPaths: additionalPaths,
		ProviderConfigConstraints: provider.IncludeExcludeConstraints{
			IncludePathsOrPatterns: []string{".*\\.(ts|js)(x?)$"},
			ExcludePathsOrPatterns: []string{
				filepath.Join(primaryPath, ".git"),
				filepath.Join(primaryPath, "dist"),
				filepath.Join(primaryPath, ".vscode"),
				filepath.Join(primaryPath, ".husky"),
				".*node_modules.*",
				".*.min.js",
			},
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

func (h *nodejsSymbolSearchHelper) MatchFileContentByConditions(content string, conditionsByCap ...provider.ConditionsByCap) [][]int {
	matches := [][]int{}
	for _, condition := range conditionsByCap {
		if condition.Cap == "referenced" {
			for _, condition := range condition.Conditions {
				var cond referencedCondition
				err := yaml.Unmarshal(condition, &cond)
				if err != nil {
					h.log.Error(err, "error unmarshaling referenced condition")
					continue
				}
				query := parseQuery(cond.Referenced.Pattern)
				if query != nil && query.Query != nil {
					query := *query.Query
					if !hasRegexMeta(query) {
						matches = append(matches, findWholeWordMatches(content, query)...)
						continue
					}
					regex, err := regexp.Compile(query)
					if err != nil {
						h.log.Error(err, "error compiling query regex")
						continue
					}
					matches = append(matches, regex.FindAllStringIndex(content, -1)...)
				}
			}
		}
	}
	return matches
}

func (h *nodejsSymbolSearchHelper) MatchSymbolByPatterns(symbol base.WorkspaceSymbolDefinitionsPair, patterns ...string) bool {
	unscopedQueries := []parsedQuery{}
	scopedQueries := []parsedQuery{}
	for _, q := range patterns {
		parsedQuery := parseQuery(q)
		if parsedQuery != nil && parsedQuery.Query != nil && parsedQuery.Package != nil && parsedQuery.Scope != nil {
			scopedQueries = append(scopedQueries, *parsedQuery)
		} else if parsedQuery != nil && parsedQuery.Query != nil {
			unscopedQueries = append(unscopedQueries, *parsedQuery)
		}
	}
	for _, query := range scopedQueries {
		if len(symbol.Definitions) > 0 {
			for _, definition := range symbol.Definitions {
				definitionLoc, ok := definition.Location.Value.(protocol.Location)
				if !ok {
					continue
				}
				if strings.Contains(string(definitionLoc.URI), fmt.Sprintf("%s/%s", *query.Package, *query.Scope)) &&
					h.SymbolSearchHelper.MatchSymbolByPatterns(base.WorkspaceSymbolDefinitionsPair{WorkspaceSymbol: definition}, *query.Query) {
					return true
				}
			}
		} else {
			symbolLoc, ok := symbol.WorkspaceSymbol.Location.Value.(protocol.Location)
			if !ok {
				continue
			}
			if strings.Contains(string(symbolLoc.URI), fmt.Sprintf("%s/%s", *query.Package, *query.Scope)) &&
				h.SymbolSearchHelper.MatchSymbolByPatterns(symbol, *query.Query) {
				return true
			}
		}
	}
	for _, query := range unscopedQueries {
		if len(symbol.Definitions) > 0 {
			for _, definition := range symbol.Definitions {
				if h.SymbolSearchHelper.MatchSymbolByPatterns(base.WorkspaceSymbolDefinitionsPair{WorkspaceSymbol: definition}, *query.Query) {
					return true
				}
			}
		} else {
			if h.SymbolSearchHelper.MatchSymbolByPatterns(symbol, *query.Query) {
				return true
			}
		}
	}
	return false
}

func (h *nodejsSymbolSearchHelper) MatchSymbolByConditions(symbol base.WorkspaceSymbolDefinitionsPair, conditions ...provider.ConditionsByCap) bool {
	for _, condition := range conditions {
		if condition.Cap == "referenced" {
			for _, condition := range condition.Conditions {
				var cond referencedCondition
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

func NewNodejsSymbolCacheHelper(log logr.Logger, config provider.InitConfig) base.SymbolSearchHelper {
	return &nodejsSymbolSearchHelper{
		SymbolSearchHelper: base.NewDefaultSymbolCacheHelper(log, config),
		log:                log,
		config:             config,
	}
}

type parsedQuery struct {
	Scope   *string
	Package *string
	Query   *string
}

func parseQuery(pattern string) *parsedQuery {
	result := &parsedQuery{}

	s := strings.TrimSpace(pattern)
	if s == "" {
		return result
	}

	hashIdx := strings.IndexRune(s, '#')
	if hashIdx == -1 {
		q := s
		result.Query = &q
		return result
	}

	prefix := s[:hashIdx]
	queryPart := s[hashIdx+1:]
	if strings.TrimSpace(queryPart) != "" {
		q := queryPart
		result.Query = &q
	}
	slashIdx := strings.IndexRune(prefix, '/')
	if slashIdx == -1 {
		return result
	}
	pkg := prefix[:slashIdx]
	scope := prefix[slashIdx+1:]
	if pkg != "" {
		result.Package = &pkg
	}
	if scope != "" {
		result.Scope = &scope
	}
	return result
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

func findWholeWordMatches(content, query string) [][]int {
	if query == "" {
		return nil
	}
	matches := [][]int{}
	searchStart := 0
	for searchStart <= len(content) {
		idx := strings.Index(content[searchStart:], query)
		if idx == -1 {
			break
		}
		start := searchStart + idx
		end := start + len(query)
		if isTokenBoundary(content, start, end) {
			matches = append(matches, []int{start, end})
		}
		searchStart = end
	}
	return matches
}

func isTokenBoundary(content string, start, end int) bool {
	if start > 0 && isIdentifierChar(content[start-1]) {
		return false
	}
	if end < len(content) && isIdentifierChar(content[end]) {
		return false
	}
	return true
}

func isIdentifierChar(b byte) bool {
	switch {
	case b >= '0' && b <= '9':
		return true
	case b >= 'A' && b <= 'Z':
		return true
	case b >= 'a' && b <= 'z':
		return true
	case b == '_', b == '$', b == '#':
		return true
	default:
		return false
	}
}

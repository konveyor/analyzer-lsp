package nodejs

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

// nodejsSymbolSearchHelper is a SymbolCacheHelper that is based on the default
// cache provider, but provides additional functionality on top to parse the
// FQNs of the queries.
type nodejsSymbolSearchHelper struct {
	base.SymbolSearchHelper
	log    logr.Logger
	config provider.InitConfig
}

// GetDocumentUris returns URIs of all nodejs related files that we need dependending on what is queried in rules.
// The referenced condition pattern is parsed as "<package>/<scope>#<type>.<child_type>...". If <package> & <scope>
// is present, the node_modules/ folder for that package is added to search. If not, only source is searched.
func (h *nodejsSymbolSearchHelper) GetDocumentUris(conditionsByCap ...provider.ConditionsByCap) []uri.URI {
	primaryPath := h.config.Location
	if after, ok := strings.CutPrefix(primaryPath, fmt.Sprintf("%s://", uri.FileScheme)); ok {
		primaryPath = after
	}
	additionalPaths := []string{}
	val, ok := h.config.ProviderSpecificConfig["workspaceFolders"].([]interface{})
	if ok {
		for _, path := range val {
			pathStr, ok := path.(string)
			if !ok {
				continue
			}
			if after, prefixOk := strings.CutPrefix(pathStr, fmt.Sprintf("%s://", uri.FileScheme)); prefixOk {
				pathStr = after
			}
			if primaryPath == "" {
				primaryPath = pathStr
				continue
			}
			additionalPaths = append(additionalPaths, pathStr)
		}
	}

	// find all files in the source first
	searcher := provider.FileSearcher{
		BasePath:        primaryPath,
		AdditionalPaths: additionalPaths,
		ProviderConfigConstraints: provider.IncludeExcludeConstraints{
			IncludePathsOrPatterns: []string{".*\\.(ts|js)(x?)$"}, // only search js files
			ExcludePathsOrPatterns: []string{
				filepath.Join(primaryPath, ".git"),
				filepath.Join(primaryPath, "dist"),
				filepath.Join(primaryPath, ".vscode"),
				filepath.Join(primaryPath, ".husky"),
				".*node_modules.*",
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
					regex, err := regexp.Compile(*query.Query)
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
	// apply stricter queries first
	for _, query := range scopedQueries {
		// when evaluating a scoped query, use the associated definitions of the symbol to check its origin
		if len(symbol.Definitions) > 0 {
			for _, definition := range symbol.Definitions {
				definitionLoc, ok := definition.Location.Value.(protocol.Location)
				if !ok {
					continue
				}
				// if the definition is under <package>/<scope> && definition name matches pattern, we have found right symbol
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

// parseQuery extracts the top level package from a pattern.
// The pattern is parsed as "<package>/<scope>#<type>.<child_type>...".
func parseQuery(pattern string) *parsedQuery {
	result := &parsedQuery{}

	s := strings.TrimSpace(pattern)
	if s == "" {
		return result
	}

	hashIdx := strings.IndexRune(s, '#')
	if hashIdx == -1 {
		// No #; entire string is query
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

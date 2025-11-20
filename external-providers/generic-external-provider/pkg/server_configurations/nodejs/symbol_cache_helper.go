package nodejs

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	base "github.com/konveyor/analyzer-lsp/lsp/base_service_client"
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
// The referenced condition pattern is parsed as "@<package>/<scope>#<type>.<child_type>...". If <package> & <scope>
// is present, the node_modules/ folder for that package is added to search. If not, only source is searched.
func (h *nodejsSymbolSearchHelper) GetDocumentUris(conditionsByCap ...provider.ConditionsByCap) []uri.URI {
	// find all files in the source first
	searcher := provider.FileSearcher{
		BasePath:        h.config.Location,
		AdditionalPaths: h.config.ProviderSpecificConfig["workspaceFolders"].([]string),
		ProviderConfigConstraints: provider.IncludeExcludeConstraints{
			IncludePathsOrPatterns: []string{".*\\.(ts|js)(x?)$"}, // only search js files
			ExcludePathsOrPatterns: []string{
				filepath.Join(h.config.Location, ".git"),
				filepath.Join(h.config.Location, "dist"),
				filepath.Join(h.config.Location, ".vscode"),
				filepath.Join(h.config.Location, ".husky"),
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

func (h *nodejsSymbolSearchHelper) MatchFileContent(content string, conditionsByCap ...provider.ConditionsByCap) [][]int {
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
	filteredPatterns := []string{}
	for _, q := range patterns {
		parsedQuery := parseQuery(q)
		if parsedQuery != nil && parsedQuery.Query != nil {
			filteredPatterns = append(filteredPatterns, *parsedQuery.Query)
		}
	}
	// if there is a definition stored with this symbol, match the definition against the patterns
	if symbol.Definitions != nil {
		for _, definition := range symbol.Definitions {
			if h.SymbolSearchHelper.MatchSymbolByPatterns(base.WorkspaceSymbolDefinitionsPair{
				WorkspaceSymbol: definition,
			}, filteredPatterns...) {
				return true
			}
		}
	}
	return h.SymbolSearchHelper.MatchSymbolByPatterns(symbol, filteredPatterns...)
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
// The pattern is parsed as "@<package>/<scope>#<type>.<child_type>...".
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
	if prefix == "" || !strings.HasPrefix(prefix, "@") {
		return result
	}
	// remove the leading @
	rest := prefix[1:]
	slashIdx := strings.IndexRune(rest, '/')
	if slashIdx == -1 {
		return result
	}
	pkg := rest[:slashIdx]
	scope := rest[slashIdx+1:]
	if pkg != "" {
		result.Package = &pkg
	}
	if scope != "" {
		result.Scope = &scope
	}
	return result
}

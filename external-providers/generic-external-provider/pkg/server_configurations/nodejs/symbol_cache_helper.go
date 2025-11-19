package nodejs

import (
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	base "github.com/konveyor/analyzer-lsp/lsp/base_service_client"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
)

// nodejsSymbolCacheHelper is a SymbolCacheHelper that is based on the default
// cache provider, but provides additional functionality on top to parse the
// FQNs of the queries.
type nodejsSymbolCacheHelper struct {
	base.SymbolCacheHelper
	log    logr.Logger
	config provider.InitConfig
}

// GetDocumentUris returns URIs of all nodejs related files that we need dependending on what is queried in rules.
// The referenced condition pattern is parsed as "@<package>/<scope>#<type>.<child_type>...". If <package> & <scope>
// is present, the node_modules/ folder for that package is added to search. If not, only source is searched.
func (h *nodejsSymbolCacheHelper) GetDocumentUris(conditionsByCap []provider.ConditionsByCap) []uri.URI {
	included, excluded := []string{}, []string{}
	nodeModulesPackages := map[string]bool{}
	for _, conditions := range conditionsByCap {
		if conditions.Cap == "referenced" {
			for _, condition := range conditions.Conditions {
				var cond referencedCondition
				err := yaml.Unmarshal(condition, &cond)
				if err != nil {
					continue
				}
				condIncluded, condExcluded := cond.ProviderContext.GetScopedFilepaths()
				included = append(included, condIncluded...)
				excluded = append(excluded, condExcluded...)
				if pkg := parseQuery(cond.Referenced.Pattern); pkg != nil && pkg.Package != nil {
					packagePath := filepath.Join(h.config.Location, "node_modules")
					if pkg.Scope != nil {
						packagePath = filepath.Join(packagePath, "@"+*pkg.Scope)
					}
					packagePath = filepath.Join(packagePath, *pkg.Package)
					nodeModulesPackages[packagePath] = true
				}
			}
		}
	}
	// find all files in the source first
	searcher := provider.FileSearcher{
		BasePath:        h.config.Location,
		AdditionalPaths: h.config.ProviderSpecificConfig["workspaceFolders"].([]string),
		ProviderConfigConstraints: provider.IncludeExcludeConstraints{
			IncludePathsOrPatterns: []string{".*\\.(ts|js)(x?)$"}, // only search js files
			ExcludePathsOrPatterns: []string{
				filepath.Join(h.config.Location, "node_modules"),
				filepath.Join(h.config.Location, "vendor"),
				filepath.Join(h.config.Location, ".git"),
				filepath.Join(h.config.Location, "dist"),
				filepath.Join(h.config.Location, "build"),
				filepath.Join(h.config.Location, "target"),
				filepath.Join(h.config.Location, ".venv"),
				filepath.Join(h.config.Location, "venv"),
				filepath.Join(h.config.Location, ".vscode"),
				filepath.Join(h.config.Location, ".idea"),
				filepath.Join(h.config.Location, ".husky"),
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
	// find all files in the node_modules folders
	if len(nodeModulesPackages) > 0 {
		nodeModulesPaths := []string{}
		for pkg := range nodeModulesPackages {
			nodeModulesPaths = append(nodeModulesPaths, pkg)
		}
		searcher = provider.FileSearcher{
			BasePath:        nodeModulesPaths[0],
			AdditionalPaths: nodeModulesPaths[1:],
			ProviderConfigConstraints: provider.IncludeExcludeConstraints{
				IncludePathsOrPatterns: []string{".*\\.(ts|js)(x?)$"}, // only search js files
			},
		}
		files = append(files, files...)
	}
	uris := []uri.URI{}
	for _, file := range files {
		uris = append(uris, uri.File(file))
	}
	return uris
}

func (h *nodejsSymbolCacheHelper) MatchSymbol(symbol protocol.WorkspaceSymbol, query ...string) bool {
	filteredQueries := []string{}
	for _, q := range query {
		parsedQuery := parseQuery(q)
		if parsedQuery != nil && parsedQuery.Query != nil {
			filteredQueries = append(filteredQueries, *parsedQuery.Query)
		}
	}
	return h.SymbolCacheHelper.MatchSymbol(symbol, filteredQueries...)
}

func NewNodejsSymbolCacheHelper(log logr.Logger, config provider.InitConfig) *nodejsSymbolCacheHelper {
	return &nodejsSymbolCacheHelper{
		SymbolCacheHelper: base.NewDefaultSymbolCacheHelper(log, config),
		log:               log,
		config:            config,
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

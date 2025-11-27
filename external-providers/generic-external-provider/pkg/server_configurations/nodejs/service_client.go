package nodejs

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/go-logr/logr"
	base "github.com/konveyor/analyzer-lsp/lsp/base_service_client"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/swaggest/openapi-go/openapi3"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
)

type NodeServiceClientConfig struct {
	base.LSPServiceClientConfig `yaml:",inline"`

	blah int `yaml:",inline"`
}

// Tidy aliases
type serviceClientFn = base.LSPServiceClientFunc[*NodeServiceClient]

type NodeServiceClient struct {
	*base.LSPServiceClientBase
	*base.LSPServiceClientEvaluator[*NodeServiceClient]

	Config NodeServiceClientConfig
}

type NodeServiceClientBuilder struct{}

func (n *NodeServiceClientBuilder) Init(ctx context.Context, log logr.Logger, c provider.InitConfig) (provider.ServiceClient, error) {
	sc := &NodeServiceClient{}

	// Unmarshal the config
	b, _ := yaml.Marshal(c.ProviderSpecificConfig)
	err := yaml.Unmarshal(b, &sc.Config)
	if err != nil {
		return nil, err
	}

	params := protocol.InitializeParams{}

	// treat location as the only workspace folder
	if c.Location != "" {
		if !strings.HasPrefix(c.Location, "file://") {
			c.Location = "file://" + c.Location
		}
		sc.Config.WorkspaceFolders = []string{c.Location}
	}

	if c.ProviderSpecificConfig == nil {
		c.ProviderSpecificConfig = map[string]interface{}{}
	}
	c.ProviderSpecificConfig["workspaceFolders"] = sc.Config.WorkspaceFolders

	if len(sc.Config.WorkspaceFolders) == 0 {
		params.RootURI = ""
	} else {
		params.RootURI = sc.Config.WorkspaceFolders[0]
	}

	var workspaceFolders []protocol.WorkspaceFolder
	seen := make(map[string]bool)
	for _, f := range sc.Config.WorkspaceFolders {
		if seen[f] {
			continue
		}
		seen[f] = true
		workspaceFolders = append(workspaceFolders, protocol.WorkspaceFolder{
			URI:  f,
			Name: filepath.Base(strings.ReplaceAll(f, "file://", "")),
		})
	}
	params.WorkspaceFolders = workspaceFolders

	params.Capabilities = protocol.ClientCapabilities{
		Workspace: &protocol.WorkspaceClientCapabilities{
			WorkspaceFolders: true,
			// enables the server to refresh diagnostics on-demand, useful in agent mode
			Diagnostics: &protocol.DiagnosticWorkspaceClientCapabilities{
				RefreshSupport: true,
			},
		},
		TextDocument: &protocol.TextDocumentClientCapabilities{
			// this enables the textDocument/definition responses to be
			// LocationLink[] instead of Location[]. LocationLink contains
			// source -> target mapping of symbols which gives us more information
			Definition: &protocol.DefinitionClientCapabilities{
				LinkSupport: true,
			},
			// this enables the documentSymbol responses to be a tree instead of a flat list
			// this allows us to understand enclosed symbols better. Right now, we use this
			// information to find a concrete symbol at a location. While a flat list could
			// work, but in future, the tree will help us with advanced queries.
			DocumentSymbol: &protocol.DocumentSymbolClientCapabilities{
				HierarchicalDocumentSymbolSupport: true,
			},
		},
	}

	var InitializationOptions map[string]any
	err = json.Unmarshal([]byte(sc.Config.LspServerInitializationOptions), &InitializationOptions)
	if err != nil {
		// fmt.Printf("Could not unmarshal into map[string]any: %s\n", sc.Config.LspServerInitializationOptions)
		params.InitializationOptions = map[string]any{}
	} else {
		params.InitializationOptions = InitializationOptions
	}

	// Initialize the base client
	scBase, err := base.NewLSPServiceClientBase(
		ctx, log, c,
		base.LogHandler(log),
		params,
		NewNodejsSymbolCacheHelper(log, c),
	)
	if err != nil {
		return nil, err
	}
	sc.LSPServiceClientBase = scBase

	// Initialize the fancy evaluator (dynamic dispatch ftw)
	eval, err := base.NewLspServiceClientEvaluator(sc, n.GetGenericServiceClientCapabilities(log))
	if err != nil {
		return nil, err
	}
	sc.LSPServiceClientEvaluator = eval

	return sc, nil
}

func (n *NodeServiceClientBuilder) GetGenericServiceClientCapabilities(log logr.Logger) []base.LSPServiceClientCapability {
	caps := []base.LSPServiceClientCapability{}
	r := openapi3.NewReflector()
	refCap, err := provider.ToProviderCap(r, log, referencedCondition{}, "referenced")
	if err != nil {
		log.Error(err, "unable to get referenced cap")
	} else {
		caps = append(caps, base.LSPServiceClientCapability{
			Capability: refCap,
			Fn:         serviceClientFn((*NodeServiceClient).EvaluateReferenced),
		})
	}
	return caps
}

type resp = provider.ProviderEvaluateResponse

// Example condition
type referencedCondition struct {
	Referenced struct {
		Pattern string `yaml:"pattern"`
	} `yaml:"referenced"`
	provider.ProviderContext `yaml:",inline"`
}

// normalizePathForComparison removes URI schemes and cleans paths for comparison.
// It handles cross-platform path differences including:
// - URI schemes (file://, file:)
// - Path separators (converts backslashes to forward slashes)
// - Case sensitivity (normalizes to lowercase on Windows)
func normalizePathForComparison(path string) string {
	// Remove common URI schemes (some systems emit file: instead of file://)
	path = strings.TrimPrefix(path, "file://")
	path = strings.TrimPrefix(path, "file:")
	// Clean the path to resolve . and .. elements
	path = filepath.Clean(path)
	// Convert to forward slashes for consistent comparison across platforms
	path = filepath.ToSlash(path)
	// On Windows, normalize to lowercase for case-insensitive comparison
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}

// Example evaluate
func (sc *NodeServiceClient) EvaluateReferenced(ctx context.Context, cap string, info []byte) (provider.ProviderEvaluateResponse, error) {
	var cond referencedCondition
	err := yaml.Unmarshal(info, &cond)
	if err != nil {
		return resp{}, fmt.Errorf("error unmarshaling query info")
	}

	query := cond.Referenced.Pattern
	if query == "" {
		return resp{}, fmt.Errorf("unable to get query info")
	}

	// Extract filepath scope from the ProviderContext
	includedFilepaths, excludedFilepaths := cond.ProviderContext.GetScopedFilepaths()

	// Build maps for O(1) lookups instead of O(n) linear searches
	// This is critical for performance with large codebases
	excludedPathsMap := make(map[string]bool, len(excludedFilepaths))
	for _, excludedPath := range excludedFilepaths {
		if excludedPath == "" {
			continue // Skip empty strings
		}
		normalizedPath := normalizePathForComparison(excludedPath)
		excludedPathsMap[normalizedPath] = true
	}

	includedPathsMap := make(map[string]bool, len(includedFilepaths))
	for _, includedPath := range includedFilepaths {
		if includedPath == "" {
			continue // Skip empty strings
		}
		normalizedPath := normalizePathForComparison(includedPath)
		includedPathsMap[normalizedPath] = true
	}

	// Query symbols once after all files are indexed
	symbols := sc.GetAllDeclarations(ctx, query, false)
	incidentsMap, err := sc.EvaluateSymbols(ctx, symbols)
	if err != nil {
		return resp{}, err
	}

	incidents := []provider.IncidentContext{}
	for _, incident := range incidentsMap {
		normalizedIncidentPath := normalizePathForComparison(string(incident.FileURI))

		// Check if excluded (O(1) lookup)
		if excludedPathsMap[normalizedIncidentPath] {
			continue
		}

		// Check if included (O(1) lookup) - only if include list exists
		if len(includedPathsMap) > 0 && !includedPathsMap[normalizedIncidentPath] {
			continue
		}

		incidents = append(incidents, incident)
	}
	if len(incidents) == 0 {
		return resp{Matched: false}, nil
	}
	return resp{
		Matched:   true,
		Incidents: incidents,
	}, nil
}

func (sc *NodeServiceClient) EvaluateSymbols(ctx context.Context, symbols []protocol.WorkspaceSymbol) (map[string]provider.IncidentContext, error) {
	incidentsMap := make(map[string]provider.IncidentContext)

	for _, s := range symbols {
		baseLocation, ok := s.Location.Value.(protocol.Location)
		if !ok {
			sc.Log.V(7).Info("unable to get base location", "symbol", s)
			continue
		}
		// Look for things that are in the location loaded,
		// Note may need to filter out vendor at some point
		if len(sc.BaseConfig.WorkspaceFolders) < 1 || !strings.Contains(baseLocation.URI, sc.BaseConfig.WorkspaceFolders[0]) {
			continue
		}

		skip := false
		for _, substr := range sc.BaseConfig.DependencyFolders {
			if substr == "" {
				continue
			}

			if strings.Contains(baseLocation.URI, substr) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		u, err := uri.Parse(baseLocation.URI)
		if err != nil {
			return nil, err
		}
		lineNumber := int(baseLocation.Range.Start.Line) + 1
		incident := provider.IncidentContext{
			FileURI:    u,
			LineNumber: &lineNumber,
			Variables: map[string]interface{}{
				"file": baseLocation.URI,
			},
			CodeLocation: &provider.Location{
				StartPosition: provider.Position{
					Line:      float64(lineNumber),
					Character: float64(baseLocation.Range.Start.Character),
				},
				EndPosition: provider.Position{
					Line:      float64(lineNumber),
					Character: float64(baseLocation.Range.End.Character),
				},
			},
		}
		b, _ := json.Marshal(incident)

		incidentsMap[string(b)] = incident
	}

	return incidentsMap, nil
}

func (sc *NodeServiceClient) GetDependencies(ctx context.Context) (map[uri.URI][]*provider.Dep, error) {
	return nil, nil
}

func (sc *NodeServiceClient) GetDependenciesDAG(ctx context.Context) (map[uri.URI][]provider.DepDAGItem, error) {
	return nil, nil
}

package nodejs_external_provider

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	base "github.com/konveyor/analyzer-lsp/lsp/base_service_client"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/progress"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/swaggest/openapi-go/openapi3"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
)

// Evolved from pre-split generic server_configurations (nodejs).
// for nodejs-external-provider (implementation plan Step 4).

type NodeServiceClientConfig struct {
	base.LSPServiceClientConfig `yaml:",inline"`

	blah int `yaml:",inline"`
}

type serviceClientFn = base.LSPServiceClientFunc[*NodeServiceClient]

type NodeServiceClient struct {
	*base.LSPServiceClientBase
	*base.LSPServiceClientEvaluator[*NodeServiceClient]

	Config        NodeServiceClientConfig
	includedPaths []string
}

type NodeServiceClientBuilder struct {
	Progress *progress.Progress
}

func (n *NodeServiceClientBuilder) Init(ctx context.Context, log logr.Logger, c provider.InitConfig) (provider.ServiceClient, error) {
	sc := &NodeServiceClient{}

	b, _ := yaml.Marshal(c.ProviderSpecificConfig)
	err := yaml.Unmarshal(b, &sc.Config)
	if err != nil {
		return nil, err
	}

	sc.includedPaths = provider.GetIncludedPathsFromConfig(c, false)

	params := protocol.InitializeParams{}

	if c.Location != "" {
		sc.Config.WorkspaceFolders = []string{string(uri.File(c.Location))}
	}

	if c.ProviderSpecificConfig == nil {
		c.ProviderSpecificConfig = map[string]any{}
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
			Diagnostics: &protocol.DiagnosticWorkspaceClientCapabilities{
				RefreshSupport: true,
			},
		},
		TextDocument: &protocol.TextDocumentClientCapabilities{
			Definition: &protocol.DefinitionClientCapabilities{
				LinkSupport: true,
			},
			DocumentSymbol: &protocol.DocumentSymbolClientCapabilities{
				HierarchicalDocumentSymbolSupport: true,
			},
		},
	}

	var InitializationOptions map[string]any
	err = json.Unmarshal([]byte(sc.Config.LspServerInitializationOptions), &InitializationOptions)
	if err != nil {
		params.InitializationOptions = map[string]any{}
	} else {
		params.InitializationOptions = InitializationOptions
	}

	scBase, err := base.NewLSPServiceClientBase(
		ctx, log, c,
		base.LogHandler(log),
		params,
		NewNodejsSymbolCacheHelper(log, c),
		n.Progress,
	)
	if err != nil {
		return nil, err
	}
	sc.LSPServiceClientBase = scBase

	eval, err := base.NewLspServiceClientEvaluator(sc, n.GetNodeServiceClientCapabilities(log))
	if err != nil {
		return nil, err
	}
	sc.LSPServiceClientEvaluator = eval

	return sc, nil
}

func (n *NodeServiceClientBuilder) GetNodeServiceClientCapabilities(log logr.Logger) []base.LSPServiceClientCapability {
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

type referencedCondition struct {
	Referenced struct {
		Pattern string `yaml:"pattern"`
	} `yaml:"referenced"`
	Filepaths                []string `yaml:"filepaths,omitempty"`
	provider.ProviderContext `yaml:",inline"`
}

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

	includedFilepaths, excludedFilepaths := cond.ProviderContext.GetScopedFilepaths()

	basePath := ""
	if len(sc.BaseConfig.WorkspaceFolders) > 0 {
		basePath = sc.BaseConfig.WorkspaceFolders[0]
		basePath = strings.TrimPrefix(basePath, "file://")
	}

	nonEmptyDependencyFolders := []string{}
	for _, folder := range sc.BaseConfig.DependencyFolders {
		if folder != "" {
			nonEmptyDependencyFolders = append(nonEmptyDependencyFolders, folder)
		}
	}

	fileSearcher := provider.FileSearcher{
		BasePath: basePath,
		ProviderConfigConstraints: provider.IncludeExcludeConstraints{
			IncludePathsOrPatterns: sc.includedPaths,
			ExcludePathsOrPatterns: nonEmptyDependencyFolders,
		},
		RuleScopeConstraints: provider.IncludeExcludeConstraints{
			IncludePathsOrPatterns: includedFilepaths,
			ExcludePathsOrPatterns: excludedFilepaths,
		},
		FailFast: true,
		Log:      sc.Log,
	}

	type fileSearchResult struct {
		fileMap map[string]struct{}
		err     error
	}
	resultCh := make(chan fileSearchResult, 1)

	go func() {
		defer close(resultCh)
		paths, err := fileSearcher.Search(provider.SearchCriteria{
			ConditionFilepaths: cond.Filepaths,
		})
		if err != nil {
			sc.Log.Error(err, "failed to search for files")
			resultCh <- fileSearchResult{err: err}
			return
		}
		fileMap := make(map[string]struct{})
		for _, path := range paths {
			normalizedPath := provider.NormalizePathForComparison(path)
			fileMap[normalizedPath] = struct{}{}
		}
		resultCh <- fileSearchResult{fileMap: fileMap}
	}()

	symbols := sc.GetAllDeclarations(ctx, query, false)

	searchResult := <-resultCh

	skipFiltering := searchResult.err != nil

	incidentsMap, err := sc.EvaluateSymbols(ctx, symbols)
	if err != nil {
		return resp{}, err
	}

	incidents := []provider.IncidentContext{}
	for _, incident := range incidentsMap {
		if !skipFiltering {
			normalizedIncidentPath := provider.NormalizePathForComparison(string(incident.FileURI))
			if _, ok := searchResult.fileMap[normalizedIncidentPath]; !ok {
				continue
			}
		}

		incidents = append(incidents, incident)
	}
	if len(incidents) == 0 {
		return resp{Matched: false}, nil
	}
	sc.Log.Info("incidents for referenced condition", "incidents", len(incidents), "condition", query)
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

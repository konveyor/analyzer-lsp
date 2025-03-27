package nodejs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

	if c.Location != "" {
		sc.Config.WorkspaceFolders = []string{c.Location}
	}

	if len(sc.Config.WorkspaceFolders) == 0 {
		params.RootURI = ""
	} else {
		params.RootURI = sc.Config.WorkspaceFolders[0]
	}
	// var workspaceFolders []protocol.WorkspaceFolder
	// for _, f := range sc.Config.WorkspaceFolders {
	// 	workspaceFolders = append(workspaceFolders, protocol.WorkspaceFolder{
	// 		URI:  f,
	// 		Name: f,
	// 	})
	// }
	// params.WorkspaceFolders = workspaceFolders

	params.Capabilities = protocol.ClientCapabilities{}

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
	)
	if err != nil {
		return nil, err
	}
	sc.LSPServiceClientBase = scBase

	// Initialize the fancy evaluator (dynamic dispatch ftw)
	eval, err := base.NewLspServiceClientEvaluator[*NodeServiceClient](sc, n.GetGenericServiceClientCapabilities(log))
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

	// get all ts files
	folder := strings.TrimPrefix(sc.Config.WorkspaceFolders[0], "file://")
	var nodeFiles []string
	err = filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// TODO something else with this
		if info.IsDir() && filepath.Base(path) == "node_modules" {
			return filepath.SkipDir
		}
		if !info.IsDir() && filepath.Ext(path) == ".ts" {
			path = "file://" + path
			nodeFiles = append(nodeFiles, path)
		}

		return nil
	})

	if err != nil {
		return provider.ProviderEvaluateResponse{}, err
	}

	didOpen := func(uri string, text []byte) error {
		params := protocol.DidOpenTextDocumentParams{
			TextDocument: protocol.TextDocumentItem{
				URI:        uri,
				LanguageID: "typescript",
				Version:    0,
				Text:       string(text),
			},
		}
		// typescript server seems to throw "No project" error without notification
		// perhaps there's a better way to do this
		return sc.Conn.Notify(ctx, "textDocument/didOpen", params)
	}

	didClose := func(uri string) error {
		params := protocol.DidCloseTextDocumentParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: uri,
			},
		}
		return sc.Conn.Notify(ctx, "textDocument/didClose", params)
	}

	BATCH_SIZE := 32
	batchRight, batchLeft := 0, 0
	var symbols []protocol.WorkspaceSymbol
	for batchRight < len(nodeFiles) {
		for batchRight-batchLeft < BATCH_SIZE && batchRight < len(nodeFiles) {
			text, err := os.ReadFile(nodeFiles[batchRight][7:])
			if err != nil {
				return provider.ProviderEvaluateResponse{}, err
			}
			err = didOpen(nodeFiles[batchRight], text)
			if err != nil {
				return provider.ProviderEvaluateResponse{}, err
			}

			batchRight++
		}
		//time.Sleep(2 * time.Second)
		symbols = sc.GetAllDeclarations(ctx, sc.BaseConfig.WorkspaceFolders, query)
	}

	incidentsMap, err := sc.EvaluateSymbols(ctx, symbols)
	if err != nil {
		return resp{}, err
	}

	for batchLeft < batchRight {
		if err = didClose(nodeFiles[batchLeft]); err != nil {
			return provider.ProviderEvaluateResponse{}, err
		}
		batchLeft++
	}

	incidents := []provider.IncidentContext{}
	for _, incident := range incidentsMap {
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
		references := sc.GetAllReferences(ctx, s.Location.Value.(protocol.Location))
		breakEarly := false
		for _, ref := range references {
			// Look for things that are in the location loaded,
			// Note may need to filter out vendor at some point
			if !strings.Contains(ref.URI, sc.BaseConfig.WorkspaceFolders[0]) {
				continue
			}

			for _, substr := range sc.BaseConfig.DependencyFolders {
				if substr == "" {
					continue
				}

				if strings.Contains(ref.URI, substr) {
					breakEarly = true
					break
				}
			}
			if breakEarly {
				break
			}

			u, err := uri.Parse(ref.URI)
			if err != nil {
				return nil, err
			}
			lineNumber := int(ref.Range.Start.Line)
			incident := provider.IncidentContext{
				FileURI:    u,
				LineNumber: &lineNumber,
				Variables: map[string]interface{}{
					"file": ref.URI,
				},
			}
			b, _ := json.Marshal(incident)

			incidentsMap[string(b)] = incident
		}
	}

	return incidentsMap, nil
}

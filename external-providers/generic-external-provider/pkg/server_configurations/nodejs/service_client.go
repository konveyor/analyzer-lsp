package nodejs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3gen"
	"github.com/go-logr/logr"
	base "github.com/konveyor/analyzer-lsp/lsp/base_service_client"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
)

type NodeServiceClientConfig struct {
	base.LSPServiceClientConfig `yaml:",inline"`

	blah int `yaml:",inline"`
}

type NodeServiceClient struct {
	*base.LSPServiceClientBase
	*base.LSPServiceClientEvaluator[*NodeServiceClient]

	Config NodeServiceClientConfig
}

func NewNodeServiceClient(ctx context.Context, log logr.Logger, c provider.InitConfig) (provider.ServiceClient, error) {
	sc := &NodeServiceClient{}

	// Unmarshal the config
	b, _ := yaml.Marshal(c.ProviderSpecificConfig)
	err := yaml.Unmarshal(b, &sc.Config)
	if err != nil {
		return nil, err
	}

	// Create the parameters for the `initialize` request
	//
	// TODO(jsussman): Support more than one folder. This hack with only taking
	// the first item in WorkspaceFolders is littered throughout.
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
	eval, err := base.NewLspServiceClientEvaluator[*NodeServiceClient](sc, NodeServiceClientCapabilities)
	if err != nil {
		return nil, err
	}
	sc.LSPServiceClientEvaluator = eval

	return sc, nil
}

// Tidy aliases

type serviceClientFn = base.LSPServiceClientFunc[*NodeServiceClient]

func serviceClientTemplateContext(v any) openapi3.SchemaRef {
	r, _ := openapi3gen.NewSchemaRefForValue(v, nil)
	return *r
}

var NodeServiceClientCapabilities = []base.LSPServiceClientCapability{
	{
		Name:            "referenced",
		TemplateContext: serviceClientTemplateContext(referencedCondition{}),
		Fn:              serviceClientFn((*NodeServiceClient).EvaluateReferenced),
	},
	{
		Name:            "dependency",
		TemplateContext: serviceClientTemplateContext(base.NoOpCondition{}),
		Fn:              serviceClientFn(base.EvaluateNoOp[*NodeServiceClient]),
	},
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

	// f, err := os.ReadFile("/path/to/test.ts")
	// if err != nil {
	// 	panic(err)
	// }

	// p := protocol.DidOpenTextDocumentParams{
	// 	TextDocument: protocol.TextDocumentItem{
	// 		URI:        "file:///path/to/test.ts",
	// 		LanguageID: "typescript",
	// 		Version:    0,
	// 		Text:       string(f),
	// 	},
	// }
	// err = sc.Conn.Notify(ctx, "textDocument/didOpen", p)
	// if err != nil {
	// 	panic(err)
	// }

	// time.Sleep(2 * time.Second)

	symbols := sc.GetAllDeclarations(ctx, sc.BaseConfig.WorkspaceFolders, query)

	fmt.Printf("symbols: %v\n", symbols)

	incidents := []provider.IncidentContext{}
	incidentsMap := make(map[string]provider.IncidentContext) // Remove duplicates

	for _, s := range symbols {
		references := sc.GetAllReferences(ctx, s.Location.Value.(protocol.Location))

		fmt.Printf("references: %v\n", references)

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
				return resp{}, err
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

	for _, incident := range incidentsMap {
		incidents = append(incidents, incident)
	}

	// No results were found.
	if len(incidents) == 0 {
		return resp{Matched: false}, nil
	}
	return resp{
		Matched:   true,
		Incidents: incidents,
	}, nil
}

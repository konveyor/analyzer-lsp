package yaml_language_server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3gen"
	"github.com/go-logr/logr"
	jsonrpc2 "github.com/konveyor/analyzer-lsp/jsonrpc2_v2"
	base "github.com/konveyor/analyzer-lsp/lsp/base_service_client"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
)

type YamlServiceClientConfig struct {
	base.LSPServiceClientConfig `yaml:",inline"`
}

type YamlServiceClient struct {
	*base.LSPServiceClientBase
	*base.LSPServiceClientEvaluator[*YamlServiceClient]

	Config YamlServiceClientConfig

	// There is a bug in yaml-language-server where a
	// workspace/didChangeConfiguration request isn't handled properly. Instead of
	// taking the config from the request, it immediately makes a
	// workspace/configuration request to the client:
	//
	// this.connection.onDidChangeConfiguration(() => this.pullConfiguration());
	//
	// https://github.com/redhat-developer/yaml-language-server/issues/927
	//
	// I'm also 90 percent sure there's another bug in the yaml-language-server,
	// where it sends an *additional* workspace/configuration request after
	// completing the first one. Running `strace -e trace=read,write,open,close -p
	// <pid> -s 128` confirms this.
	//
	// I'm not sure if it matters what we send for the second request, but nothing
	// broke (I think) when we sent the exact same response as the first one.
	configParams []map[string]any
}

func NewYamlServiceClient(ctx context.Context, log logr.Logger, c provider.InitConfig) (provider.ServiceClient, error) {
	sc := &YamlServiceClient{}

	// Unmarshal the config
	b, _ := yaml.Marshal(c.ProviderSpecificConfig)
	err := yaml.Unmarshal(b, &sc.Config)
	if err != nil {
		return nil, err
	}

	// Create handler for workspace/configuration requests. See configParams
	// comment
	sc.configParams = make([]map[string]any, 0)
	h := func(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
		switch req.Method {
		case "workspace/configuration":
			return sc.configParams, nil
		}
		return nil, jsonrpc2.ErrNotHandled
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

	params.Capabilities = protocol.ClientCapabilities{}

	var InitializationOptions map[string]any
	err = json.Unmarshal([]byte(sc.Config.LspServerInitializationOptions), &InitializationOptions)
	if err != nil {
		params.InitializationOptions = map[string]any{}
	} else {
		params.InitializationOptions = InitializationOptions
	}

	// Initialize the base client
	scBase, err := base.NewLSPServiceClientBase(
		ctx, log, c,
		base.NewChainHandler(
			jsonrpc2.HandlerFunc(h),
			base.LogHandler(log),
		),
		params,
	)
	if err != nil {
		return nil, err
	}
	sc.LSPServiceClientBase = scBase

	// Initialize the fancy evaluator
	eval, err := base.NewLspServiceClientEvaluator[*YamlServiceClient](sc, YamlServiceClientCapabilities)
	if err != nil {
		return nil, err
	}
	sc.LSPServiceClientEvaluator = eval

	return sc, nil
}

// Tidy alias
type serviceClientFn = base.LSPServiceClientFunc[*YamlServiceClient]

func serviceClientTemplateContext(v any) openapi3.SchemaRef {
	r, _ := openapi3gen.NewSchemaRefForValue(v, nil)
	return *r
}

var YamlServiceClientCapabilities = []base.LSPServiceClientCapability{
	{
		Name:            "referenced",
		TemplateContext: serviceClientTemplateContext(referencedCondition{}),
		Fn:              serviceClientFn((*YamlServiceClient).EvaluateReferenced),
	},
	{
		Name:            "dependency",
		TemplateContext: serviceClientTemplateContext(base.NoOpCondition{}),
		Fn:              serviceClientFn(base.EvaluateNoOp[*YamlServiceClient]),
	},
}

type referencedCondition struct {
	Referenced struct {
		Pattern string `yaml:"pattern"`
	} `yaml:"referenced"`
}

func (sc *YamlServiceClient) EvaluateReferenced(ctx context.Context, cap string, info []byte) (provider.ProviderEvaluateResponse, error) {
	var cond referencedCondition
	err := yaml.Unmarshal(info, &cond)
	if err != nil {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("error unmarshaling query info")
	}

	query := cond.Referenced.Pattern
	if query == "" {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get query info")
	}

	// Transform query into schema and save to temp file
	schemaMap := map[string]any{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"anyOf": []map[string]any{
			{
				"$ref": "#/definitions/commentObject",
			},
			{
				"type":        "array",
				"description": "Array of Comment objects",
				"items": map[string]any{
					"$ref": "#/definitions/commentObject",
				},
			},
		},
		"definitions": map[string]any{
			"commentObject": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"patternProperties": map[string]any{
					"^.*$": map[string]any{
						"not": map[string]any{
							"const": query,
						},
					},
				},
			},
		},
	}

	schemaBytes, _ := json.Marshal(schemaMap)

	schemaFile, err := os.CreateTemp("", "schema*.json")
	if err != nil {
		return provider.ProviderEvaluateResponse{}, err
	}
	defer os.Remove(schemaFile.Name())

	if _, err := schemaFile.Write(schemaBytes); err != nil {
		return provider.ProviderEvaluateResponse{}, err
	}

	if err := schemaFile.Close(); err != nil {
		return provider.ProviderEvaluateResponse{}, err
	}

	fmt.Println(schemaFile.Name())

	// Send a workspace/didChangeConfiguration request
	sc.configParams = []map[string]any{
		{
			"schemas": map[string]any{
				// yaml-language-server has built-in kubernetes validation. Neat!
				// "kubernetes": examplesURI + "*",
				// examplesURI + "custom.json": examplesURI + "*",
				// "file:///home/jonah/Projects/analyzer-lsp/examples/yaml/custom.json": sc.BaseConfig.WorkspaceFolders[0] + "*",
				"file://" + schemaFile.Name(): sc.BaseConfig.WorkspaceFolders[0] + "*",
			},
			"validate": true,
		},
	}

	err = sc.Conn.Notify(ctx, "workspace/didChangeConfiguration", sc.configParams)
	if err != nil {
		return provider.ProviderEvaluateResponse{}, err
	}

	// Get all yaml files
	folder := strings.TrimPrefix(sc.Config.WorkspaceFolders[0], "file://")
	var yamlFiles []string
	err = filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && (filepath.Ext(path) == ".yaml" || filepath.Ext(path) == ".yml") {
			path = "file://" + path
			yamlFiles = append(yamlFiles, path)
		}

		return nil
	})

	if err != nil {
		return provider.ProviderEvaluateResponse{}, err
	}

	// Helper didOpen and didClose functions
	didOpen := func(uri string, text []byte) error {
		params := protocol.DidOpenTextDocumentParams{
			TextDocument: protocol.TextDocumentItem{
				URI:        uri,
				LanguageID: "yaml",
				Version:    0,
				Text:       string(text),
			},
		}
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

	// Process in batches of size BATCH_SIZE
	BATCH_SIZE := 32
	batchRight, batchLeft := 0, 0
	incidents := []provider.IncidentContext{}

	for batchRight < len(yamlFiles) {
		for batchRight-batchLeft < BATCH_SIZE && batchRight < len(yamlFiles) {
			text, err := os.ReadFile(yamlFiles[batchRight][7:])
			if err != nil {
				return provider.ProviderEvaluateResponse{}, err
			}

			fmt.Printf("didOpen %s\n", yamlFiles[batchRight])

			err = didOpen(yamlFiles[batchRight], text)
			if err != nil {
				return provider.ProviderEvaluateResponse{}, err
			}

			batchRight++
		}

		time.Sleep(2 * time.Second)

		// This function gets the diagnostics
		var res json.RawMessage
		err = sc.Conn.Call(ctx, "yaml/get/jsonSchema", yamlFiles[batchLeft]).Await(ctx, &res)
		if err != nil {
			return provider.ProviderEvaluateResponse{}, err
		}

		time.Sleep(2 * time.Second)

		for i := batchLeft; i < batchRight; i++ {
			diagnostics := sc.PublishDiagnosticsCache.Get(yamlFiles[i]).Await()
			if len(diagnostics) == 0 {
				continue
			}

			for _, diagnostic := range diagnostics {
				lineNumber := int(diagnostic.Range.Start.Line)
				incidents = append(incidents, provider.IncidentContext{
					FileURI:    uri.URI(yamlFiles[i]),
					LineNumber: &lineNumber,
				})
			}
		}

		for batchLeft < batchRight {
			if err = didClose(yamlFiles[batchLeft]); err != nil {
				return provider.ProviderEvaluateResponse{}, err
			}
			batchLeft++
		}
	}

	if len(incidents) == 0 {
		return provider.ProviderEvaluateResponse{Matched: false}, nil
	}
	return provider.ProviderEvaluateResponse{
		Matched:   true,
		Incidents: incidents,
	}, nil
}

package generic

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3gen"
	"github.com/go-logr/logr"
	base "github.com/konveyor/analyzer-lsp/lsp/base_service_client"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"gopkg.in/yaml.v2"
)

// **DELETE THIS COMMENT BLOCK FOR NEW SERVICE CLIENTS**
//
// Suppose the name of your language server is `foo-lsp`. The recommended
// pattern of adding new service clients is:
//
// 1. Create a new folder in `server_configurations` with the name of your lsp
//    server, `foo_lsp`. Copy `generic/service_client.go` into the new directory.
//
// 2. Change the package name to `foo_lsp`. Change the occurrences of
//    `GenericServiceClient` to `FooServiceClient`.
//
// 3. Add any variables you need to the `FooServiceClient` and
//    `FooServiceClientConfig` struct.
//
// 4. Modify any parameters related to the `initialize` request. If you need
//    additional `jsonrpc2_v2` handlers, say for responding to messages from the
//    server in a specific way, pass those into the base service client.
//
// 5. Implement your capabilities and update the `FooServiceClientCapabilities`
//    slice
//
// 6. In constants.go, add `NewFooServiceClient` to SupportedLanguages and
//    `FooServiceClientCapabilities` to SupportedCapabilities

type GenericServiceClientConfig struct {
	base.LSPServiceClientConfig `yaml:",inline"`
}

type GenericServiceClient struct {
	*base.LSPServiceClientBase
	*base.LSPServiceClientEvaluator[*GenericServiceClient]

	Config GenericServiceClientConfig
}

func NewGenericServiceClient(ctx context.Context, log logr.Logger, c provider.InitConfig) (provider.ServiceClient, error) {
	sc := &GenericServiceClient{}

	// Unmarshal the config
	b, _ := yaml.Marshal(c.ProviderSpecificConfig)
	err := yaml.Unmarshal(b, &sc.Config)
	if err != nil {
		return nil, fmt.Errorf("generic providerSpecificConfig Unmarshal error: %w", err)
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
		return nil, fmt.Errorf("base client initialization error: %w", err)
	}
	sc.LSPServiceClientBase = scBase

	// Initialize the fancy evaluator (dynamic dispatch ftw)
	eval, err := base.NewLspServiceClientEvaluator[*GenericServiceClient](sc, GenericServiceClientCapabilities)
	if err != nil {
		return nil, fmt.Errorf("lsp service client evaluator error: %w", err)
	}
	sc.LSPServiceClientEvaluator = eval

	return sc, nil
}

// Tidy aliases

type serviceClientFn = base.LSPServiceClientFunc[*GenericServiceClient]

func serviceClientTemplateContext(v any) openapi3.SchemaRef {
	r, _ := openapi3gen.NewSchemaRefForValue(v, nil)
	return *r
}

var GenericServiceClientCapabilities = []base.LSPServiceClientCapability{
	{
		Name:            "referenced",
		TemplateContext: serviceClientTemplateContext(base.ReferencedCondition{}),
		Fn:              serviceClientFn(base.EvaluateReferenced[*GenericServiceClient]),
	},
	{
		Name:            "dependency",
		TemplateContext: serviceClientTemplateContext(base.NoOpCondition{}),
		Fn:              serviceClientFn(base.EvaluateNoOp[*GenericServiceClient]),
	},
	{
		Name:            "echo",
		TemplateContext: serviceClientTemplateContext(echoCondition{}),
		Fn:              serviceClientFn((*GenericServiceClient).EvaluateEcho),
	},
}

// Example condition
type echoCondition struct {
	Echo struct {
		Input string `yaml:"input" json:"input"`
	} `yaml:"echo" json:"input"`
}

// Example evaluate
func (sc *GenericServiceClient) EvaluateEcho(ctx context.Context, cap string, info []byte) (provider.ProviderEvaluateResponse, error) {
	var cond echoCondition
	err := yaml.Unmarshal(info, &cond)
	if err != nil {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("error unmarshaling query info")
	}

	return provider.ProviderEvaluateResponse{
		Matched: true,
		Incidents: []provider.IncidentContext{
			{
				Variables: map[string]interface{}{
					"output": cond.Echo.Input,
				},
			},
		},
	}, nil
}

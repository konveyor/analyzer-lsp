package pylsp

import (
	"context"
	"encoding/json"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3gen"
	"github.com/go-logr/logr"
	base "github.com/konveyor/analyzer-lsp/lsp/base_service_client"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"gopkg.in/yaml.v2"
)

type PythonServiceClientConfig struct {
	base.LSPServiceClientConfig `yaml:",inline"`

	blah int `yaml:",inline"`
}

type PythonServiceClient struct {
	*base.LSPServiceClientBase
	*base.LSPServiceClientEvaluator[*PythonServiceClient]

	Config PythonServiceClientConfig
}

func NewPythonServiceClient(ctx context.Context, log logr.Logger, c provider.InitConfig) (provider.ServiceClient, error) {
	sc := &PythonServiceClient{}

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
	eval, err := base.NewLspServiceClientEvaluator[*PythonServiceClient](sc, PythonServiceClientCapabilities)
	if err != nil {
		return nil, err
	}
	sc.LSPServiceClientEvaluator = eval

	return sc, nil
}

// Tidy aliases

type serviceClientFn = base.LSPServiceClientFunc[*PythonServiceClient]

func serviceClientTemplateContext(v any) openapi3.SchemaRef {
	r, _ := openapi3gen.NewSchemaRefForValue(v, nil)
	return *r
}

var PythonServiceClientCapabilities = []base.LSPServiceClientCapability{
	{
		Name:            "referenced",
		TemplateContext: serviceClientTemplateContext(base.ReferencedCondition{}),
		Fn:              serviceClientFn(base.EvaluateReferenced[*PythonServiceClient]),
	},
	{
		Name:            "dependency",
		TemplateContext: serviceClientTemplateContext(base.NoOpCondition{}),
		Fn:              serviceClientFn(base.EvaluateNoOp[*PythonServiceClient]),
	},
}

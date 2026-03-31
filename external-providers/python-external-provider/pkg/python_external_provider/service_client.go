package python_external_provider

import (
	"context"
	"encoding/json"

	"github.com/go-logr/logr"
	base "github.com/konveyor/analyzer-lsp/lsp/base_service_client"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/progress"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/swaggest/openapi-go/openapi3"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
)

// Evolved from pre-split generic server_configurations (pylsp).
// for python-external-provider (generic tree unchanged — implementation plan Step 3).

type PythonServiceClientConfig struct {
	base.LSPServiceClientConfig `yaml:",inline"`

	blah int `yaml:",inline"`
}

type serviceClientFn = base.LSPServiceClientFunc[*PythonServiceClient]

type PythonServiceClient struct {
	*base.LSPServiceClientBase
	*base.LSPServiceClientEvaluator[*PythonServiceClient]

	Config PythonServiceClientConfig
}

type PythonServiceClientBuilder struct {
	Progress *progress.Progress
}

func (p *PythonServiceClientBuilder) Init(ctx context.Context, log logr.Logger, c provider.InitConfig) (provider.ServiceClient, error) {
	sc := &PythonServiceClient{}

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

	params.Capabilities = protocol.ClientCapabilities{}

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
		nil,
		p.Progress,
	)
	if err != nil {
		return nil, err
	}
	sc.LSPServiceClientBase = scBase

	eval, err := base.NewLspServiceClientEvaluator(sc, p.GetPythonServiceClientCapabilities(log))
	if err != nil {
		return nil, err
	}
	sc.LSPServiceClientEvaluator = eval

	return sc, nil
}

func (p *PythonServiceClientBuilder) GetPythonServiceClientCapabilities(log logr.Logger) []base.LSPServiceClientCapability {
	caps := []base.LSPServiceClientCapability{}
	r := openapi3.NewReflector()
	refCap, err := provider.ToProviderCap(r, log, base.ReferencedCondition{}, "referenced")
	if err != nil {
		log.Error(err, "unable to get referenced cap")
	} else {
		caps = append(caps, base.LSPServiceClientCapability{
			Capability: refCap,
			Fn:         serviceClientFn(base.EvaluateReferenced[*PythonServiceClient]),
		})
	}
	return caps
}

func (sc *PythonServiceClient) GetDependencies(ctx context.Context) (map[uri.URI][]*provider.Dep, error) {
	return nil, nil
}

func (sc *PythonServiceClient) GetDependenciesDAG(ctx context.Context) (map[uri.URI][]provider.DepDAGItem, error) {
	return nil, nil
}

package go_external_provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	base "github.com/konveyor/analyzer-lsp/lsp/base_service_client"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/progress"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/swaggest/openapi-go/openapi3"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
)

// Evolved from pre-split generic server_configurations (gopls / "generic").
// for the dedicated go-external-provider (no generic tree changes — see implementation plan Step 2).

type GoServiceClientConfig struct {
	base.LSPServiceClientConfig `yaml:",inline"`
}

type serviceClientFn = base.LSPServiceClientFunc[*GoServiceClient]

type GoServiceClient struct {
	*base.LSPServiceClientBase
	*base.LSPServiceClientEvaluator[*GoServiceClient]

	Config GoServiceClientConfig
}

type GoServiceClientBuilder struct {
	Progress *progress.Progress
}

func (g *GoServiceClientBuilder) Init(ctx context.Context, log logr.Logger, c provider.InitConfig) (provider.ServiceClient, error) {
	sc := &GoServiceClient{}

	b, _ := yaml.Marshal(c.ProviderSpecificConfig)
	err := yaml.Unmarshal(b, &sc.Config)
	if err != nil {
		return nil, fmt.Errorf("go provider ProviderSpecificConfig unmarshal error: %w", err)
	}

	params := protocol.InitializeParams{}

	if c.Location != "" {
		sc.Config.WorkspaceFolders = []string{string(uri.File(c.Location))}
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
		g.Progress,
	)
	if err != nil {
		return nil, fmt.Errorf("base client initialization error: %w", err)
	}
	sc.LSPServiceClientBase = scBase

	eval, err := base.NewLspServiceClientEvaluator[*GoServiceClient](sc, g.GetGoServiceClientCapabilities(log))
	if err != nil {
		return nil, fmt.Errorf("lsp service client evaluator error: %w", err)
	}
	sc.LSPServiceClientEvaluator = eval

	return sc, nil
}

func (g *GoServiceClientBuilder) GetGoServiceClientCapabilities(log logr.Logger) []base.LSPServiceClientCapability {
	caps := []base.LSPServiceClientCapability{}
	r := openapi3.NewReflector()
	refCap, err := provider.ToProviderCap(r, log, base.ReferencedCondition{}, "referenced")
	if err != nil {
		log.Error(err, "unable to get referenced cap")
	} else {
		caps = append(caps, base.LSPServiceClientCapability{
			Capability: refCap,
			Fn:         serviceClientFn(base.EvaluateReferenced[*GoServiceClient]),
		})
	}
	depCap, err := provider.ToProviderCap(r, log, base.NoOpCondition{}, "dependency")
	if err != nil {
		log.Error(err, "unable to get dependency cap")
	} else {
		caps = append(caps, base.LSPServiceClientCapability{
			Capability: depCap,
			Fn:         serviceClientFn(base.EvaluateNoOp[*GoServiceClient]),
		})
	}
	return caps
}

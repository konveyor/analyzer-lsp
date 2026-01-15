package server_configurations

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/external-providers/generic-external-provider/pkg/server_configurations/generic"
	"github.com/konveyor/analyzer-lsp/external-providers/generic-external-provider/pkg/server_configurations/nodejs"
	"github.com/konveyor/analyzer-lsp/external-providers/generic-external-provider/pkg/server_configurations/pylsp"
	yaml "github.com/konveyor/analyzer-lsp/external-providers/generic-external-provider/pkg/server_configurations/yaml_language_server"
	base "github.com/konveyor/analyzer-lsp/lsp/base_service_client"
	"github.com/konveyor/analyzer-lsp/provider"
)

const (
	GenericClient = "generic"
	PythonClient  = "pylsp"
	YamlClient    = "yaml_language_server"
	NodeClient    = "nodejs"
)

type ServiceClientBuilder interface {
	Init(context.Context, logr.Logger, provider.InitConfig) (provider.ServiceClient, error)
	GetGenericServiceClientCapabilities(log logr.Logger) []base.LSPServiceClientCapability
}

var SupportedLanguages = map[string]ServiceClientBuilder{
	// "":        generic.NewGenericServiceClient,
	GenericClient: &generic.GenericServiceClientBuilder{},
	PythonClient:  &pylsp.PythonServiceClientBuilder{},
	YamlClient:    &yaml.YamlServiceClientBuilder{},
	NodeClient:    &nodejs.NodeServiceClientBuilder{},
}

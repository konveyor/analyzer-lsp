package server_configurations

import (
	"context"

	"github.com/go-logr/logr"
	base "github.com/konveyor/analyzer-lsp/lsp/base_service_client"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/generic-external-provider/pkg/server_configurations/generic"
	"github.com/konveyor/generic-external-provider/pkg/server_configurations/nodejs"
	"github.com/konveyor/generic-external-provider/pkg/server_configurations/pylsp"
	yaml "github.com/konveyor/generic-external-provider/pkg/server_configurations/yaml_language_server"
)

type ServiceClientBuilder interface {
	Init(context.Context, logr.Logger, provider.InitConfig) (provider.ServiceClient, error)
	GetGenericServiceClientCapabilities(log logr.Logger) []base.LSPServiceClientCapability
}

var SupportedLanguages = map[string]ServiceClientBuilder{
	// "":        generic.NewGenericServiceClient,
	"generic":              &generic.GenericServiceClientBuilder{},
	"pylsp":                &pylsp.PythonServiceClientBuilder{},
	"yaml_language_server": &yaml.YamlServiceClientBuilder{},
	"nodejs":               &nodejs.NodeServiceClientBuilder{},
}

package server_configurations

import (
	"context"

	"github.com/go-logr/logr"
	base "github.com/konveyor/analyzer-lsp/lsp/base_service_client"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/generic-external-provider/pkg/server_configurations/generic"
	"github.com/konveyor/generic-external-provider/pkg/server_configurations/nodejs"
	"github.com/konveyor/generic-external-provider/pkg/server_configurations/pylsp"
	"github.com/konveyor/generic-external-provider/pkg/server_configurations/yaml_language_server"
)

type ServiceClientConstructor func(context.Context, logr.Logger, provider.InitConfig) (provider.ServiceClient, error)

var SupportedLanguages = map[string]ServiceClientConstructor{
	// "":        generic.NewGenericServiceClient,
	"generic":              generic.NewGenericServiceClient,
	"pylsp":                pylsp.NewPythonServiceClient,
	"yaml_language_server": yaml_language_server.NewYamlServiceClient,
	"nodejs":               nodejs.NewNodeServiceClient,
}

var SupportedCapabilities = map[string][]base.LSPServiceClientCapability{
	// "":        generic.GenericServiceClientCapabilities,
	"generic":              generic.GenericServiceClientCapabilities,
	"pylsp":                pylsp.PythonServiceClientCapabilities,
	"yaml_language_server": yaml_language_server.YamlServiceClientCapabilities,
	"nodejs":               nodejs.NodeServiceClientCapabilities,
}

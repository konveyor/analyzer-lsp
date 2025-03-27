package generic_external_provider

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	serverconf "github.com/konveyor/analyzer-lsp/external-providers/generic-external-provider/pkg/server_configurations"
	"github.com/konveyor/analyzer-lsp/external-providers/generic-external-provider/pkg/server_configurations/generic"
	"github.com/konveyor/analyzer-lsp/external-providers/generic-external-provider/pkg/server_configurations/nodejs"
	"github.com/konveyor/analyzer-lsp/external-providers/generic-external-provider/pkg/server_configurations/pylsp"
	"github.com/konveyor/analyzer-lsp/external-providers/generic-external-provider/pkg/server_configurations/yaml_language_server"
	"github.com/konveyor/analyzer-lsp/provider"
)

// TODO(shawn-hurley): Pipe the logger through Determine how and where external
// providers will add the logs to make the logs viewable in a single location.
//
// NOTE(jsussman): Should we change this name to "lspServerProvider"?
type genericProvider struct {
	ctx          context.Context
	capabilities []provider.Capability

	// Limit this instance of the generic provider to one lsp server type
	lspServerName        string
	serviceClientBuilder serverconf.ServiceClientBuilder
}

// Create a generic provider locked to a specific service client found in the
// server_configuration maps. If the lspServerName is not found, then it
// defaults to "generic"
func NewGenericProvider(lspServerName string, log logr.Logger) *genericProvider {
	// Get the constructor associated with the server
	ctor, ok := serverconf.SupportedLanguages[lspServerName]
	if !ok {
		lspServerName = "generic"
		ctor = serverconf.SupportedLanguages["generic"]
	}

	p := genericProvider{
		ctx:                  context.TODO(),
		lspServerName:        lspServerName,
		serviceClientBuilder: ctor,
	}

	// Load up the capabilities for this lsp server into the provider
	for _, cap := range ctor.GetGenericServiceClientCapabilities(log) {
		p.capabilities = append(p.capabilities, provider.Capability{
			Name:   cap.Name,
			Input:  cap.Input,
			Output: cap.Output,
		})
	}

	return &p
}

// Return the capabilities of the generic provider.
func (p *genericProvider) Capabilities() []provider.Capability {
	return p.capabilities
}

// Creates a new service client stemmed from the generic service provider. See
// "provider/grpc/ProviderInit()" for more info.
//
// NOTE(jsussman): With the current architecture, there's really tight coupling
// with the rest of the analyzer-lsp. For example, this Init() function returns
// a provider.ServiceClient to the original analyzer process. genericProvider
// here just sort of... doesn't matter at all
func (p *genericProvider) Init(ctx context.Context, log logr.Logger, c provider.InitConfig) (provider.ServiceClient, provider.InitConfig, error) {

	fmt.Fprintf(os.Stderr, "started generic provider init")
	lspServerName, ok := c.ProviderSpecificConfig["lspServerName"].(string)
	if !ok {
		lspServerName = "generic"
	}

	// because we spawn a generic client first, before knowing which service client we will need
	if p.lspServerName != lspServerName {
		// we want to be able to set which generic provider to use by tne provider config
		// because 'generic' is selected from the start, we need to update that if needed
		p.lspServerName = lspServerName
		// these have already been verified in NewGenericProvider() - no need to err
		switch p.lspServerName {
		case serverconf.GenericClient:
			p.serviceClientBuilder = &generic.GenericServiceClientBuilder{}
		case serverconf.PythonClient:
			p.serviceClientBuilder = &pylsp.PythonServiceClientBuilder{}
		case serverconf.NodeClient:
			p.serviceClientBuilder = &nodejs.NodeServiceClientBuilder{}
		case serverconf.YamlClient:
			p.serviceClientBuilder = &yaml_language_server.YamlServiceClientBuilder{}
		}

	}
	// Simple matter of calling the constructor that we set earlier to get the
	// service client
	sc, err := p.serviceClientBuilder.Init(ctx, log, c)

	if err != nil {
		log.Error(err, "ctor error")
		fmt.Fprintf(os.Stderr, "ctor blah")
		return nil, provider.InitConfig{}, fmt.Errorf("ctor error: %w", err)
	}

	return sc, provider.InitConfig{}, nil
}

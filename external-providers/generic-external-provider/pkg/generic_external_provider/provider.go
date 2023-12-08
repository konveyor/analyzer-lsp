package generic_external_provider

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/generic-external-provider/pkg/server_configurations"
)

// TODO(shawn-hurley): Pipe the logger through Determine how and where external
// providers will add the logs to make the logs viewable in a single location.
//
// NOTE(jsussman): Should we change this name to "lspServerProvider"?
type genericProvider struct {
	ctx          context.Context
	capabilities []provider.Capability

	// Limit this instance of the generic provider to one lsp server type
	lspServerName string
	ctor          server_configurations.ServiceClientConstructor
}

// Create a generic provider locked to a specific service client found in the
// server_configuration maps. If the lspServerName is not found, then it
// defaults to "generic"
func NewGenericProvider(lspServerName string) *genericProvider {
	// Get the constructor associated with the server
	ctor, ok := server_configurations.SupportedLanguages[lspServerName]
	if !ok {
		lspServerName = "generic"
		ctor = server_configurations.SupportedLanguages["generic"]
	}

	// Get the capabilities associated with the server
	caps, ok := server_configurations.SupportedCapabilities[lspServerName]
	if !ok || len(caps) == 0 {
		fmt.Printf("%s has no capabilities", lspServerName)
		lspServerName = "generic"
		ctor = server_configurations.SupportedLanguages["generic"]
		caps = server_configurations.SupportedCapabilities["generic"]
	}

	p := genericProvider{
		ctx:           context.TODO(),
		lspServerName: lspServerName,
		ctor:          ctor,
	}

	// Load up the capabilities for this lsp server into the provider
	for _, cap := range caps {
		p.capabilities = append(p.capabilities, provider.Capability{
			Name:            cap.Name,
			TemplateContext: cap.TemplateContext,
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
func (p *genericProvider) Init(ctx context.Context, log logr.Logger, c provider.InitConfig) (provider.ServiceClient, error) {
	// return nil, fmt.Errorf("nothing")

	log.Error(fmt.Errorf("Nothing"), "Started generic provider init")
	fmt.Fprintf(os.Stderr, "started generic provider init")
	lspServerName, ok := c.ProviderSpecificConfig["lspServerName"].(string)
	if !ok {
		lspServerName = "generic"
	}

	if p.lspServerName != lspServerName {
		log.Error(fmt.Errorf("lspServerName must be the same for each instantiation of the generic-external-provider (%s != %s)", p.lspServerName, lspServerName), "Inside genericProvider init")
		fmt.Fprintf(os.Stderr, "lspservername blah")

		return nil, fmt.Errorf("lspServerName must be the same for each instantiation of the generic-external-provider (%s != %s)", p.lspServerName, lspServerName)
	}

	// Simple matter of calling the constructor that we set earlier to get the
	// service client
	sc, err := p.ctor(ctx, log, c)
	if err != nil {
		log.Error(err, "ctor error")
		fmt.Fprintf(os.Stderr, "ctor blah")
		return nil, fmt.Errorf("ctor error: %w", err)
	}

	return sc, nil
}

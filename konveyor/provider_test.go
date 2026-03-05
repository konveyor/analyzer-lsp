package konveyor

import (
	"testing"

	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/stretchr/testify/assert"
)

func TestProvider_Capabilities(t *testing.T) {
	tests := []struct {
		name                 string
		mockCapabilities     []provider.Capability
		expectedCapabilities []provider.Capability
	}{
		{
			name: "single capability",
			mockCapabilities: []provider.Capability{
				{Name: "dependency"},
			},
			expectedCapabilities: []provider.Capability{
				{Name: "dependency"},
			},
		},
		{
			name: "multiple capabilities",
			mockCapabilities: []provider.Capability{
				{Name: "dependency"},
				{Name: "referenced"},
			},
			expectedCapabilities: []provider.Capability{
				{Name: "dependency"},
				{Name: "referenced"},
			},
		},
		{
			name:                 "no capabilities",
			mockCapabilities:     []provider.Capability{},
			expectedCapabilities: []provider.Capability{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Provider{
				Name: "test-provider",
				provider: &mockProviderClient{
					capabilities: tt.mockCapabilities,
				},
			}

			capabilities := p.Capabilities()
			assert.Equal(t, tt.expectedCapabilities, capabilities)
		})
	}
}

func TestProvider_SupportsFeature(t *testing.T) {
	p := &Provider{
		Name:     "test-provider",
		provider: &mockProviderClient{},
	}

	// Currently always returns false as per implementation
	result := p.SupportsFeature("dependency")
	assert.False(t, result)

	result = p.SupportsFeature("anything")
	assert.False(t, result)
}

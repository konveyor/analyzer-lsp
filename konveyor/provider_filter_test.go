package konveyor

import (
	"testing"

	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/stretchr/testify/assert"
)

func TestFilterByCapability(t *testing.T) {
	tests := []struct {
		name         string
		capability   string
		provider     Provider
		expected     bool
	}{
		{
			name:       "provider has dependency capability",
			capability: "dependency",
			provider: Provider{
				Name:     "java",
				provider: &mockProviderClient{
					capabilities: []provider.Capability{
						{Name: "dependency"},
					},
				},
			},
			expected: true,
		},
		{
			name:       "provider does not have capability",
			capability: "dependency",
			provider: Provider{
				Name:     "java",
				provider: &mockProviderClient{
					capabilities: []provider.Capability{
						{Name: "other"},
					},
				},
			},
			expected: false,
		},
		{
			name:       "provider has multiple capabilities including target",
			capability: "referenced",
			provider: Provider{
				Name:     "java",
				provider: &mockProviderClient{
					capabilities: []provider.Capability{
						{Name: "dependency"},
						{Name: "referenced"},
						{Name: "other"},
					},
				},
			},
			expected: true,
		},
		{
			name:       "provider has no capabilities",
			capability: "dependency",
			provider: Provider{
				Name:     "java",
				provider: &mockProviderClient{
					capabilities: []provider.Capability{},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := FilterByCapability(tt.capability)
			result := filter(tt.provider)
			assert.Equal(t, tt.expected, result)
		})
	}
}

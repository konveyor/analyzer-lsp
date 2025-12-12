package konveyor

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/stretchr/testify/assert"
)

func TestAnalyzer_RuleLabels(t *testing.T) {
	// Test with no ruleset
	analyzer := &analyzer{
		log: logr.Discard(),
	}
	labels := analyzer.RuleLabels()
	assert.Empty(t, labels)
}

func TestAnalyzer_RulesetFilepaths(t *testing.T) {
	// Test with no ruleset
	analyzer := &analyzer{
		log: logr.Discard(),
	}
	filepaths := analyzer.RulesetFilepaths()
	assert.Empty(t, filepaths)
}

func TestAnalyzer_GetProviderForLanguage(t *testing.T) {
	analyzer := &analyzer{
		log: logr.Discard(),
		allConfigProviders: map[string]provider.InternalProviderClient{
			"builtin": &mockProviderClient{},
			"java":    &mockProviderClient{},
		},
	}

	tests := []struct {
		name         string
		language     string
		expectFound  bool
		expectedName string
	}{
		{
			name:         "find builtin provider",
			language:     "builtin",
			expectFound:  true,
			expectedName: "builtin",
		},
		{
			name:         "find java provider",
			language:     "java",
			expectFound:  true,
			expectedName: "java",
		},
		{
			name:        "provider not found",
			language:    "go",
			expectFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prov, found := analyzer.GetProviderForLanguage(tt.language)

			assert.Equal(t, tt.expectFound, found)
			if tt.expectFound {
				assert.Equal(t, tt.expectedName, prov.Name)
			}
		})
	}
}

func TestAnalyzer_GetProviders(t *testing.T) {
	analyzer := &analyzer{
		log:       logr.Discard(),
		providers: []Provider{},
	}

	// Before parsing rules, providers list is empty
	providers := analyzer.GetProviders()
	assert.Empty(t, providers)
}

func TestAnalyzer_Run_WithoutRules(t *testing.T) {
	analyzer := &analyzer{
		log: logr.Discard(),
	}

	// Run without parsing rules should return nil
	rulesets := analyzer.Run()
	assert.Nil(t, rulesets)
}

func TestAnalyzer_Run_WithoutProviders(t *testing.T) {
	analyzer := &analyzer{
		log:     logr.Discard(),
		ruleset: []engine.RuleSet{{Name: "test"}},
	}

	// Run without providers should return nil
	rulesets := analyzer.Run()
	assert.Nil(t, rulesets)
}

func TestAnalyzer_ProviderStart(t *testing.T) {
	analyzer := &analyzer{
		log: logr.Discard(),
	}

	// ProviderStart without ParseRules should fail
	err := analyzer.ProviderStart()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no providers to start")
}

func TestAnalyzer_GetProviders_WithFilters(t *testing.T) {
	// Create analyzer with mock providers
	analyzer := &analyzer{
		providers: []Provider{
			{
				Name: "java",
				provider: &mockProviderClient{
					capabilities: []provider.Capability{
						{Name: "dependency"},
					},
				},
			},
			{
				Name: "go",
				provider: &mockProviderClient{
					capabilities: []provider.Capability{
						{Name: "referenced"},
					},
				},
			},
			{
				Name: "python",
				provider: &mockProviderClient{
					capabilities: []provider.Capability{
						{Name: "dependency"},
						{Name: "referenced"},
					},
				},
			},
		},
		log: logr.Discard(),
	}

	tests := []struct {
		name          string
		filters       []Filter
		expectedCount int
		expectedNames []string
	}{
		{
			name:          "no filters",
			filters:       []Filter{},
			expectedCount: 0,
			expectedNames: []string{},
		},
		{
			name: "filter by dependency capability",
			filters: []Filter{
				FilterByCapability("dependency"),
			},
			expectedCount: 2,
			expectedNames: []string{"java", "python"},
		},
		{
			name: "filter by referenced capability",
			filters: []Filter{
				FilterByCapability("referenced"),
			},
			expectedCount: 2,
			expectedNames: []string{"go", "python"},
		},
		{
			name: "filter by non-existent capability",
			filters: []Filter{
				FilterByCapability("non-existent"),
			},
			expectedCount: 0,
			expectedNames: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			providers := analyzer.GetProviders(tt.filters...)

			assert.Equal(t, tt.expectedCount, len(providers))

			names := make([]string, len(providers))
			for i, p := range providers {
				names[i] = p.Name
			}

			for _, expectedName := range tt.expectedNames {
				assert.Contains(t, names, expectedName)
			}
		})
	}
}

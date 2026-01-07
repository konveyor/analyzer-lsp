package konveyor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/stretchr/testify/assert"
)

func TestSetupProviderConfigs(t *testing.T) {
	tests := []struct {
		name                     string
		providerConfigs          []provider.Config
		setupTempDirs            func(t *testing.T) []string
		expectedConfigCount      int
		expectedLocationCount    int
		expectBuiltinConfig      bool
		expectedNonBuiltinCount  int
	}{
		{
			name: "empty provider configs",
			providerConfigs: []provider.Config{},
			setupTempDirs: func(t *testing.T) []string {
				return []string{}
			},
			expectedConfigCount:     0,
			expectedLocationCount:   0,
			expectBuiltinConfig:     false,
			expectedNonBuiltinCount: 0,
		},
		{
			name: "single non-builtin provider with valid location",
			providerConfigs: []provider.Config{
				{
					Name: "java",
					InitConfig: []provider.InitConfig{
						{
							Location: "", // will be set by setupTempDirs
						},
					},
				},
			},
			setupTempDirs: func(t *testing.T) []string {
				tmpDir := t.TempDir()
				subDir := filepath.Join(tmpDir, "java")
				err := os.MkdirAll(subDir, 0755)
				assert.NoError(t, err)
				return []string{subDir}
			},
			expectedConfigCount:     2, // java + builtin
			expectedLocationCount:   1,
			expectBuiltinConfig:     true,
			expectedNonBuiltinCount: 1,
		},
		{
			name: "multiple providers with same location",
			providerConfigs: []provider.Config{
				{
					Name: "java",
					InitConfig: []provider.InitConfig{
						{
							Location: "", // will be set by setupTempDirs
						},
					},
				},
				{
					Name: "go",
					InitConfig: []provider.InitConfig{
						{
							Location: "", // will be set to same as java
						},
					},
				},
			},
			setupTempDirs: func(t *testing.T) []string {
				tmpDir := t.TempDir()
				subDir := filepath.Join(tmpDir, "shared")
				err := os.MkdirAll(subDir, 0755)
				assert.NoError(t, err)
				return []string{subDir, subDir} // same location for both
			},
			// Note: Due to the current implementation of setupProviderConfigs,
			// builtin is added inside the loop, so we get: java, builtin, go, builtin
			expectedConfigCount:     4, // java + builtin + go + builtin
			expectedLocationCount:   1, // deduplicated location
			expectBuiltinConfig:     true,
			expectedNonBuiltinCount: 2,
		},
		{
			name: "provider with non-existent location",
			providerConfigs: []provider.Config{
				{
					Name: "java",
					InitConfig: []provider.InitConfig{
						{
							Location: "/non/existent/path",
						},
					},
				},
			},
			setupTempDirs: func(t *testing.T) []string {
				return []string{}
			},
			expectedConfigCount:     2, // java + builtin (even though location doesn't exist)
			expectedLocationCount:   0, // no valid locations
			expectBuiltinConfig:     true,
			expectedNonBuiltinCount: 1,
		},
		{
			name: "builtin provider config",
			providerConfigs: []provider.Config{
				{
					Name: "builtin",
					InitConfig: []provider.InitConfig{
						{
							Location: "",
							ProviderSpecificConfig: map[string]interface{}{
								"key": "value",
							},
						},
					},
				},
			},
			setupTempDirs: func(t *testing.T) []string {
				tmpDir := t.TempDir()
				subDir := filepath.Join(tmpDir, "builtin")
				err := os.MkdirAll(subDir, 0755)
				assert.NoError(t, err)
				return []string{subDir}
			},
			expectedConfigCount:     1, // only builtin
			expectedLocationCount:   1,
			expectBuiltinConfig:     true,
			expectedNonBuiltinCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup temp directories and update locations
			tempDirs := tt.setupTempDirs(t)
			for i := range tt.providerConfigs {
				for j := range tt.providerConfigs[i].InitConfig {
					if i < len(tempDirs) && tt.providerConfigs[i].InitConfig[j].Location == "" {
						tt.providerConfigs[i].InitConfig[j].Location = tempDirs[i]
					}
				}
			}

			configs, locations := setupProviderConfigs(tt.providerConfigs)

			assert.Equal(t, tt.expectedConfigCount, len(configs), "unexpected number of configs")
			assert.Equal(t, tt.expectedLocationCount, len(locations), "unexpected number of locations")

			// Check if builtin config exists
			hasBuiltin := false
			nonBuiltinCount := 0
			for _, cfg := range configs {
				if cfg.Name == "builtin" {
					hasBuiltin = true
				} else {
					nonBuiltinCount++
				}
			}
			assert.Equal(t, tt.expectBuiltinConfig, hasBuiltin, "builtin config presence mismatch")
			assert.Equal(t, tt.expectedNonBuiltinCount, nonBuiltinCount, "non-builtin config count mismatch")
		})
	}
}

func TestSetupProviderConfigs_LocationDeduplication(t *testing.T) {
	tmpDir := t.TempDir()
	location1 := filepath.Join(tmpDir, "location1")
	location2 := filepath.Join(tmpDir, "location2")

	err := os.MkdirAll(location1, 0755)
	assert.NoError(t, err)
	err = os.MkdirAll(location2, 0755)
	assert.NoError(t, err)

	providerConfigs := []provider.Config{
		{
			Name: "java",
			InitConfig: []provider.InitConfig{
				{Location: location1},
				{Location: location2},
				{Location: location1}, // duplicate
			},
		},
	}

	configs, locations := setupProviderConfigs(providerConfigs)

	// Should have java + builtin
	assert.Equal(t, 2, len(configs))
	// Should deduplicate locations
	assert.Equal(t, 2, len(locations))
	assert.Contains(t, locations, location1)
	assert.Contains(t, locations, location2)
}

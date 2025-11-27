package nodejs

import (
	"testing"

	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
)

func TestNormalizePathForComparison(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "file:// URI scheme",
			input:    "file:///project/src/App.tsx",
			expected: "/project/src/App.tsx",
		},
		{
			name:     "file: URI scheme",
			input:    "file:/project/src/App.tsx",
			expected: "/project/src/App.tsx",
		},
		{
			name:     "plain path",
			input:    "/project/src/App.tsx",
			expected: "/project/src/App.tsx",
		},
		{
			name:     "path with ..",
			input:    "/project/src/../src/App.tsx",
			expected: "/project/src/App.tsx",
		},
		{
			name:     "path with .",
			input:    "/project/./src/App.tsx",
			expected: "/project/src/App.tsx",
		},
		{
			name:     "windows-style path",
			input:    "file:///C:/project/src/App.tsx",
			expected: "/C:/project/src/App.tsx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizePathForComparison(tt.input)
			expected := tt.expected
			// On Windows, paths are normalized to lowercase
			if runtime.GOOS == "windows" {
				expected = strings.ToLower(expected)
			}
			if result != expected {
				t.Errorf("normalizePathForComparison(%q) = %q, want %q", tt.input, result, expected)
			}
		})
	}
}

func TestFilepathFiltering(t *testing.T) {
	// Create test incidents
	createIncident := func(path string) provider.IncidentContext {
		return provider.IncidentContext{
			FileURI: uri.URI(path),
		}
	}

	tests := []struct {
		name              string
		incidents         map[string]provider.IncidentContext
		includedFilepaths []string
		excludedFilepaths []string
		expectedCount     int
		expectedPaths     []string
	}{
		{
			name: "no filtering - all incidents included",
			incidents: map[string]provider.IncidentContext{
				"1": createIncident("file:///project/src/App.tsx"),
				"2": createIncident("file:///project/src/utils.ts"),
			},
			includedFilepaths: []string{},
			excludedFilepaths: []string{},
			expectedCount:     2,
			expectedPaths:     []string{"/project/src/App.tsx", "/project/src/utils.ts"},
		},
		{
			name: "included paths filtering",
			incidents: map[string]provider.IncidentContext{
				"1": createIncident("file:///project/src/App.tsx"),
				"2": createIncident("file:///project/src/utils.ts"),
				"3": createIncident("file:///project/test/App.test.tsx"),
			},
			includedFilepaths: []string{"/project/src/App.tsx"},
			excludedFilepaths: []string{},
			expectedCount:     1,
			expectedPaths:     []string{"/project/src/App.tsx"},
		},
		{
			name: "excluded paths filtering",
			incidents: map[string]provider.IncidentContext{
				"1": createIncident("file:///project/src/App.tsx"),
				"2": createIncident("file:///project/src/utils.ts"),
				"3": createIncident("file:///project/test/App.test.tsx"),
			},
			includedFilepaths: []string{},
			excludedFilepaths: []string{"/project/test/App.test.tsx"},
			expectedCount:     2,
			expectedPaths:     []string{"/project/src/App.tsx", "/project/src/utils.ts"},
		},
		{
			name: "both included and excluded paths",
			incidents: map[string]provider.IncidentContext{
				"1": createIncident("file:///project/src/App.tsx"),
				"2": createIncident("file:///project/src/utils.ts"),
				"3": createIncident("file:///project/src/config.ts"),
				"4": createIncident("file:///project/test/App.test.tsx"),
			},
			includedFilepaths: []string{
				"/project/src/App.tsx",
				"/project/src/utils.ts",
				"/project/src/config.ts",
			},
			excludedFilepaths: []string{"/project/src/config.ts"},
			expectedCount:     2,
			expectedPaths:     []string{"/project/src/App.tsx", "/project/src/utils.ts"},
		},
		{
			name: "no false positives - similar filenames",
			incidents: map[string]provider.IncidentContext{
				"1": createIncident("file:///project/src/Toolbar.tsx"),
				"2": createIncident("file:///project/src/ToolbarGroup.tsx"),
				"3": createIncident("file:///project/src/ToolbarItem.tsx"),
			},
			includedFilepaths: []string{"/project/src/Toolbar.tsx"},
			excludedFilepaths: []string{},
			expectedCount:     1,
			expectedPaths:     []string{"/project/src/Toolbar.tsx"},
		},
		{
			name: "URI normalization - different schemes match",
			incidents: map[string]provider.IncidentContext{
				"1": createIncident("file:///project/src/App.tsx"),
				"2": createIncident("file:/project/src/utils.ts"),
			},
			includedFilepaths: []string{
				"file:///project/src/App.tsx",  // Triple slash
				"/project/src/utils.ts",         // Plain path
			},
			excludedFilepaths: []string{},
			expectedCount:     2,
			expectedPaths:     []string{"/project/src/App.tsx", "/project/src/utils.ts"},
		},
		{
			name: "path cleaning - .. and . resolved",
			incidents: map[string]provider.IncidentContext{
				"1": createIncident("file:///project/src/../src/App.tsx"),
				"2": createIncident("file:///project/./src/utils.ts"),
			},
			includedFilepaths: []string{
				"/project/src/App.tsx",
				"/project/src/utils.ts",
			},
			excludedFilepaths: []string{},
			expectedCount:     2,
			expectedPaths:     []string{"/project/src/App.tsx", "/project/src/utils.ts"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build maps for O(1) lookups
			excludedPathsMap := make(map[string]bool, len(tt.excludedFilepaths))
			for _, excludedPath := range tt.excludedFilepaths {
				normalizedPath := normalizePathForComparison(excludedPath)
				excludedPathsMap[normalizedPath] = true
			}

			includedPathsMap := make(map[string]bool, len(tt.includedFilepaths))
			for _, includedPath := range tt.includedFilepaths {
				normalizedPath := normalizePathForComparison(includedPath)
				includedPathsMap[normalizedPath] = true
			}

			// Filter incidents
			filteredIncidents := []provider.IncidentContext{}
			for _, incident := range tt.incidents {
				normalizedIncidentPath := normalizePathForComparison(string(incident.FileURI))

				// Check if excluded
				if excludedPathsMap[normalizedIncidentPath] {
					continue
				}

				// Check if included
				if len(includedPathsMap) > 0 && !includedPathsMap[normalizedIncidentPath] {
					continue
				}

				filteredIncidents = append(filteredIncidents, incident)
			}

			// Verify count
			if len(filteredIncidents) != tt.expectedCount {
				t.Errorf("Expected %d incidents, got %d", tt.expectedCount, len(filteredIncidents))
			}

			// Verify expected paths are present
			foundPaths := make(map[string]bool)
			for _, incident := range filteredIncidents {
				normalizedPath := normalizePathForComparison(string(incident.FileURI))
				foundPaths[normalizedPath] = true
			}

			for _, expectedPath := range tt.expectedPaths {
				normalizedExpectedPath := normalizePathForComparison(expectedPath)
				if !foundPaths[normalizedExpectedPath] {
					t.Errorf("Expected path %q not found in filtered incidents", expectedPath)
				}
			}
		})
	}
}

func BenchmarkFilepathFiltering(b *testing.B) {
	// Create test data with many incidents and scoped paths
	incidents := make(map[string]provider.IncidentContext)
	for i := 0; i < 10000; i++ {
		path := uri.URI(uri.File("/project/src/file" + string(rune(i)) + ".tsx"))
		incidents[string(rune(i))] = provider.IncidentContext{FileURI: path}
	}

	includedPaths := make([]string, 100)
	for i := 0; i < 100; i++ {
		includedPaths[i] = "/project/src/file" + string(rune(i)) + ".tsx"
	}

	b.Run("with map optimization", func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			// Build maps
			includedPathsMap := make(map[string]bool, len(includedPaths))
			for _, includedPath := range includedPaths {
				normalizedPath := normalizePathForComparison(includedPath)
				includedPathsMap[normalizedPath] = true
			}

			// Filter
			filtered := []provider.IncidentContext{}
			for _, incident := range incidents {
				normalizedPath := normalizePathForComparison(string(incident.FileURI))
				if len(includedPathsMap) > 0 && !includedPathsMap[normalizedPath] {
					continue
				}
				filtered = append(filtered, incident)
			}
		}
	})

	b.Run("without map optimization (nested loops)", func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			// Filter with nested loops
			filtered := []provider.IncidentContext{}
			for _, incident := range incidents {
				normalizedIncidentPath := normalizePathForComparison(string(incident.FileURI))

				if len(includedPaths) > 0 {
					found := false
					for _, includedPath := range includedPaths {
						normalizedIncludedPath := normalizePathForComparison(includedPath)
						if normalizedIncidentPath == normalizedIncludedPath {
							found = true
							break
						}
					}
					if !found {
						continue
					}
				}

				filtered = append(filtered, incident)
			}
		}
	})
}

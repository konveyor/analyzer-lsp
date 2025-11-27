package dotnet

import (
	"testing"

	"go.lsp.dev/protocol"
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
			input:    "file:///project/src/Program.cs",
			expected: "/project/src/Program.cs",
		},
		{
			name:     "file: URI scheme",
			input:    "file:/project/src/Program.cs",
			expected: "/project/src/Program.cs",
		},
		{
			name:     "plain path",
			input:    "/project/src/Program.cs",
			expected: "/project/src/Program.cs",
		},
		{
			name:     "path with ..",
			input:    "/project/src/../src/Program.cs",
			expected: "/project/src/Program.cs",
		},
		{
			name:     "path with .",
			input:    "/project/./src/Program.cs",
			expected: "/project/src/Program.cs",
		},
		{
			name:     "windows-style path",
			input:    "file:///C:/project/src/Program.cs",
			expected: "/C:/project/src/Program.cs",
		},
		{
			name:     "csharp metadata URI",
			input:    "csharp:/metadata/projects/MyApp/assemblies/System.Web.Mvc/symbols/Controller.cs",
			expected: "csharp:/metadata/projects/MyApp/assemblies/System.Web.Mvc/symbols/Controller.cs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizePathForComparison(tt.input)
			if result != tt.expected {
				t.Errorf("normalizePathForComparison(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFilepathFiltering(t *testing.T) {
	// Create test locations
	createLocation := func(path string) protocol.Location {
		return protocol.Location{
			URI: uri.URI(path),
			Range: protocol.Range{
				Start: protocol.Position{Line: 10, Character: 5},
			},
		}
	}

	tests := []struct {
		name              string
		locations         []protocol.Location
		includedFilepaths []string
		excludedFilepaths []string
		expectedCount     int
		expectedPaths     []string
	}{
		{
			name: "no filtering - all locations included",
			locations: []protocol.Location{
				createLocation("file:///project/src/Program.cs"),
				createLocation("file:///project/src/Utils.cs"),
			},
			includedFilepaths: []string{},
			excludedFilepaths: []string{},
			expectedCount:     2,
			expectedPaths:     []string{"/project/src/Program.cs", "/project/src/Utils.cs"},
		},
		{
			name: "included paths filtering",
			locations: []protocol.Location{
				createLocation("file:///project/src/Program.cs"),
				createLocation("file:///project/src/Utils.cs"),
				createLocation("file:///project/tests/ProgramTests.cs"),
			},
			includedFilepaths: []string{"/project/src/Program.cs"},
			excludedFilepaths: []string{},
			expectedCount:     1,
			expectedPaths:     []string{"/project/src/Program.cs"},
		},
		{
			name: "excluded paths filtering",
			locations: []protocol.Location{
				createLocation("file:///project/src/Program.cs"),
				createLocation("file:///project/src/Utils.cs"),
				createLocation("file:///project/tests/ProgramTests.cs"),
			},
			includedFilepaths: []string{},
			excludedFilepaths: []string{"/project/tests/ProgramTests.cs"},
			expectedCount:     2,
			expectedPaths:     []string{"/project/src/Program.cs", "/project/src/Utils.cs"},
		},
		{
			name: "both included and excluded paths",
			locations: []protocol.Location{
				createLocation("file:///project/src/Program.cs"),
				createLocation("file:///project/src/Utils.cs"),
				createLocation("file:///project/src/Config.cs"),
				createLocation("file:///project/tests/ProgramTests.cs"),
			},
			includedFilepaths: []string{
				"/project/src/Program.cs",
				"/project/src/Utils.cs",
				"/project/src/Config.cs",
			},
			excludedFilepaths: []string{"/project/src/Config.cs"},
			expectedCount:     2,
			expectedPaths:     []string{"/project/src/Program.cs", "/project/src/Utils.cs"},
		},
		{
			name: "no false positives - similar filenames",
			locations: []protocol.Location{
				createLocation("file:///project/src/Service.cs"),
				createLocation("file:///project/src/ServiceImpl.cs"),
				createLocation("file:///project/src/ServiceFactory.cs"),
			},
			includedFilepaths: []string{"/project/src/Service.cs"},
			excludedFilepaths: []string{},
			expectedCount:     1,
			expectedPaths:     []string{"/project/src/Service.cs"},
		},
		{
			name: "URI normalization - different schemes match",
			locations: []protocol.Location{
				createLocation("file:///project/src/Program.cs"),
				createLocation("file:/project/src/Utils.cs"),
			},
			includedFilepaths: []string{
				"file:///project/src/Program.cs", // Triple slash
				"/project/src/Utils.cs",           // Plain path
			},
			excludedFilepaths: []string{},
			expectedCount:     2,
			expectedPaths:     []string{"/project/src/Program.cs", "/project/src/Utils.cs"},
		},
		{
			name: "path cleaning - .. and . resolved",
			locations: []protocol.Location{
				createLocation("file:///project/src/../src/Program.cs"),
				createLocation("file:///project/./src/Utils.cs"),
			},
			includedFilepaths: []string{
				"/project/src/Program.cs",
				"/project/src/Utils.cs",
			},
			excludedFilepaths: []string{},
			expectedCount:     2,
			expectedPaths:     []string{"/project/src/Program.cs", "/project/src/Utils.cs"},
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

			// Filter locations
			filteredLocations := []protocol.Location{}
			for _, loc := range tt.locations {
				normalizedPath := normalizePathForComparison(string(loc.URI))

				// Check if excluded
				if excludedPathsMap[normalizedPath] {
					continue
				}

				// Check if included
				if len(includedPathsMap) > 0 && !includedPathsMap[normalizedPath] {
					continue
				}

				filteredLocations = append(filteredLocations, loc)
			}

			// Verify count
			if len(filteredLocations) != tt.expectedCount {
				t.Errorf("Expected %d locations, got %d", tt.expectedCount, len(filteredLocations))
			}

			// Verify expected paths are present
			foundPaths := make(map[string]bool)
			for _, loc := range filteredLocations {
				normalizedPath := normalizePathForComparison(string(loc.URI))
				foundPaths[normalizedPath] = true
			}

			for _, expectedPath := range tt.expectedPaths {
				if !foundPaths[expectedPath] {
					t.Errorf("Expected path %q not found in filtered locations", expectedPath)
				}
			}
		})
	}
}

func BenchmarkFilepathFiltering(b *testing.B) {
	// Create test data with many locations and scoped paths
	locations := make([]protocol.Location, 10000)
	for i := 0; i < 10000; i++ {
		path := uri.URI("file:///project/src/File" + string(rune(i)) + ".cs")
		locations[i] = protocol.Location{
			URI: path,
			Range: protocol.Range{
				Start: protocol.Position{Line: 10, Character: 5},
			},
		}
	}

	includedPaths := make([]string, 100)
	for i := 0; i < 100; i++ {
		includedPaths[i] = "/project/src/File" + string(rune(i)) + ".cs"
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
			filtered := []protocol.Location{}
			for _, loc := range locations {
				normalizedPath := normalizePathForComparison(string(loc.URI))
				if len(includedPathsMap) > 0 && !includedPathsMap[normalizedPath] {
					continue
				}
				filtered = append(filtered, loc)
			}
		}
	})

	b.Run("without map optimization (nested loops)", func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			// Filter with nested loops
			filtered := []protocol.Location{}
			for _, loc := range locations {
				normalizedLocPath := normalizePathForComparison(string(loc.URI))

				if len(includedPaths) > 0 {
					found := false
					for _, includedPath := range includedPaths {
						normalizedIncludedPath := normalizePathForComparison(includedPath)
						if normalizedLocPath == normalizedIncludedPath {
							found = true
							break
						}
					}
					if !found {
						continue
					}
				}

				filtered = append(filtered, loc)
			}
		}
	})
}

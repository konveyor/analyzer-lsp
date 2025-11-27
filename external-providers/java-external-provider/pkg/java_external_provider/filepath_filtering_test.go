package java

import (
	"testing"

	"github.com/konveyor/analyzer-lsp/lsp/protocol"
)

func TestNormalizePathForComparison(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "file:// URI scheme",
			input:    "file:///project/src/Main.java",
			expected: "/project/src/Main.java",
		},
		{
			name:     "file: URI scheme",
			input:    "file:/project/src/Main.java",
			expected: "/project/src/Main.java",
		},
		{
			name:     "plain path",
			input:    "/project/src/Main.java",
			expected: "/project/src/Main.java",
		},
		{
			name:     "path with ..",
			input:    "/project/src/../src/Main.java",
			expected: "/project/src/Main.java",
		},
		{
			name:     "path with .",
			input:    "/project/./src/Main.java",
			expected: "/project/src/Main.java",
		},
		{
			name:     "windows-style path",
			input:    "file:///C:/project/src/Main.java",
			expected: "/C:/project/src/Main.java",
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
	// Create test workspace symbols
	createWorkspaceSymbol := func(path string) protocol.WorkspaceSymbol {
		sym := protocol.WorkspaceSymbol{
			Location: protocol.OrPLocation_workspace_symbol{
				Value: protocol.Location{
					URI: protocol.DocumentURI(path),
				},
			},
		}
		sym.BaseSymbolInformation.Name = "TestSymbol"
		return sym
	}

	tests := []struct {
		name              string
		symbols           []protocol.WorkspaceSymbol
		includedFilepaths []string
		excludedFilepaths []string
		expectedCount     int
		expectedPaths     []string
	}{
		{
			name: "no filtering - all symbols included",
			symbols: []protocol.WorkspaceSymbol{
				createWorkspaceSymbol("file:///project/src/Main.java"),
				createWorkspaceSymbol("file:///project/src/Utils.java"),
			},
			includedFilepaths: []string{},
			excludedFilepaths: []string{},
			expectedCount:     2,
			expectedPaths:     []string{"/project/src/Main.java", "/project/src/Utils.java"},
		},
		{
			name: "included paths filtering",
			symbols: []protocol.WorkspaceSymbol{
				createWorkspaceSymbol("file:///project/src/Main.java"),
				createWorkspaceSymbol("file:///project/src/Utils.java"),
				createWorkspaceSymbol("file:///project/test/MainTest.java"),
			},
			includedFilepaths: []string{"/project/src/Main.java"},
			excludedFilepaths: []string{},
			expectedCount:     1,
			expectedPaths:     []string{"/project/src/Main.java"},
		},
		{
			name: "excluded paths filtering",
			symbols: []protocol.WorkspaceSymbol{
				createWorkspaceSymbol("file:///project/src/Main.java"),
				createWorkspaceSymbol("file:///project/src/Utils.java"),
				createWorkspaceSymbol("file:///project/test/MainTest.java"),
			},
			includedFilepaths: []string{},
			excludedFilepaths: []string{"/project/test/MainTest.java"},
			expectedCount:     2,
			expectedPaths:     []string{"/project/src/Main.java", "/project/src/Utils.java"},
		},
		{
			name: "both included and excluded paths",
			symbols: []protocol.WorkspaceSymbol{
				createWorkspaceSymbol("file:///project/src/Main.java"),
				createWorkspaceSymbol("file:///project/src/Utils.java"),
				createWorkspaceSymbol("file:///project/src/Config.java"),
				createWorkspaceSymbol("file:///project/test/MainTest.java"),
			},
			includedFilepaths: []string{
				"/project/src/Main.java",
				"/project/src/Utils.java",
				"/project/src/Config.java",
			},
			excludedFilepaths: []string{"/project/src/Config.java"},
			expectedCount:     2,
			expectedPaths:     []string{"/project/src/Main.java", "/project/src/Utils.java"},
		},
		{
			name: "no false positives - similar filenames",
			symbols: []protocol.WorkspaceSymbol{
				createWorkspaceSymbol("file:///project/src/Service.java"),
				createWorkspaceSymbol("file:///project/src/ServiceImpl.java"),
				createWorkspaceSymbol("file:///project/src/ServiceFactory.java"),
			},
			includedFilepaths: []string{"/project/src/Service.java"},
			excludedFilepaths: []string{},
			expectedCount:     1,
			expectedPaths:     []string{"/project/src/Service.java"},
		},
		{
			name: "URI normalization - different schemes match",
			symbols: []protocol.WorkspaceSymbol{
				createWorkspaceSymbol("file:///project/src/Main.java"),
				createWorkspaceSymbol("file:/project/src/Utils.java"),
			},
			includedFilepaths: []string{
				"file:///project/src/Main.java", // Triple slash
				"/project/src/Utils.java",        // Plain path
			},
			excludedFilepaths: []string{},
			expectedCount:     2,
			expectedPaths:     []string{"/project/src/Main.java", "/project/src/Utils.java"},
		},
		{
			name: "path cleaning - .. and . resolved",
			symbols: []protocol.WorkspaceSymbol{
				createWorkspaceSymbol("file:///project/src/../src/Main.java"),
				createWorkspaceSymbol("file:///project/./src/Utils.java"),
			},
			includedFilepaths: []string{
				"/project/src/Main.java",
				"/project/src/Utils.java",
			},
			excludedFilepaths: []string{},
			expectedCount:     2,
			expectedPaths:     []string{"/project/src/Main.java", "/project/src/Utils.java"},
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

			// Filter symbols
			filteredSymbols := []protocol.WorkspaceSymbol{}
			for _, symbol := range tt.symbols {
				refLocation, ok := symbol.Location.Value.(protocol.Location)
				if !ok {
					continue
				}
				normalizedRefURI := normalizePathForComparison(string(refLocation.URI))

				// Check if excluded
				if excludedPathsMap[normalizedRefURI] {
					continue
				}

				// Check if included
				if len(includedPathsMap) > 0 && !includedPathsMap[normalizedRefURI] {
					continue
				}

				filteredSymbols = append(filteredSymbols, symbol)
			}

			// Verify count
			if len(filteredSymbols) != tt.expectedCount {
				t.Errorf("Expected %d symbols, got %d", tt.expectedCount, len(filteredSymbols))
			}

			// Verify expected paths are present
			foundPaths := make(map[string]bool)
			for _, symbol := range filteredSymbols {
				if refLocation, ok := symbol.Location.Value.(protocol.Location); ok {
					normalizedPath := normalizePathForComparison(string(refLocation.URI))
					foundPaths[normalizedPath] = true
				}
			}

			for _, expectedPath := range tt.expectedPaths {
				normalizedExpectedPath := normalizePathForComparison(expectedPath)
				if !foundPaths[normalizedExpectedPath] {
					t.Errorf("Expected path %q not found in filtered symbols", expectedPath)
				}
			}
		})
	}
}

func BenchmarkFilepathFiltering(b *testing.B) {
	// Create test data with many symbols and scoped paths
	symbols := make([]protocol.WorkspaceSymbol, 10000)
	for i := 0; i < 10000; i++ {
		path := protocol.DocumentURI("file:///project/src/File" + string(rune(i)) + ".java")
		sym := protocol.WorkspaceSymbol{
			Location: protocol.OrPLocation_workspace_symbol{
				Value: protocol.Location{
					URI: path,
				},
			},
		}
		sym.BaseSymbolInformation.Name = "TestSymbol"
		symbols[i] = sym
	}

	includedPaths := make([]string, 100)
	for i := 0; i < 100; i++ {
		includedPaths[i] = "/project/src/File" + string(rune(i)) + ".java"
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
			filtered := []protocol.WorkspaceSymbol{}
			for _, symbol := range symbols {
				if refLocation, ok := symbol.Location.Value.(protocol.Location); ok {
					normalizedPath := normalizePathForComparison(string(refLocation.URI))
					if len(includedPathsMap) > 0 && !includedPathsMap[normalizedPath] {
						continue
					}
					filtered = append(filtered, symbol)
				}
			}
		}
	})

	b.Run("without map optimization (nested loops)", func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			// Filter with nested loops
			filtered := []protocol.WorkspaceSymbol{}
			for _, symbol := range symbols {
				if refLocation, ok := symbol.Location.Value.(protocol.Location); ok {
					normalizedSymbolPath := normalizePathForComparison(string(refLocation.URI))

					if len(includedPaths) > 0 {
						found := false
						for _, includedPath := range includedPaths {
							normalizedIncludedPath := normalizePathForComparison(includedPath)
							if normalizedSymbolPath == normalizedIncludedPath {
								found = true
								break
							}
						}
						if !found {
							continue
						}
					}

					filtered = append(filtered, symbol)
				}
			}
		}
	})
}

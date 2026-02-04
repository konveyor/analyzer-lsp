package provider

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	logrusr "github.com/bombsimon/logrusr/v3"
	"github.com/sirupsen/logrus"
)

func TestMultilineGrep(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		pattern  string
		window   int
		want     int
		wantErr  bool
	}{
		{
			name:     "plain single line text",
			filePath: "./testdata/small.xml",
			pattern:  "xmlns:xsi=\"http://www.w3.org/2001/XMLSchema-instance\"",
			want:     2,
			window:   1,
			wantErr:  false,
		},
		{
			name:     "multi-line simple pattern",
			filePath: "./testdata/small.xml",
			pattern:  "com.fasterxml.jackson.core.*?jackson-core.*",
			want:     68,
			window:   2,
			wantErr:  false,
		},
		{
			name:     "multi-line complex pattern",
			filePath: "./testdata/small.xml",
			pattern:  "(<groupId>com.fasterxml.jackson.core</groupId>|<artifactId>jackson-core</artifactId>).*?(<artifactId>jackson-core</artifactId>|<groupId>com.fasterxml.jackson.core</groupId>).*",
			want:     68,
			window:   2,
			wantErr:  false,
		},
		{
			name:     "multi-line complex pattern",
			filePath: "./testdata/big.xml",
			pattern:  "(<groupId>io.konveyor.demo</groupId>|<artifactId>config-utils</artifactId>).*?(<artifactId>config-utils</artifactId>|<groupId>io.konveyor.demo</groupId>).*",
			want:     664,
			window:   2,
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MultilineGrep(context.TODO(), tt.window, tt.filePath, tt.pattern)
			if (err != nil) != tt.wantErr {
				t.Errorf("MultilineGrep() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("MultilineGrep() = %v, want %v", got, tt.want)
			}
		})
	}
}

func BenchmarkMultilineGrepFileSizeSmall(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ctx, canMe := context.WithTimeout(context.TODO(), time.Second*3)
		MultilineGrep(ctx, 5,
			"./testdata/small.xml",
			"(<groupId>com.fasterxml.jackson.core</groupId>|<artifactId>jackson-core</artifactId>).*?(<artifactId>jackson-core</artifactId>|<groupId>com.fasterxml.jackson.core</groupId>).*")
		canMe()
	}
}

func BenchmarkMultilineGrepFileSizeBig(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ctx, canMe := context.WithTimeout(context.TODO(), time.Second*3)
		MultilineGrep(ctx, 5,
			"./testdata/big.xml",
			"(<groupId>io.konveyor.demo</groupId>|<artifactId>config-utils</artifactId>).*?(<artifactId>config-utils</artifactId>|<groupId>io.konveyor.demo</groupId>).*")
		canMe()
	}
}

func TestGetExcludedDirsFromConfig(t *testing.T) {
	// Note: .git and .venv are regex-escaped to match literal directory names
	defaultExcludes := []string{
		"node_modules",
		"vendor",
		"\\.git",
		"dist",
		"build",
		"target",
		"\\.venv",
		"venv",
	}

	tests := []struct {
		name       string
		initConfig InitConfig
		want       []string
	}{
		{
			name: "no user config - returns defaults only",
			initConfig: InitConfig{
				Location:               "/project",
				ProviderSpecificConfig: map[string]interface{}{},
			},
			want: defaultExcludes,
		},
		{
			name: "empty array - clears all defaults",
			initConfig: InitConfig{
				Location: "/project",
				ProviderSpecificConfig: map[string]interface{}{
					ExcludedDirsConfigKey: []interface{}{},
				},
			},
			want: []string{},
		},
		{
			name: "user provides relative directory names - keeps them as-is",
			initConfig: InitConfig{
				Location: "/project",
				ProviderSpecificConfig: map[string]interface{}{
					ExcludedDirsConfigKey: []interface{}{
						"bower_components",
						"jspm_packages",
					},
				},
			},
			want: append(defaultExcludes, "bower_components", "jspm_packages"),
		},
		{
			name: "user provides absolute paths - keeps them as-is",
			initConfig: InitConfig{
				Location: "/project",
				ProviderSpecificConfig: map[string]interface{}{
					ExcludedDirsConfigKey: []interface{}{
						"/absolute/path/to/exclude",
						"/another/absolute/path",
					},
				},
			},
			want: append(defaultExcludes, "/absolute/path/to/exclude", "/another/absolute/path"),
		},
		{
			name: "mix of relative and absolute paths",
			initConfig: InitConfig{
				Location: "/project",
				ProviderSpecificConfig: map[string]interface{}{
					ExcludedDirsConfigKey: []interface{}{
						"bower_components",
						"/absolute/path/to/specific/dir",
						"custom_vendor",
					},
				},
			},
			want: append(defaultExcludes, "bower_components", "/absolute/path/to/specific/dir", "custom_vendor"),
		},
		{
			name: "nested relative paths - keeps them as directory patterns",
			initConfig: InitConfig{
				Location: "/project",
				ProviderSpecificConfig: map[string]interface{}{
					ExcludedDirsConfigKey: []interface{}{
						"src/generated",
						"test/fixtures",
					},
				},
			},
			want: append(defaultExcludes, "src/generated", "test/fixtures"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetExcludedDirsFromConfig(tt.initConfig)
			if len(got) != len(tt.want) {
				t.Errorf("GetExcludedDirsFromConfig() returned %d items, want %d items", len(got), len(tt.want))
				t.Errorf("got: %v", got)
				t.Errorf("want: %v", tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("GetExcludedDirsFromConfig()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

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
		{
			name:     "csharp metadata URI",
			input:    "csharp:/metadata/projects/MyApp/assemblies/System.Web.Mvc/symbols/Controller.cs",
			expected: "csharp:/metadata/projects/MyApp/assemblies/System.Web.Mvc/symbols/Controller.cs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizePathForComparison(tt.input)
			expected := tt.expected
			// On Windows, paths are normalized to lowercase (except csharp: URIs)
			if runtime.GOOS == "windows" && !strings.HasPrefix(tt.input, "csharp:") {
				expected = strings.ToLower(expected)
			}
			if result != expected {
				t.Errorf("NormalizePathForComparison(%q) = %q, want %q", tt.input, result, expected)
			}
		})
	}
}

func TestFileSearcherWithPatternOnly(t *testing.T) {
	// This test reproduces the bug where filePattern parameter finds zero files
	// Setup logger
	logrusLog := logrusr.New(logrus.New())

	// Create a FileSearcher with the test directory
	fs := FileSearcher{
		BasePath:        "./testdata",
		AdditionalPaths: []string{},
		ProviderConfigConstraints: IncludeExcludeConstraints{
			IncludePathsOrPatterns: []string{},
			ExcludePathsOrPatterns: []string{"node_modules", "vendor", ".git"},
		},
		RuleScopeConstraints: IncludeExcludeConstraints{
			IncludePathsOrPatterns: nil,
			ExcludePathsOrPatterns: nil,
		},
		FailFast: true,
		Log:      logrusLog,
	}

	// Test 1: Search with pattern only (reproduces the bug)
	t.Run("Pattern only - BUG", func(t *testing.T) {
		result, err := fs.Search(SearchCriteria{
			Patterns:           []string{`\.([jt])sx?$`},
			ConditionFilepaths: nil,
		})
		if err != nil {
			t.Errorf("Search failed: %v", err)
		}
		t.Logf("Found %d files with pattern only", len(result))
		for _, file := range result {
			t.Logf("  - %s", file)
		}
		if len(result) == 0 {
			t.Error("BUG CONFIRMED: Expected to find at least test.jsx, but got 0 files")
		}
	})

	// Test 2: Search with filepath only (workaround that works)
	t.Run("Filepath only - WORKS", func(t *testing.T) {
		result, err := fs.Search(SearchCriteria{
			Patterns:           []string{},
			ConditionFilepaths: []string{"test.jsx"},
		})
		if err != nil {
			t.Errorf("Search failed: %v", err)
		}
		t.Logf("Found %d files with filepath only", len(result))
		for _, file := range result {
			t.Logf("  - %s", file)
		}
		if len(result) == 0 {
			t.Error("Expected to find test.jsx, but got 0 files")
		}
	})

	// Test 3: Search with pattern using absolute path (like production)
	t.Run("Pattern with absolute path", func(t *testing.T) {
		// Get absolute path to testdata
		absPath, err := filepath.Abs("./testdata")
		if err != nil {
			t.Fatalf("Failed to get absolute path: %v", err)
		}

		fsAbs := FileSearcher{
			BasePath:        absPath,
			AdditionalPaths: []string{},
			ProviderConfigConstraints: IncludeExcludeConstraints{
				IncludePathsOrPatterns: []string{},
				// Note: User patterns are treated as regex, so .git must be escaped
				ExcludePathsOrPatterns: []string{"node_modules", "vendor", "\\.git"},
			},
			RuleScopeConstraints: IncludeExcludeConstraints{
				IncludePathsOrPatterns: nil,
				ExcludePathsOrPatterns: nil,
			},
			FailFast: true,
			Log:      logrusLog,
		}

		result, err := fsAbs.Search(SearchCriteria{
			Patterns:           []string{`\.([jt])sx?$`},
			ConditionFilepaths: nil,
		})
		if err != nil {
			t.Errorf("Search failed: %v", err)
		}
		t.Logf("Found %d files with pattern and absolute path", len(result))
		for _, file := range result {
			t.Logf("  - %s", file)
		}
		if len(result) == 0 {
			t.Error("BUG with absolute path: Expected to find at least test.jsx, but got 0 files")
		}
	})

	t.Run("User-provided patterns are treated as regex", func(t *testing.T) {
		// User-provided exclude patterns are treated as regex patterns.
		// This means ".example" will match paths containing any-char + "example" (e.g., "theexample").
		// If a user wants to match a literal ".example" directory, they should escape it: "\\.example"
		// Only default excludes (.git, .venv) are pre-escaped in GetExcludedDirsFromConfig.
		absPath, err := filepath.Abs("./testdata")
		if err != nil {
			t.Fatalf("Failed to get absolute path: %v", err)
		}
		t.Logf("Test running on GOOS=%s, absPath=%s", runtime.GOOS, absPath)

		// Test 1: Unescaped ".example" matches paths containing "theexample" (regex behavior)
		fsUnescaped := FileSearcher{
			BasePath:        absPath,
			AdditionalPaths: []string{},
			ProviderConfigConstraints: IncludeExcludeConstraints{
				IncludePathsOrPatterns: []string{},
				ExcludePathsOrPatterns: []string{".example"}, // Regex: . matches any char
			},
			RuleScopeConstraints: IncludeExcludeConstraints{
				IncludePathsOrPatterns: nil,
				ExcludePathsOrPatterns: nil,
			},
			FailFast: true,
			Log:      logrusLog,
		}

		resultUnescaped, err := fsUnescaped.Search(SearchCriteria{
			Patterns:           []string{`\.([jt])sx?$`},
			ConditionFilepaths: nil,
		})
		if err != nil {
			t.Errorf("Search failed: %v", err)
		}

		// With unescaped ".example", files in "theexample/" should be excluded (regex matches)
		foundTheExampleUnescaped := false
		for _, file := range resultUnescaped {
			if strings.Contains(filepath.Dir(file), "theexample") && filepath.Base(file) == "component.jsx" {
				foundTheExampleUnescaped = true
			}
		}
		if foundTheExampleUnescaped {
			t.Errorf("Expected .example to exclude theexample/ (regex behavior), but file was found")
		}

		// Test 2: Escaped "\\.example" only matches literal ".example" directory
		fsEscaped := FileSearcher{
			BasePath:        absPath,
			AdditionalPaths: []string{},
			ProviderConfigConstraints: IncludeExcludeConstraints{
				IncludePathsOrPatterns: []string{},
				ExcludePathsOrPatterns: []string{"\\.example"}, // Escaped: matches literal .example
			},
			RuleScopeConstraints: IncludeExcludeConstraints{
				IncludePathsOrPatterns: nil,
				ExcludePathsOrPatterns: nil,
			},
			FailFast: true,
			Log:      logrusLog,
		}

		resultEscaped, err := fsEscaped.Search(SearchCriteria{
			Patterns:           []string{`\.([jt])sx?$`},
			ConditionFilepaths: nil,
		})
		if err != nil {
			t.Errorf("Search failed: %v", err)
		}

		// With escaped "\\.example", files in "theexample/" should NOT be excluded
		foundTheExampleEscaped := false
		for _, file := range resultEscaped {
			if strings.Contains(filepath.Dir(file), "theexample") && filepath.Base(file) == "component.jsx" {
				foundTheExampleEscaped = true
				t.Logf("Correctly found file in theexample with escaped pattern: %s", file)
			}
		}
		if !foundTheExampleEscaped {
			t.Errorf("Expected \\.example to NOT exclude theexample/, but file was not found. Got %d files: %v", len(resultEscaped), resultEscaped)
		}
	})
}

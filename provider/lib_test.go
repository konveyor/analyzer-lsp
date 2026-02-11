package provider

import (
	"context"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"
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
	defaultExcludes := []string{
		"node_modules",
		"vendor",
		".git",
		"dist",
		"build",
		"target",
		".venv",
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

func TestFileSearcher_Search(t *testing.T) {
	testBasePath, err := filepath.Abs("./testdata/file-search")
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	tests := []struct {
		name                      string
		basePath                  string
		additionalPaths           []string
		providerConfigConstraints IncludeExcludeConstraints
		ruleScopeConstraints      IncludeExcludeConstraints
		searchCriteria            SearchCriteria
		failFast                  bool
		wantFilePatterns          []string // patterns to match in results
		wantErr                   bool
		wantErrContains           string
	}{
		{
			name:     "search all files with no constraints",
			basePath: testBasePath,
			searchCriteria: SearchCriteria{
				Patterns:           []string{},
				ConditionFilepaths: []string{},
			},
			wantFilePatterns: []string{
				".mvn/maven-wrapper.jar",
				"src/main.go",
				"src/helper.go",
				"src/test.txt",
				"lib/util.go",
				"lib/common.go",
				"README.md",
				"config.yaml",
				"vendor/dep.go",
				"node_modules/package.json",
			},
			wantErr: false,
		},
		{
			name:     "search with pattern for .go files",
			basePath: testBasePath,
			searchCriteria: SearchCriteria{
				Patterns:           []string{`.*\.go$`},
				ConditionFilepaths: []string{},
			},
			wantFilePatterns: []string{
				"src/main.go",
				"src/helper.go",
				"lib/util.go",
				"lib/common.go",
				"vendor/dep.go",
			},
			wantErr: false,
		},
		{
			name:     "search with wildcard pattern",
			basePath: testBasePath,
			searchCriteria: SearchCriteria{
				Patterns:           []string{"*.yaml"},
				ConditionFilepaths: []string{},
			},
			wantFilePatterns: []string{
				"config.yaml",
			},
			wantErr: false,
		},
		{
			name:     "search with provider config include path",
			basePath: testBasePath,
			providerConfigConstraints: IncludeExcludeConstraints{
				IncludePathsOrPatterns: []string{filepath.Join(testBasePath, "src")},
				ExcludePathsOrPatterns: []string{},
			},
			searchCriteria: SearchCriteria{
				Patterns:           []string{},
				ConditionFilepaths: []string{},
			},
			wantFilePatterns: []string{
				"src/main.go",
				"src/helper.go",
				"src/test.txt",
			},
			wantErr: false,
		},
		{
			name:     "search with provider config exclude pattern",
			basePath: testBasePath,
			providerConfigConstraints: IncludeExcludeConstraints{
				IncludePathsOrPatterns: []string{},
				ExcludePathsOrPatterns: []string{"vendor", "node_modules"},
			},
			searchCriteria: SearchCriteria{
				Patterns:           []string{},
				ConditionFilepaths: []string{},
			},
			wantFilePatterns: []string{
				"src/main.go",
				"src/helper.go",
				"src/test.txt",
				"lib/util.go",
				"lib/common.go",
				"README.md",
				"config.yaml",
				".mvn/maven-wrapper.jar",
			},
			wantErr: false,
		},
		{
			name:     "rule scope constraints override provider config",
			basePath: testBasePath,
			providerConfigConstraints: IncludeExcludeConstraints{
				IncludePathsOrPatterns: []string{filepath.Join(testBasePath, "src")},
				ExcludePathsOrPatterns: []string{},
			},
			ruleScopeConstraints: IncludeExcludeConstraints{
				IncludePathsOrPatterns: []string{filepath.Join(testBasePath, "lib")},
				ExcludePathsOrPatterns: []string{},
			},
			searchCriteria: SearchCriteria{
				Patterns:           []string{},
				ConditionFilepaths: []string{},
			},
			wantFilePatterns: []string{
				"lib/util.go",
				"lib/common.go",
			},
			wantErr: false,
		},
		{
			name:     "search criteria patterns filter results",
			basePath: testBasePath,
			providerConfigConstraints: IncludeExcludeConstraints{
				IncludePathsOrPatterns: []string{filepath.Join(testBasePath, "src")},
				ExcludePathsOrPatterns: []string{},
			},
			searchCriteria: SearchCriteria{
				Patterns:           []string{`.*\.go$`},
				ConditionFilepaths: []string{},
			},
			wantFilePatterns: []string{
				"src/main.go",
				"src/helper.go",
			},
			wantErr: false,
		},
		{
			name:     "search criteria with condition filepaths",
			basePath: testBasePath,
			searchCriteria: SearchCriteria{
				Patterns:           []string{},
				ConditionFilepaths: []string{"*.go"},
			},
			wantFilePatterns: []string{
				"src/main.go",
				"src/helper.go",
				"lib/util.go",
				"lib/common.go",
				"vendor/dep.go",
			},
			wantErr: false,
		},
		{
			name:     "search criteria condition filepaths intersect with includes",
			basePath: testBasePath,
			providerConfigConstraints: IncludeExcludeConstraints{
				IncludePathsOrPatterns: []string{filepath.Join(testBasePath, "src")},
				ExcludePathsOrPatterns: []string{},
			},
			searchCriteria: SearchCriteria{
				Patterns:           []string{},
				ConditionFilepaths: []string{"main.go"},
			},
			wantFilePatterns: []string{
<<<<<<< HEAD
<<<<<<< HEAD
				"src/main.go",
=======
				"main.go",
			},
			wantErr: false,
		},
		{
			name:     "exclude pattern with regex",
			basePath: testBasePath,
			providerConfigConstraints: IncludeExcludeConstraints{
				IncludePathsOrPatterns: []string{},
				ExcludePathsOrPatterns: []string{`.*\.txt$`},
			},
			searchCriteria: SearchCriteria{
				Patterns:           []string{},
				ConditionFilepaths: []string{},
			},
			wantFilePatterns: []string{
				"main.go",
				"helper.go",
				"util.go",
				"common.go",
				"README.md",
				"config.yaml",
				"src/main.go",
			},
			wantErr: false,
		},
		{
			name:     "include pattern with wildcard",
			basePath: testBasePath,
			providerConfigConstraints: IncludeExcludeConstraints{
				IncludePathsOrPatterns: []string{"*.md"},
				ExcludePathsOrPatterns: []string{},
			},
			searchCriteria: SearchCriteria{
				Patterns:           []string{},
				ConditionFilepaths: []string{},
			},
			wantFilePatterns: []string{
				"README.md",
			},
			wantErr: false,
		},
		{
			name:     "rule scope exclude overrides provider config",
			basePath: testBasePath,
			providerConfigConstraints: IncludeExcludeConstraints{
				IncludePathsOrPatterns: []string{},
				ExcludePathsOrPatterns: []string{"vendor"},
			},
			ruleScopeConstraints: IncludeExcludeConstraints{
				IncludePathsOrPatterns: []string{},
				ExcludePathsOrPatterns: []string{filepath.Join(testBasePath, "src")},
			},
			searchCriteria: SearchCriteria{
				Patterns:           []string{},
				ConditionFilepaths: []string{},
			},
			wantFilePatterns: []string{
				"lib/util.go",
				"lib/common.go",
				"README.md",
				"vendor/dep.go",
				"config.yaml",
				".mvn/maven-wrapper.jar",
				"node_modules/package.json",
			},
			wantErr: false,
		},
		{
			name:     "multiple patterns in search criteria",
			basePath: testBasePath,
			searchCriteria: SearchCriteria{
				Patterns:           []string{`.*\.go$`, `.*\.txt$`},
				ConditionFilepaths: []string{},
			},
			wantFilePatterns: []string{
				"src/main.go",
				"src/helper.go",
				"lib/util.go",
				"lib/common.go",
				"src/test.txt",
				"vendor/dep.go",
			},
			wantErr: false,
		},
		{
			name:     "space-separated condition filepaths",
			basePath: testBasePath,
			searchCriteria: SearchCriteria{
				Patterns:           []string{},
				ConditionFilepaths: []string{"main.go helper.go"},
			},
			wantFilePatterns: []string{
				"src/main.go",
				"src/helper.go",
			},
			wantErr: false,
		},
		{
			name:     "jar file search",
			basePath: testBasePath,
			providerConfigConstraints: IncludeExcludeConstraints{
				IncludePathsOrPatterns: []string{},
				ExcludePathsOrPatterns: []string{"node_modules", "vendor", ".git", "dist", "build", "target", ".venv", "venv"},
			},
			searchCriteria: SearchCriteria{
				Patterns:           []string{".*hazelcast.*\\.jar$"},
				ConditionFilepaths: []string{""},
			},
			wantFilePatterns: []string{},
			wantErr:          false,
		},
		{
			name:     "jar file search app cat",
			basePath: testBasePath,
			providerConfigConstraints: IncludeExcludeConstraints{
				IncludePathsOrPatterns: []string{},
				ExcludePathsOrPatterns: []string{"node_modules", "vendor", ".git", "dist", "build", "target", ".venv", "venv"},
			},
			searchCriteria: SearchCriteria{
				Patterns:           []string{`(/|\\)([a-zA-Z0-9._-]+)\.jar$`},
				ConditionFilepaths: []string{""},
			},
			wantFilePatterns: []string{".mvn/maven-wrapper.jar"},
			wantErr:          false,
		},
		{
			name:     "top level file search app cat",
			basePath: testBasePath,
			providerConfigConstraints: IncludeExcludeConstraints{
				IncludePathsOrPatterns: []string{},
				ExcludePathsOrPatterns: []string{"node_modules", "vendor", ".git", "dist", "build", "target", ".venv", "venv"},
			},
			searchCriteria: SearchCriteria{
				Patterns:           []string{`(/|\\)([a-zA-Z0-9._-]+)\.md$`},
				ConditionFilepaths: []string{""},
			},
			wantFilePatterns: []string{"README.md"},
			wantErr:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testr := testr.NewWithOptions(t, testr.Options{
				Verbosity: 10,
			})
			searcher := &FileSearcher{
				BasePath:                  tt.basePath,
				AdditionalPaths:           tt.additionalPaths,
				ProviderConfigConstraints: tt.providerConfigConstraints,
				RuleScopeConstraints:      tt.ruleScopeConstraints,
				FailFast:                  tt.failFast,
				Log:                       testr,
			}

			got, err := searcher.Search(tt.searchCriteria)

			// Check error expectations
			if tt.wantErr {
				if err == nil {
					t.Errorf("FileSearcher.Search() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if tt.wantErrContains != "" && !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Errorf("FileSearcher.Search() error = %v, want error containing %q", err, tt.wantErrContains)
				}
				return
			}

			if err != nil {
				t.Errorf("FileSearcher.Search() unexpected error = %v", err)
				return
			}

			if len(got) != len(tt.wantFilePatterns) {
				t.Errorf("received different number of filepaths then expected got: %v but expected: %v", len(got), len(tt.wantFilePatterns))
			}

			// Check that all expected file patterns are found in results
			for _, pattern := range tt.wantFilePatterns {
				found := slices.Contains(got, filepath.Join(tt.basePath, pattern))
				if !found {
					t.Errorf("FileSearcher.Search() missing expected file pattern %q in results: %v", pattern, got)
				}
			}

			// Check that results don't contain excluded files
			// Determine which exclusion constraints are actually in effect
			activeExcludes := tt.providerConfigConstraints.ExcludePathsOrPatterns
			if len(tt.ruleScopeConstraints.ExcludePathsOrPatterns) > 0 {
				// Rule scope overrides provider config
				activeExcludes = tt.ruleScopeConstraints.ExcludePathsOrPatterns
			}

			for _, file := range got {
				// Verify excluded directories/patterns are not in results
				for _, exclude := range activeExcludes {
					if strings.Contains(file, exclude) {
						// Check if this is a path-based exclusion
						if strings.Contains(exclude, "/") {
							// It's a path, check if file is under that path
							if strings.HasPrefix(file, exclude) {
								t.Errorf("FileSearcher.Search() result contains excluded file from path %q: %v", exclude, file)
							}
						} else {
							// It's a directory name or pattern, check if it appears in the path
							parts := strings.Split(file, string(filepath.Separator))
							for _, part := range parts {
								if part == exclude {
									t.Errorf("FileSearcher.Search() result contains excluded file from pattern %q: %v", exclude, file)
									break
								}
							}
						}
					}
				}
			}
		})
	}
}

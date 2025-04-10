package provider

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/engine/labels"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"go.lsp.dev/uri"
)

var _ Client = &fakeClient{}

type fakeClient struct {
	dependencies []*Dep
}

func (c *fakeClient) Capabilities() []Capability { return nil }
func (c *fakeClient) HasCapability(string) bool  { return true }
func (c *fakeClient) Evaluate(context.Context, string, []byte) (ProviderEvaluateResponse, error) {
	return ProviderEvaluateResponse{}, nil
}
func (c *fakeClient) Init(context.Context, logr.Logger, InitConfig) (ServiceClient, InitConfig, error) {
	return nil, InitConfig{}, nil
}
func (c *fakeClient) Stop() {}
func (c *fakeClient) NotifyFileChanges(ctx context.Context, changes ...FileChange) error {
	return nil
}
func (c *fakeClient) GetDependencies(ctx context.Context) (map[uri.URI][]*Dep, error) {
	m := map[uri.URI][]*Dep{
		uri.URI("test"): c.dependencies,
	}
	return m, nil
}

func (c *fakeClient) GetDependenciesDAG(ctx context.Context) (map[uri.URI][]DepDAGItem, error) {
	return nil, nil
}

func Test_dependencyConditionEvaluation(t *testing.T) {
	tests := []struct {
		title        string
		name         string
		upperbound   string
		lowerbound   string
		dependencies []*Dep
		shouldMatch  bool
		shouldErr    bool
	}{
		{
			title:        "no matching dependency should return no match",
			name:         "DNE",
			upperbound:   "10.0",
			dependencies: []*Dep{{Name: "DE", Version: "v4.0.0"}},
		},
		{
			title:        "A existing dependency that falls within the bounds should match",
			name:         "DE",
			upperbound:   "4.0.2",
			lowerbound:   "4.0.0",
			dependencies: []*Dep{{Name: "DE", Version: "v4.0.1"}},
			shouldMatch:  true,
		},
		{
			title:        "A existing dependency that falls above the lowerbound should match",
			name:         "DE",
			lowerbound:   "3.0.1",
			dependencies: []*Dep{{Name: "DE", Version: "v4.0.0"}},
			shouldMatch:  true,
		},
		{
			title:        "A existing dependency that falls below the upperbound should match",
			name:         "DE",
			upperbound:   "4.2.1",
			dependencies: []*Dep{{Name: "DE", Version: "v4.0.0"}},
			shouldMatch:  true,
		},
		{
			title:        "A existing dependency that falls outside the bounds should not match",
			name:         "DE",
			upperbound:   "3.0",
			lowerbound:   "0",
			dependencies: []*Dep{{Name: "DE", Version: "v4.0.0"}},
			shouldMatch:  false,
		},
		{
			title:        "A existing dependency that falls below the lowerbound should not match",
			name:         "DE",
			lowerbound:   "v5.10.7",
			dependencies: []*Dep{{Name: "DE", Version: "v4.0.0"}},
			shouldMatch:  false,
		},
		{
			title:        "A existing dependency that falls above the upperbound should not match",
			name:         "DE",
			upperbound:   "v5.10.7",
			dependencies: []*Dep{{Name: "DE", Version: "72.13.4788"}},
			shouldMatch:  false,
		},
		{
			title:        "Invalid versions should error",
			name:         "DE",
			upperbound:   "3.0",
			lowerbound:   "0",
			dependencies: []*Dep{{Name: "DE", Version: "seventeen point six"}},
			shouldErr:    true,
		},
		{
			title:        "Invalid constraints should error",
			name:         "DE",
			upperbound:   "3.0",
			lowerbound:   "zero point 10",
			dependencies: []*Dep{{Name: "DE", Version: "10.0.0"}},
			shouldErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			depCondition := DependencyCondition{
				DependencyConditionCap: DependencyConditionCap{
					Name:       tt.name,
					Upperbound: tt.upperbound,
					Lowerbound: tt.lowerbound,
				},
				Client: &fakeClient{dependencies: tt.dependencies},
			}

			resp, err := depCondition.Evaluate(context.TODO(), logr.Logger{}, engine.ConditionContext{})
			if err != nil {
				if !tt.shouldErr {
					t.Error(err)
				} else {
					return
				}
			}
			if resp.Matched != tt.shouldMatch {
				t.Errorf("Evaluating the dependency %s with bounds [ lower: %s , upper: %s ] did not match expected result", tt.name, tt.lowerbound, tt.upperbound)
			}

		})
	}

}

func Test_matchDepLabelSelector(t *testing.T) {
	tests := []struct {
		name          string
		labelSelector string
		incident      IncidentContext
		deps          map[uri.URI][]*konveyor.Dep
		want          bool
		wantErr       bool
	}{
		{
			name:          "no deps, incident should match",
			labelSelector: "!konveyor.io/dep-source=open-source",
			incident: IncidentContext{
				FileURI: "file://test-file-uri",
			},
			want: true,
		},
		{
			name:          "incident does not come from a dep, should match",
			labelSelector: "!konveyor.io/dep-source=open-source",
			incident:      IncidentContext{},
			deps: map[uri.URI][]*konveyor.Dep{
				"pom.xml": {
					{
						Name:               "test-dep",
						Version:            "0.5.2",
						ResolvedIdentifier: "sha256",
						Labels: []string{
							"konveyor.io/dep-source=open-source",
						},
						FileURIPrefix: "file://test-file-uri",
					},
				},
			},
			want: true,
		},
		{
			name:          "label selector matches",
			labelSelector: "konveyor.io/dep-source=open-source",
			incident: IncidentContext{
				FileURI:              "file://test-file-uri/test-file",
				IsDependencyIncident: true,
			},
			deps: map[uri.URI][]*konveyor.Dep{
				"pom.xml": {
					{
						Name:               "test-dep",
						Version:            "0.5.2",
						ResolvedIdentifier: "sha256",
						Labels: []string{
							"konveyor.io/dep-source=open-source",
						},
						FileURIPrefix: "file://test-file-uri",
					},
				},
			},
			want: true,
		},
		{
			name:          "label selector does not match",
			labelSelector: "!konveyor.io/dep-source=exclude",
			incident: IncidentContext{
				FileURI:              "file://test-file-uri/test-file",
				IsDependencyIncident: true,
			},
			deps: map[uri.URI][]*konveyor.Dep{
				"pom.xml": {
					{
						Name:               "test-dep",
						Version:            "0.5.2",
						ResolvedIdentifier: "sha256",
						Labels: []string{
							"konveyor.io/dep-source=exclude",
						},
						FileURIPrefix: "file://test-file-uri",
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labelSelector, err := labels.NewLabelSelector[*Dep](tt.labelSelector, nil)
			if err != nil {
				t.Errorf("invalid label selector %s", tt.labelSelector)
				return
			}
			got, err := matchDepLabelSelector(labelSelector, tt.incident, tt.deps)
			if (err != nil) != tt.wantErr {
				t.Errorf("matchDepLabelSelector() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("matchDepLabelSelector() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_deduplication(t *testing.T) {
	tests := []struct {
		title        string
		dependencies map[uri.URI][]*Dep
		expected     map[uri.URI][]*Dep
	}{
		{
			title: "no duplicates within a file should result in an unchanged list",
			dependencies: map[uri.URI][]*Dep{
				uri.URI("file1"): {
					{Name: "dep1", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
					{Name: "dep2", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
				},
				uri.URI("file2"): {
					{Name: "dep1", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
					{Name: "dep2", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
				},
			},
			expected: map[uri.URI][]*Dep{
				uri.URI("file1"): {
					{Name: "dep1", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
					{Name: "dep2", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
				},
				uri.URI("file2"): {
					{Name: "dep1", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
					{Name: "dep2", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
				},
			},
		},
		{
			title: "different versions or shas of the same dependency should not be deduped",
			dependencies: map[uri.URI][]*Dep{
				uri.URI("file1"): {
					{Name: "dep1", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
					{Name: "dep1", Version: "v2.0.0", ResolvedIdentifier: "abcd"},
					{Name: "dep2", Version: "v1.0.0", ResolvedIdentifier: "abcde"},
					{Name: "dep2", Version: "v1.0.0", ResolvedIdentifier: "abcdf"},
				},
			},
			expected: map[uri.URI][]*Dep{
				uri.URI("file1"): {
					{Name: "dep1", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
					{Name: "dep1", Version: "v2.0.0", ResolvedIdentifier: "abcd"},
					{Name: "dep2", Version: "v1.0.0", ResolvedIdentifier: "abcde"},
					{Name: "dep2", Version: "v1.0.0", ResolvedIdentifier: "abcdf"},
				},
			},
		},
		{
			title: "duplicates within a file should be removed",
			dependencies: map[uri.URI][]*Dep{
				uri.URI("file1"): {
					{Name: "dep1", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
					{Name: "dep1", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
				},
			},
			expected: map[uri.URI][]*Dep{
				uri.URI("file1"): {
					{Name: "dep1", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
				},
			},
		},
		{
			title: "direct dependencies should be preferred over indirect",
			dependencies: map[uri.URI][]*Dep{
				uri.URI("file1"): {
					{Name: "dep1", Version: "v1.0.0", ResolvedIdentifier: "abcd", Indirect: true},
					{Name: "dep1", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
				},
			},
			expected: map[uri.URI][]*Dep{
				uri.URI("file1"): {
					{Name: "dep1", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			deduped := deduplicateDependencies(tt.dependencies)
			for uri, deps := range tt.expected {
				for i, dep := range deps {
					if !reflect.DeepEqual(deduped[uri][i], dep) {
						t.Errorf("Expected '%+v', got '%+v'", tt.expected, deduped)
					}
				}
			}
		})
	}

}

func Test_GetConfigs(t *testing.T) {
	tests := []struct {
		title                          string
		testdataFile                   string
		expectedProviderSpecificConfig map[string]interface{}
		shouldErr                      bool
	}{
		{
			title:        "testnested",
			testdataFile: "testdata/provider_settings_nested_types.json",
			expectedProviderSpecificConfig: map[string]interface{}{
				"lspServerName":                  "generic",
				"lspServerPath":                  "/root/go/bin/gopls",
				"lspServerArgs":                  []interface{}{"string"},
				"lspServerInitializationOptions": "",
				"workspaceFolders":               []interface{}{"file:///analyzer-lsp/examples/golang"},
				"dependencyFolders":              []interface{}{},
				"groupVersionKinds": []interface{}{
					map[string]interface{}{"group": "apps", "version": "v1", "kind": "Deployment"},
				},
				"object":                 map[string]interface{}{"nestedObject": "object"},
				"dependencyProviderPath": "/usr/bin/golang-dependency-provider",
			},
		},
		{
			title:        "test nested yaml",
			testdataFile: "testdata/provider_settings_simple.yaml",
			expectedProviderSpecificConfig: map[string]interface{}{
				"lspServerName":                  "generic",
				"lspServerPath":                  "/root/go/bin/gopls",
				"lspServerArgs":                  []interface{}{"string"},
				"lspServerInitializationOptions": "",
				"workspaceFolders":               []interface{}{"file:///analyzer-lsp/examples/golang"},
				"dependencyFolders":              []interface{}{},
				"groupVersionKinds": []interface{}{
					map[string]interface{}{"group": "apps", "version": "v1", "kind": "Deployment"},
				},
				"object":                 map[string]interface{}{"nestedObject": "object"},
				"dependencyProviderPath": "/usr/bin/golang-dependency-provider",
			},
		},
		{
			title:        "test yaml int keys",
			testdataFile: "testdata/provider_settings_invalid.yaml",
			shouldErr:    true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.title, func(t *testing.T) {
			config, err := GetConfig(tc.testdataFile)
			if err != nil && !tc.shouldErr {
				t.Fatalf("got error: %v", err)
			}
			if err != nil && tc.shouldErr {
				return
			}
			// This is true because of the builtin config that will be added if not there
			if len(config) != 1 {
				t.Fatalf("got config not equal to one: %v", len(config))
			}
			c := config[0]
			if len(c.InitConfig) != 1 {
				t.Fatalf("got init config longer than one: %v", len(c.InitConfig))
			}
			pc := c.InitConfig[0]
			if !reflect.DeepEqual(pc.ProviderSpecificConfig, tc.expectedProviderSpecificConfig) {
				fmt.Printf("\\n%#v", pc.ProviderSpecificConfig)
				fmt.Printf("\\n%#v\\n", tc.expectedProviderSpecificConfig)
				t.Fatalf("Got config is different than expected config")
			}
		})
	}
}

func TestProviderContext_GetScopedFilepaths(t *testing.T) {
	tests := []struct {
		name       string
		template   map[string]engine.ChainTemplate
		inputPaths []string
		want       []string
	}{
		{
			name: "tc-0: only included filepaths present in context, must return that list as-is",
			template: map[string]engine.ChainTemplate{
				engine.TemplateContextPathScopeKey: {
					Filepaths: []string{"a/", "b/", "c/"},
				},
			},
			inputPaths: []string{},
			want:       []string{"a/", "b/", "c/"},
		},
		{
			name: "tc-1: included paths present in context, a list of additional paths provided as input, no exclusion, must return union of two lists",
			template: map[string]engine.ChainTemplate{
				engine.TemplateContextPathScopeKey: {
					Filepaths: []string{"a/", "b/"},
				},
			},
			inputPaths: []string{"c/"},
			want:       []string{"a/", "b/", "c/"},
		},
		{
			name: "tc-2: included paths present in context, a list of additional paths provided as input, and an exclusion list present, must return correctly filtered list",
			template: map[string]engine.ChainTemplate{
				engine.TemplateContextPathScopeKey: {
					Filepaths: []string{"a/", "b/c/", "b/c/a.java", "d/e/f.py"},
					ExcludedPaths: []string{
						"b/c/",
					},
				},
			},
			inputPaths: []string{"c/p.xml"},
			want:       []string{"a/", "d/e/f.py", "c/p.xml"},
		},
		{
			name:       "tc-3: no included or excluded paths present in context, must return input paths as-is",
			template:   map[string]engine.ChainTemplate{},
			inputPaths: []string{"a/", "b/", "c/"},
			want:       []string{"a/", "b/", "c/"},
		},
		{
			name: "tc-4: no included or excluded paths, must return input paths as-is",
			template: map[string]engine.ChainTemplate{
				engine.TemplateContextPathScopeKey: {
					ExcludedPaths: []string{},
				},
			},
			inputPaths: []string{"a/", "b/", "c/"},
			want:       []string{"a/", "b/", "c/"},
		},
		{
			name: "tc-5: included and excluded paths given but no input paths, must return correct list of included paths with excluded ones removed",
			template: map[string]engine.ChainTemplate{
				engine.TemplateContextPathScopeKey: {
					Filepaths:     []string{"a/b.py", "c/d/e/f.java", "l/m/n/p.py"},
					ExcludedPaths: []string{".*e.*"},
				},
			},
			inputPaths: []string{},
			want:       []string{"a/b.py", "l/m/n/p.py"},
		},
		{
			name: "tc-6: excluded paths provided with input paths, windows specific case",
			template: map[string]engine.ChainTemplate{
				engine.TemplateContextPathScopeKey: {
					ExcludedPaths: []string{"D:\\a\\analyzer-lsp\\analyzer-lsp\\provider\\internal\\builtin\\testdata\\search_scopes\\dir_a"},
				},
			},
			inputPaths: []string{
				"D:\\a\\analyzer-lsp\\analyzer-lsp\\provider\\internal\\builtin\\testdata\\search_scopes\\dir_a\\a.properties",
				"D:\\a\\analyzer-lsp\\analyzer-lsp\\provider\\internal\\builtin\\testdata\\search_scopes\\dir_a\\dir_b\\ab.properties",
				"D:\\a\\analyzer-lsp\\analyzer-lsp\\provider\\internal\\builtin\\testdata\\search_scopes\\dir_b\\b.properties",
				"D:\\a\\analyzer-lsp\\analyzer-lsp\\provider\\internal\\builtin\\testdata\\search_scopes\\dir_b\\dir_a\\ba.properties",
			},
			want: []string{
				"D:\\a\\analyzer-lsp\\analyzer-lsp\\provider\\internal\\builtin\\testdata\\search_scopes\\dir_b\\b.properties",
				"D:\\a\\analyzer-lsp\\analyzer-lsp\\provider\\internal\\builtin\\testdata\\search_scopes\\dir_b\\dir_a\\ba.properties",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &ProviderContext{
				Template: tt.template,
				RuleID:   "test",
			}
			if _, got := p.GetScopedFilepaths(tt.inputPaths...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ProviderContext.FilterExcludedPaths() = %v, want %v", got, tt.want)
			}
		})
	}
}

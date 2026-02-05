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
	dependencies   []*Dep
	evaluateResp   ProviderEvaluateResponse
	evaluateErr    error
	evaluateCalled bool
}

func (c *fakeClient) Prepare(ctx context.Context, conditionsByCap []ConditionsByCap) error {
	return nil
}
func (c *fakeClient) Capabilities() []Capability { return nil }
func (c *fakeClient) HasCapability(string) bool  { return true }
func (c *fakeClient) Evaluate(context.Context, string, []byte) (ProviderEvaluateResponse, error) {
	c.evaluateCalled = true
	return c.evaluateResp, c.evaluateErr
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

func Test_templateCondition(t *testing.T) {
	tests := []struct {
		name      string
		condition []byte
		ctx       map[string]engine.ChainTemplate
		want      []byte
		wantErr   bool
	}{
		{
			name:      "simple condition with no variables",
			condition: []byte("xml:\n  xpath: //dependencies/dependency"),
			ctx:       map[string]engine.ChainTemplate{},
			want:      []byte("xml:\n  xpath: //dependencies/dependency"),
			wantErr:   false,
		},
		{
			name:      "condition with quoted variables should have quotes removed",
			condition: []byte("xml:\n  filepaths: '{{poms.filepaths}}'\n  xpath: //dependencies/dependency"),
			ctx: map[string]engine.ChainTemplate{
				"poms": {
					Filepaths: []string{"pom.xml", "subdir/pom.xml"},
				},
			},
			want:    []byte("xml:\n  filepaths: [pom.xml subdir/pom.xml]\n  xpath: //dependencies/dependency"),
			wantErr: false,
		},
		{
			name:      "condition with single variable replacement",
			condition: []byte("xml:\n  filepaths: {{poms.filepaths}}\n  xpath: //dependencies/dependency"),
			ctx: map[string]engine.ChainTemplate{
				"poms": {
					Filepaths: []string{"pom.xml"},
				},
			},
			want:    []byte("xml:\n  filepaths: [pom.xml]\n  xpath: //dependencies/dependency"),
			wantErr: false,
		},
		{
			name:      "condition with multiple variables",
			condition: []byte("xml:\n  filepaths: {{poms.filepaths}}\n  excludedPaths: {{poms.excludedPaths}}\n  xpath: //dependencies/dependency"),
			ctx: map[string]engine.ChainTemplate{
				"poms": {
					Filepaths:     []string{"pom.xml", "build.xml"},
					ExcludedPaths: []string{"target/", "build/"},
				},
			},
			want:    []byte("xml:\n  filepaths: [pom.xml build.xml]\n  excludedPaths: [target/ build/]\n  xpath: //dependencies/dependency"),
			wantErr: false,
		},
		{
			name:      "condition with extras map",
			condition: []byte("custom:\n  config: {{settings.extras}}"),
			ctx: map[string]engine.ChainTemplate{
				"settings": {
					Extras: map[string]interface{}{
						"key1": "value1",
						"key2": 42,
					},
				},
			},
			want:    []byte("custom:\n  config: map[key1:value1 key2:42]"),
			wantErr: false,
		},
		{
			name:      "empty condition",
			condition: []byte(""),
			ctx:       map[string]engine.ChainTemplate{},
			want:      []byte(""),
			wantErr:   false,
		},
		{
			name:      "empty context",
			condition: []byte("xml:\n  xpath: //dependencies/dependency"),
			ctx:       nil,
			want:      []byte("xml:\n  xpath: //dependencies/dependency"),
			wantErr:   false,
		},
		{
			name:      "condition with undefined variable",
			condition: []byte("xml:\n  filepaths: {{undefined.filepaths}}"),
			ctx:       map[string]engine.ChainTemplate{},
			want:      []byte("xml:\n  filepaths: "),
			wantErr:   false,
		},
		{
			name:      "condition with nested template references",
			condition: []byte("search:\n  files: {{sources.filepaths}}\n  exclude: {{sources.excludedPaths}}"),
			ctx: map[string]engine.ChainTemplate{
				"sources": {
					Filepaths:     []string{"src/main.go", "src/utils.go"},
					ExcludedPaths: []string{"vendor/", "test/"},
				},
			},
			want:    []byte("search:\n  files: [src/main.go src/utils.go]\n  exclude: [vendor/ test/]"),
			wantErr: false,
		},
		{
			name:      "condition with empty ChainTemplate fields",
			condition: []byte("xml:\n  filepaths: {{empty.filepaths}}\n  extras: {{empty.extras}}"),
			ctx: map[string]engine.ChainTemplate{
				"empty": {},
			},
			want:    []byte("xml:\n  filepaths: []\n  extras: map[]"),
			wantErr: false,
		},
		{
			name:      "condition with multiple ChainTemplates",
			condition: []byte("config:\n  javaPaths: {{java.filepaths}}\n  goPaths: {{golang.filepaths}}"),
			ctx: map[string]engine.ChainTemplate{
				"java": {
					Filepaths: []string{"src/Main.java"},
				},
				"golang": {
					Filepaths: []string{"main.go"},
				},
			},
			want:    []byte("config:\n  javaPaths: [src/Main.java]\n  goPaths: [main.go]"),
			wantErr: false,
		},
		{
			name:      "condition with both quoted and unquoted variables",
			condition: []byte("xml:\n  quoted: '{{poms.filepaths}}'\n  unquoted: {{poms.filepaths}}"),
			ctx: map[string]engine.ChainTemplate{
				"poms": {
					Filepaths: []string{"pom.xml"},
				},
			},
			want:    []byte("xml:\n  quoted: [pom.xml]\n  unquoted: [pom.xml]"),
			wantErr: false,
		},
		{
			name:      "condition with special characters in paths",
			condition: []byte("xml:\n  filepaths: {{special.filepaths}}"),
			ctx: map[string]engine.ChainTemplate{
				"special": {
					Filepaths: []string{"path/with spaces/file.xml", "path-with-dashes/file.xml"},
				},
			},
			want:    []byte("xml:\n  filepaths: [path/with spaces/file.xml path-with-dashes/file.xml]"),
			wantErr: false,
		},
		{
			name:      "condition accessing specific extras value",
			condition: []byte("setting: {{config.extras.timeout}}"),
			ctx: map[string]engine.ChainTemplate{
				"config": {
					Extras: map[string]interface{}{
						"timeout": 30,
						"retries": 3,
					},
				},
			},
			want:    []byte("setting: 30"),
			wantErr: false,
		},
		{
			name:      "regex pattern with escapes",
			condition: []byte(`pattern: \".*\testing"`),
			ctx:       map[string]engine.ChainTemplate{},
			want:      []byte(`pattern: \".*\testing"`),
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := templateCondition(tt.condition, tt.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("templateCondition() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("templateCondition() = %q, want %q", string(got), string(tt.want))
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
				"lspServerPath":                  "/usr/local/bin/gopls",
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
				"lspServerPath":                  "/usr/local/bin/gopls",
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

func Test_ProviderCondition_Evaluate(t *testing.T) {
	lineNum := 10
	message := "test message"

	tests := []struct {
		name            string
		providerResp    ProviderEvaluateResponse
		providerErr     error
		capability      string
		conditionInfo   interface{}
		condCtx         engine.ConditionContext
		expectedMatched bool
		expectedIncLen  int
		shouldErr       bool
	}{
		{
			name:       "evaluation with incidents should return matched response",
			capability: "java",
			conditionInfo: map[string]interface{}{
				"referenced": map[string]interface{}{
					"pattern": "javax.*",
				},
			},
			providerResp: ProviderEvaluateResponse{
				Matched: true,
				Incidents: []IncidentContext{
					{
						FileURI:    uri.URI("file:///test.java"),
						LineNumber: &lineNum,
						Variables: map[string]interface{}{
							"package": "javax.servlet",
						},
					},
				},
				TemplateContext: map[string]interface{}{
					"package": "javax.servlet",
				},
			},
			condCtx: engine.ConditionContext{
				Tags: map[string]interface{}{
					"test": "tag",
				},
				Template: map[string]engine.ChainTemplate{},
				RuleID:   "test-rule-001",
			},
			expectedMatched: true,
			expectedIncLen:  1,
			shouldErr:       false,
		},
		{
			name:       "evaluation with no incidents should return no match",
			capability: "java",
			conditionInfo: map[string]interface{}{
				"referenced": map[string]interface{}{
					"pattern": "javax.*",
				},
			},
			providerResp: ProviderEvaluateResponse{
				Matched:   false,
				Incidents: []IncidentContext{},
			},
			condCtx: engine.ConditionContext{
				RuleID: "test-rule-002",
			},
			expectedMatched: false,
			expectedIncLen:  0,
			shouldErr:       false,
		},
		{
			name:       "evaluation with multiple incidents should return all",
			capability: "builtin",
			conditionInfo: map[string]interface{}{
				"filecontent": map[string]interface{}{
					"pattern": "TODO",
				},
			},
			providerResp: ProviderEvaluateResponse{
				Matched: true,
				Incidents: []IncidentContext{
					{
						FileURI:    uri.URI("file:///test1.java"),
						LineNumber: &lineNum,
					},
					{
						FileURI:    uri.URI("file:///test2.java"),
						LineNumber: &lineNum,
					},
					{
						FileURI:    uri.URI("file:///test3.java"),
						LineNumber: &lineNum,
					},
				},
			},
			condCtx: engine.ConditionContext{
				RuleID: "test-rule-003",
			},
			expectedMatched: true,
			expectedIncLen:  3,
			shouldErr:       false,
		},
		{
			name:       "evaluation with template context should preserve it",
			capability: "java",
			conditionInfo: map[string]interface{}{
				"referenced": "test",
			},
			providerResp: ProviderEvaluateResponse{
				Matched: true,
				Incidents: []IncidentContext{
					{
						FileURI: uri.URI("file:///test.java"),
					},
				},
				TemplateContext: map[string]interface{}{
					"key1": "value1",
					"key2": 42,
				},
			},
			condCtx: engine.ConditionContext{
				RuleID: "test-rule-004",
			},
			expectedMatched: true,
			expectedIncLen:  1,
			shouldErr:       false,
		},
		{
			name:       "client error should return error",
			capability: "java",
			conditionInfo: map[string]interface{}{
				"referenced": "test",
			},
			providerErr: fmt.Errorf("provider error"),
			condCtx: engine.ConditionContext{
				RuleID: "test-rule-005",
			},
			expectedMatched: false,
			expectedIncLen:  0,
			shouldErr:       true,
		},
		{
			name:       "evaluation with code location should preserve it",
			capability: "java",
			conditionInfo: map[string]interface{}{
				"referenced": "test",
			},
			providerResp: ProviderEvaluateResponse{
				Matched: true,
				Incidents: []IncidentContext{
					{
						FileURI:    uri.URI("file:///test.java"),
						LineNumber: &lineNum,
						CodeLocation: &Location{
							StartPosition: Position{Line: 10, Character: 5},
							EndPosition:   Position{Line: 10, Character: 20},
						},
					},
				},
			},
			condCtx: engine.ConditionContext{
				RuleID: "test-rule-006",
			},
			expectedMatched: true,
			expectedIncLen:  1,
			shouldErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeClient{
				evaluateResp: tt.providerResp,
				evaluateErr:  tt.providerErr,
			}

			providerCondition := ProviderCondition{
				Client:        client,
				Capability:    tt.capability,
				ConditionInfo: tt.conditionInfo,
				Rule: engine.Rule{
					RuleMeta: engine.RuleMeta{
						RuleID: tt.condCtx.RuleID,
					},
					Perform: engine.Perform{
						Message: engine.Message{
							Text: &message,
						},
					},
				},
			}

			resp, err := providerCondition.Evaluate(context.TODO(), logr.Logger{}, tt.condCtx)

			if tt.shouldErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if !client.evaluateCalled {
				t.Errorf("client.Evaluate was not called")
			}

			if resp.Matched != tt.expectedMatched {
				t.Errorf("expected Matched to be %v, got %v", tt.expectedMatched, resp.Matched)
			}

			if len(resp.Incidents) != tt.expectedIncLen {
				t.Errorf("expected %d incidents, got %d", tt.expectedIncLen, len(resp.Incidents))
			}

			// Verify template context is preserved when present
			if len(tt.providerResp.TemplateContext) > 0 {
				if !reflect.DeepEqual(resp.TemplateContext, tt.providerResp.TemplateContext) {
					t.Errorf("expected TemplateContext %v, got %v", tt.providerResp.TemplateContext, resp.TemplateContext)
				}
			}

			// Verify code location is preserved when present
			if len(resp.Incidents) > 0 && tt.providerResp.Incidents[0].CodeLocation != nil {
				if resp.Incidents[0].CodeLocation == nil {
					t.Errorf("expected code location to be preserved")
				} else {
					expectedLine := int(tt.providerResp.Incidents[0].CodeLocation.StartPosition.Line)
					if resp.Incidents[0].CodeLocation.StartPosition.Line != expectedLine {
						t.Errorf("expected start line %d, got %d", expectedLine, resp.Incidents[0].CodeLocation.StartPosition.Line)
					}
				}
			}
		})
	}
}

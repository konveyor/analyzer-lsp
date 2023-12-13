package provider

import (
	"context"
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
func (c *fakeClient) Init(context.Context, logr.Logger, InitConfig) (ServiceClient, error) {
	return nil, nil
}
func (c *fakeClient) Stop() {}

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
				Name:       tt.name,
				Upperbound: tt.upperbound,
				Lowerbound: tt.lowerbound,
				Client:     &fakeClient{dependencies: tt.dependencies},
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
				uri.URI("file1"): []*Dep{
					{Name: "dep1", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
					{Name: "dep2", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
				},
				uri.URI("file2"): []*Dep{
					{Name: "dep1", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
					{Name: "dep2", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
				},
			},
			expected: map[uri.URI][]*Dep{
				uri.URI("file1"): []*Dep{
					{Name: "dep1", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
					{Name: "dep2", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
				},
				uri.URI("file2"): []*Dep{
					{Name: "dep1", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
					{Name: "dep2", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
				},
			},
		},
		{
			title: "different versions or shas of the same dependency should not be deduped",
			dependencies: map[uri.URI][]*Dep{
				uri.URI("file1"): []*Dep{
					{Name: "dep1", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
					{Name: "dep1", Version: "v2.0.0", ResolvedIdentifier: "abcd"},
					{Name: "dep2", Version: "v1.0.0", ResolvedIdentifier: "abcde"},
					{Name: "dep2", Version: "v1.0.0", ResolvedIdentifier: "abcdf"},
				},
			},
			expected: map[uri.URI][]*Dep{
				uri.URI("file1"): []*Dep{
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
				uri.URI("file1"): []*Dep{
					{Name: "dep1", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
					{Name: "dep1", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
				},
			},
			expected: map[uri.URI][]*Dep{
				uri.URI("file1"): []*Dep{
					{Name: "dep1", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
				},
			},
		},
		{
			title: "direct dependencies should be preferred over indirect",
			dependencies: map[uri.URI][]*Dep{
				uri.URI("file1"): []*Dep{
					{Name: "dep1", Version: "v1.0.0", ResolvedIdentifier: "abcd", Indirect: true},
					{Name: "dep1", Version: "v1.0.0", ResolvedIdentifier: "abcd"},
				},
			},
			expected: map[uri.URI][]*Dep{
				uri.URI("file1"): []*Dep{
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

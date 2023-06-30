package provider

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine"
	"go.lsp.dev/uri"
)

var _ Client = &fakeClient{}

type fakeClient struct {
	dependencies []*Dep
}

func (c *fakeClient) Capabilities() []Capability { return nil }
func (c *fakeClient) HasCapability(string) bool  { return true }
func (c *fakeClient) Evaluate(string, []byte) (ProviderEvaluateResponse, error) {
	return ProviderEvaluateResponse{}, nil
}
func (c *fakeClient) Init(context.Context, logr.Logger, InitConfig) (ServiceClient, error) {
	return nil, nil
}
func (c *fakeClient) Stop() {}

func (c *fakeClient) GetDependencies() (map[uri.URI][]*Dep, error) {
	m := map[uri.URI][]*Dep{
		uri.URI("test"): c.dependencies,
	}
	return m, nil
}

func (c *fakeClient) GetDependenciesDAG() (map[uri.URI][]DepDAGItem, error) {
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

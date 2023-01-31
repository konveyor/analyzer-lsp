package parser

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/dependency/dependency"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/lib"
)

type testProvider struct {
	caps []lib.Capability
}

func (t testProvider) Capabilities() ([]lib.Capability, error) {
	return t.caps, nil
}

func (t testProvider) Init(ctx context.Context, log logr.Logger) error {
	return nil
}

func (t testProvider) GetDependencies(path string) (map[dependency.Dep][]dependency.Dep, error) {
	return nil, nil
}
func (t testProvider) Evaluate(cap string, conditionInfo []byte) (lib.ProviderEvaluateResponse, error) {
	return lib.ProviderEvaluateResponse{}, nil
}

func (t testProvider) Stop() {}

var _ provider.Client = &testProvider{}

func TestLoadRules(t *testing.T) {
	allGoFiles := "all go files"
	allGoOrJsonFiles := "all go or json files"
	allGoAndJsonFiles := "all go and json files"
	testCases := []struct {
		Name               string
		testFileName       string
		providerNameClient map[string]provider.Client
		ExpectedRules      map[string]engine.Rule
		ExpectedProvider   map[string]provider.Client
		ShouldErr          bool
		ErrorMessage       string
	}{
		{
			Name:         "basic single condition",
			testFileName: "rule-simple-default.yaml",
			providerNameClient: map[string]provider.Client{
				"builtin": testProvider{
					caps: []lib.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []lib.Capability{{
						Name: "fake",
					}},
				},
			},
			ExpectedRules: map[string]engine.Rule{
				"file-001": {
					RuleID:      "file-001",
					Description: "",
					Category:    "",
					Perform:     engine.Perform{Message: &allGoFiles},
				},
			},
			ExpectedProvider: map[string]provider.Client{
				"builtin": testProvider{
					caps: []lib.Capability{{
						Name: "file",
					}},
				},
			},
		},
		{
			Name:         "basic rule from dir",
			testFileName: "test-folder",
			providerNameClient: map[string]provider.Client{
				"builtin": testProvider{
					caps: []lib.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []lib.Capability{{
						Name: "fake",
					}},
				},
			},
			ExpectedRules: map[string]engine.Rule{
				"file-001": {
					RuleID:      "file-001",
					Description: "",
					Category:    "",
					Perform:     engine.Perform{Message: &allGoFiles},
				},
			},
			ExpectedProvider: map[string]provider.Client{
				"builtin": testProvider{
					caps: []lib.Capability{{
						Name: "file",
					}},
				},
			},
		},
		{
			Name:         "rule invalid message",
			testFileName: "invalid-message.yaml",
			providerNameClient: map[string]provider.Client{
				"builtin": testProvider{
					caps: []lib.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []lib.Capability{{
						Name: "fake",
					}},
				},
			},
			ShouldErr:    true,
			ErrorMessage: "message must be a string",
		},
		{
			Name:         "rule invalid ruleID",
			testFileName: "invalid-rule-id.yaml",
			providerNameClient: map[string]provider.Client{
				"builtin": testProvider{
					caps: []lib.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []lib.Capability{{
						Name: "fake",
					}},
				},
			},
			ShouldErr:    true,
			ErrorMessage: "unable to find ruleID in rule",
		},
		{
			Name:         "test-and-rule",
			testFileName: "rule-and.yaml",
			providerNameClient: map[string]provider.Client{
				"builtin": testProvider{
					caps: []lib.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []lib.Capability{{
						Name: "fake",
					}},
				},
			},
			ExpectedRules: map[string]engine.Rule{
				"file-001": {
					RuleID:      "file-001",
					Description: "",
					Category:    "",
					Perform:     engine.Perform{Message: &allGoAndJsonFiles},
				},
			},
			ExpectedProvider: map[string]provider.Client{
				"builtin": testProvider{
					caps: []lib.Capability{{
						Name: "file",
					}},
				},
			},
		},
		{
			Name:         "test-or-rule",
			testFileName: "rule-or.yaml",
			providerNameClient: map[string]provider.Client{
				"builtin": testProvider{
					caps: []lib.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []lib.Capability{{
						Name: "fake",
					}},
				},
			},
			ExpectedRules: map[string]engine.Rule{
				"file-001": {
					RuleID:      "file-001",
					Description: "",
					Category:    "",
					Perform:     engine.Perform{Message: &allGoOrJsonFiles},
				},
			},
			ExpectedProvider: map[string]provider.Client{
				"builtin": testProvider{
					caps: []lib.Capability{{
						Name: "file",
					}},
				},
			},
		},
		{
			Name:         "test-or-rule",
			testFileName: "rule-chain.yaml",
			providerNameClient: map[string]provider.Client{
				"builtin": testProvider{
					caps: []lib.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []lib.Capability{{
						Name: "fake",
					}},
				},
			},
			ExpectedRules: map[string]engine.Rule{
				"file-001": {
					RuleID:      "file-001",
					Description: "",
					Category:    "",
					Perform:     engine.Perform{Message: &allGoOrJsonFiles},
				},
			},
			ExpectedProvider: map[string]provider.Client{
				"builtin": testProvider{
					caps: []lib.Capability{{
						Name: "file",
					}},
				},
			},
		},
		{
			Name:         "rule no provider",
			testFileName: "rule-simple-default.yaml",
			providerNameClient: map[string]provider.Client{
				"notadded": testProvider{
					caps: []lib.Capability{{
						Name: "fake",
					}},
				},
			},
			ShouldErr:    true,
			ErrorMessage: "unable to find provider for :builtin",
		},
		{
			Name:         "rule no conditions",
			testFileName: "invalid-rule-no-conditions.yaml",
			providerNameClient: map[string]provider.Client{
				"notadded": testProvider{
					caps: []lib.Capability{{
						Name: "fake",
					}},
				},
			},
			ShouldErr:    true,
			ErrorMessage: "a Rule must have a single condition",
		},
		{
			Name:         "rule invalid conditions",
			testFileName: "invalid-rule-invalid-conditions.yaml",
			providerNameClient: map[string]provider.Client{
				"notadded": testProvider{
					caps: []lib.Capability{{
						Name: "fake",
					}},
				},
			},
			ShouldErr:    true,
			ErrorMessage: "a Rule must have a single condition",
		},
		{
			Name:         "rule not simple",
			testFileName: "rule-not-simple.yaml",
			providerNameClient: map[string]provider.Client{
				"builtin": testProvider{
					caps: []lib.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []lib.Capability{{
						Name: "fake",
					}},
				},
			},
			ExpectedRules: map[string]engine.Rule{
				"file-001": {
					RuleID:      "file-001",
					Description: "",
					Category:    "",
					Perform:     engine.Perform{Message: &allGoOrJsonFiles},
				},
			},
			ExpectedProvider: map[string]provider.Client{
				"builtin": testProvider{
					caps: []lib.Capability{{
						Name: "file",
					}},
				},
			},
		},
		{
			Name:         "rule duplicate id",
			testFileName: "invalid-dup-rule-id.yaml",
			providerNameClient: map[string]provider.Client{
				"builtin": testProvider{
					caps: []lib.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []lib.Capability{{
						Name: "fake",
					}},
				},
			},
			ShouldErr:    true,
			ErrorMessage: "duplicated rule id: file-001",
		},
		{
			Name:         "rule or/and/chain layer",
			testFileName: "or-and-chain-layer.yaml",
			providerNameClient: map[string]provider.Client{
				"builtin": testProvider{
					caps: []lib.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []lib.Capability{{
						Name: "fake",
					}},
				},
			},
			ExpectedRules: map[string]engine.Rule{
				"file-001": {
					RuleID:      "file-001",
					Description: "",
					Category:    "",
					Perform:     engine.Perform{Message: &allGoOrJsonFiles},
				},
			},
			ExpectedProvider: map[string]provider.Client{
				"builtin": testProvider{
					caps: []lib.Capability{{
						Name: "file",
					}},
				},
			},
		},
		{
			Name:         "test multiple actions, cannot set message and tag both",
			testFileName: "multiple-actions.yaml",
			providerNameClient: map[string]provider.Client{
				"builtin": testProvider{
					caps: []lib.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []lib.Capability{{
						Name: "fake",
					}},
				},
			},
			ShouldErr:    true,
			ErrorMessage: "cannot perform message and tag both",
		},
		{
			Name:         "no actions, at least one action must be set",
			testFileName: "no-actions.yaml",
			providerNameClient: map[string]provider.Client{
				"builtin": testProvider{
					caps: []lib.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []lib.Capability{{
						Name: "fake",
					}},
				},
			},
			ShouldErr:    true,
			ErrorMessage: "either message or tag must be set",
		},
		{
			Name:         "test valid tag action",
			testFileName: "valid-tag-rule.yaml",
			providerNameClient: map[string]provider.Client{
				"builtin": testProvider{
					caps: []lib.Capability{{
						Name: "file",
					}},
				},
			},
			ExpectedProvider: map[string]provider.Client{
				"builtin": testProvider{
					caps: []lib.Capability{{
						Name: "file",
					}},
				},
			},
			ExpectedRules: map[string]engine.Rule{
				"tag-001": {
					RuleID: "tag-001",
					Perform: engine.Perform{
						Tag: []string{"test"},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			ruleParser := RuleParser{
				ProviderNameToClient: tc.providerNameClient,
			}

			rules, clients, err := ruleParser.LoadRules(filepath.Join("testdata", tc.testFileName))
			if err != nil {
				if tc.ShouldErr && tc.ErrorMessage == err.Error() {
					return
				}
				t.Errorf("Got err: %v expected: should have error: %v or message: %v", err, tc.ShouldErr, tc.ErrorMessage)
			}
			if err == nil && tc.ShouldErr {
				t.Errorf("expected error but not none")
			}

			for k, c := range clients {
				gotCaps, _ := c.Capabilities()
				expectedProvider, ok := tc.ExpectedProvider[k]
				if !ok {
					t.Errorf("could not find provider: %v", k)
				}
				expectedCaps, _ := expectedProvider.Capabilities()
				if !reflect.DeepEqual(gotCaps, expectedCaps) {
					t.Errorf("expected provider and got provider caps don't match")
				}
			}

			for _, rule := range rules {
				r, ok := tc.ExpectedRules[rule.RuleID]
				if !ok {
					t.Errorf("unable to find rule with ruleID: %v", rule.RuleID)
				}

				// We will test the conditions getter by itself.
				if !reflect.DeepEqual(r.Perform, rule.Perform) || r.Category != rule.Category || r.Description != rule.Description {
					t.Errorf("rules are not equal got: %v wanted; %v", rule, r)
				}
			}
		})
	}
}

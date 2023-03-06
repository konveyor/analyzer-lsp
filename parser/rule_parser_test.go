package parser_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/bombsimon/logrusr/v3"
	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/dependency/dependency"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/hubapi"
	ruleparser "github.com/konveyor/analyzer-lsp/parser"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/lib"
	"github.com/sirupsen/logrus"
)

type testProvider struct {
	caps []lib.Capability
}

func (t testProvider) Capabilities() []lib.Capability {
	return t.caps
}
func (t testProvider) HasCapability(string) bool { return true }

func (t testProvider) Init(ctx context.Context, log logr.Logger) error {
	return nil
}

func (t testProvider) Evaluate(cap string, conditionInfo []byte) (lib.ProviderEvaluateResponse, error) {
	return lib.ProviderEvaluateResponse{}, nil
}

func (t testProvider) GetDependencies() ([]dependency.Dep, error) {
	return nil, nil
}

func (t testProvider) GetDependenciesLinkedList() (map[dependency.Dep][]dependency.Dep, error) {
	return nil, nil
}

func (t testProvider) Stop() {}

var _ provider.Client = &testProvider{}

func TestLoadRules(t *testing.T) {
	allGoFiles := "all go files"
	allGoOrJsonFiles := "all go or json files"
	allGoAndJsonFiles := "all go and json files"
	effort := 3
	testCases := []struct {
		Name               string
		testFileName       string
		providerNameClient map[string]provider.Client
		ExpectedRuleSet    map[string]engine.RuleSet
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
			ExpectedRuleSet: map[string]engine.RuleSet{
				"konveyor-analysis": {
					Rules: []engine.Rule{
						{
							RuleID: "file-001",
							Links: []hubapi.Link{
								{
									URL:   "https://go.dev",
									Title: "Golang",
								},
							},
							Labels: []string{
								"testing",
								"test",
							},
							Effort:      &effort,
							Description: "",
							Category:    &hubapi.Potential,
							Perform:     engine.Perform{Message: &allGoFiles},
						},
					},
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
			ExpectedRuleSet: map[string]engine.RuleSet{
				"file-ruleset": {
					Rules: []engine.Rule{
						{
							RuleID:      "file-001",
							Description: "",
							Perform:     engine.Perform{Message: &allGoFiles},
						},
					},
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
			ExpectedRuleSet: map[string]engine.RuleSet{
				"konveyor-analysis": {
					Rules: []engine.Rule{
						{
							RuleID:      "file-001",
							Description: "",
							Perform:     engine.Perform{Message: &allGoAndJsonFiles},
						},
					},
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
			ExpectedRuleSet: map[string]engine.RuleSet{
				"konveyor-analysis": {
					Rules: []engine.Rule{
						{
							RuleID:      "file-001",
							Description: "",
							Perform:     engine.Perform{Message: &allGoOrJsonFiles},
						},
					},
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
			ExpectedRuleSet: map[string]engine.RuleSet{
				"konveyor-analysis": {
					Rules: []engine.Rule{
						{
							RuleID:      "file-001",
							Description: "",
							Perform:     engine.Perform{Message: &allGoOrJsonFiles},
						},
					},
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
			ExpectedRuleSet: map[string]engine.RuleSet{
				"konveyor-analysis": {
					Rules: []engine.Rule{
						{
							RuleID:      "file-001",
							Description: "",
							Perform:     engine.Perform{Message: &allGoOrJsonFiles},
						},
					},
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
			ExpectedRuleSet: map[string]engine.RuleSet{
				"konveyor-analysis": {
					Rules: []engine.Rule{
						{
							RuleID:      "file-001",
							Description: "",
							Perform:     engine.Perform{Message: &allGoOrJsonFiles},
						},
					},
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
			ExpectedRuleSet: map[string]engine.RuleSet{
				"konveyor-analysis": {
					Rules: []engine.Rule{
						{
							RuleID: "tag-001",
							Perform: engine.Perform{
								Tag: []string{"test"},
							},
						},
					},
				},
			},
		},
		{
			Name:         "multiple-rulesets",
			testFileName: "folder-of-rulesets",
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
			ExpectedRuleSet: map[string]engine.RuleSet{
				"file-ruleset-a": {
					Rules: []engine.Rule{
						{
							RuleID:      "file-001",
							Description: "",
							Perform:     engine.Perform{Message: &allGoFiles},
						},
					},
				},
				"file-ruleset-b": {
					Rules: []engine.Rule{
						{
							RuleID:      "file-001",
							Description: "",
							Perform:     engine.Perform{Message: &allGoFiles},
						},
					},
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
			Name:         "failure-no-ruleset",
			testFileName: "no-ruleset",
			ShouldErr:    true,
			ErrorMessage: "unable to find ruleset.yaml",
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
		},
	}

	for _, tc := range testCases {
		logrusLog := logrus.New()
		t.Run(tc.Name, func(t *testing.T) {
			ruleParser := ruleparser.RuleParser{
				ProviderNameToClient: tc.providerNameClient,
				Log:                  logrusr.New(logrusLog),
			}

			ruleSets, clients, err := ruleParser.LoadRules(filepath.Join("testdata", tc.testFileName))
			if err != nil {
				if tc.ShouldErr && tc.ErrorMessage == err.Error() {
					return
				}
				t.Errorf("Got err: %v expected: should have error: %v or message: %v", err, tc.ShouldErr, tc.ErrorMessage)
			}
			if err == nil && tc.ShouldErr {
				t.Errorf("expected error but not none")
			}
			if len(tc.ExpectedProvider) != 0 && len(clients) == 0 {
				t.Errorf("unable to get correct clients")
			}
			if len(tc.ExpectedRuleSet) != 0 && len(ruleSets) == 0 {
				t.Errorf("unable to get correct ruleSets")
			}

			for k, c := range clients {
				gotCaps := c.Capabilities()
				expectedProvider, ok := tc.ExpectedProvider[k]
				if !ok {
					t.Errorf("could not find provider: %v", k)
				}
				expectedCaps := expectedProvider.Capabilities()
				if !reflect.DeepEqual(gotCaps, expectedCaps) {
					t.Errorf("expected provider and got provider caps don't match")
				}
			}

			for _, ruleSet := range ruleSets {
				expectedSet := tc.ExpectedRuleSet[ruleSet.Name]
				if len(ruleSet.Rules) != len(expectedSet.Rules) {
					t.Errorf("rule sets did not have matching rules")
				}
				for _, rule := range ruleSet.Rules {
					foundRule := false
					for _, expectedRule := range expectedSet.Rules {
						if reflect.DeepEqual(expectedRule.Perform, rule.Perform) && expectedRule.Description == rule.Description {
							if expectedRule.Category != nil && rule.Category != nil {
								foundRule = *expectedRule.Category == *rule.Category
							} else if expectedRule.Category != nil || rule.Category != nil {
								foundRule = false
							} else {
								foundRule = true
							}
						}
					}
					if !foundRule {
						t.Errorf("not have matching rule go: %#v, expected rules: %#v", rule, expectedSet.Rules)
					}
				}
				// We will test the conditions getter by itself.
			}
		})
	}
}

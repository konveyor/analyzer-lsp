package parser_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/bombsimon/logrusr/v3"
	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	ruleparser "github.com/konveyor/analyzer-lsp/parser"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/sirupsen/logrus"
	"go.lsp.dev/uri"
)

type testProvider struct {
	caps []provider.Capability
}

func (t testProvider) Capabilities() []provider.Capability {
	return t.caps
}

func (t testProvider) Init(ctx context.Context, log logr.Logger, config provider.InitConfig) (provider.ServiceClient, error) {
	return nil, nil
}

func (t testProvider) Evaluate(ctx context.Context, cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
	return provider.ProviderEvaluateResponse{}, nil
}

func (t testProvider) GetDependencies(ctx context.Context) (map[uri.URI][]*provider.Dep, error) {
	return nil, nil
}

func (t testProvider) GetDependenciesDAG(ctx context.Context) (map[uri.URI][]provider.DepDAGItem, error) {
	return nil, nil
}

func (t testProvider) ProviderInit(context.Context) error {
	return nil
}

func (t testProvider) Stop() {}

var _ provider.InternalProviderClient = &testProvider{}

func TestLoadRules(t *testing.T) {
	allGoFiles := "all go files"
	allGoOrJsonFiles := "all go or json files"
	allGoAndJsonFiles := "all go and json files"
	effort := 3
	testCases := []struct {
		Name               string
		testFileName       string
		providerNameClient map[string]provider.InternalProviderClient
		ExpectedRuleSet    map[string]engine.RuleSet
		ExpectedProvider   map[string]provider.InternalProviderClient
		ShouldErr          bool
		ErrorMessage       string
	}{
		{
			Name:         "test rule invalidID newline",
			testFileName: "rule-invalid-newline-ruleID.yaml",
			providerNameClient: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []provider.Capability{{
						Name: "fake",
					}},
				},
			},
			ExpectedRuleSet: map[string]engine.RuleSet{
				"konveyor-analysis": {
					Rules: []engine.Rule{},
				},
			},
			ExpectedProvider: map[string]provider.InternalProviderClient{},
		},
		{
			Name:         "test rule invalidID semi-colon",
			testFileName: "rule-invalid-semicolon-ruleID.yaml",
			providerNameClient: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []provider.Capability{{
						Name: "fake",
					}},
				},
			},
			ExpectedRuleSet: map[string]engine.RuleSet{
				"konveyor-analysis": {
					Rules: []engine.Rule{},
				},
			},
			ExpectedProvider: map[string]provider.InternalProviderClient{},
		},
		{
			Name:         "basic single condition",
			testFileName: "rule-simple-default.yaml",
			providerNameClient: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []provider.Capability{{
						Name: "fake",
					}},
				},
			},
			ExpectedRuleSet: map[string]engine.RuleSet{
				"konveyor-analysis": {
					Rules: []engine.Rule{
						{
							RuleMeta: engine.RuleMeta{
								RuleID: "file-001",
								Labels: []string{
									"testing",
									"test",
								},
								Effort:      &effort,
								Description: "",
								Category:    &konveyor.Potential,
							},
							Perform: engine.Perform{
								Message: engine.Message{
									Text: &allGoFiles,
									Links: []konveyor.Link{
										{
											URL:   "https://go.dev",
											Title: "Golang",
										},
									},
								},
							},
							When: engine.ConditionEntry{},
						},
					},
				},
			},
			ExpectedProvider: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
			},
		},
		{
			Name:         "basic rule from dir",
			testFileName: "test-folder",
			providerNameClient: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []provider.Capability{{
						Name: "fake",
					}},
				},
			},
			ExpectedRuleSet: map[string]engine.RuleSet{
				"file-ruleset": {
					Rules: []engine.Rule{
						{
							RuleMeta: engine.RuleMeta{
								RuleID:      "file-001",
								Description: "",
								Category:    &konveyor.Potential,
							},
							Perform: engine.Perform{Message: engine.Message{Text: &allGoFiles, Links: []konveyor.Link{}}},
							When:    engine.ConditionEntry{},
						},
					},
				},
			},
			ExpectedProvider: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
			},
		},
		{
			Name:         "rule invalid message",
			testFileName: "invalid-message.yaml",
			providerNameClient: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []provider.Capability{{
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
			providerNameClient: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []provider.Capability{{
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
			providerNameClient: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []provider.Capability{{
						Name: "fake",
					}},
				},
			},
			ExpectedRuleSet: map[string]engine.RuleSet{
				"konveyor-analysis": {
					Rules: []engine.Rule{
						{
							RuleMeta: engine.RuleMeta{
								RuleID:      "file-001",
								Description: "",
								Category:    &konveyor.Potential,
							},
							Perform: engine.Perform{Message: engine.Message{Text: &allGoAndJsonFiles, Links: []konveyor.Link{}}},
							When:    engine.ConditionEntry{},
						},
					},
				},
			},
			ExpectedProvider: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
			},
		},
		{
			Name:         "test-or-rule",
			testFileName: "rule-or.yaml",
			providerNameClient: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []provider.Capability{{
						Name: "fake",
					}},
				},
			},
			ExpectedRuleSet: map[string]engine.RuleSet{
				"konveyor-analysis": {
					Rules: []engine.Rule{
						{
							RuleMeta: engine.RuleMeta{
								RuleID:      "file-001",
								Description: "",
								Category:    &konveyor.Potential,
							},
							Perform: engine.Perform{Message: engine.Message{Text: &allGoOrJsonFiles, Links: []konveyor.Link{}}},
							When:    engine.ConditionEntry{},
						},
					},
				},
			},
			ExpectedProvider: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
			},
		},
		{
			Name:         "test-or-rule",
			testFileName: "rule-chain.yaml",
			providerNameClient: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []provider.Capability{{
						Name: "fake",
					}},
				},
			},
			ExpectedRuleSet: map[string]engine.RuleSet{
				"konveyor-analysis": {
					Rules: []engine.Rule{
						{
							RuleMeta: engine.RuleMeta{
								RuleID:      "file-001",
								Description: "",
								Category:    &konveyor.Potential,
							},
							Perform: engine.Perform{Message: engine.Message{Text: &allGoOrJsonFiles, Links: []konveyor.Link{}}},
							When:    engine.ConditionEntry{},
						},
					},
				},
			},
			ExpectedProvider: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
			},
		},
		{
			Name:         "rule no provider",
			testFileName: "rule-simple-default.yaml",
			providerNameClient: map[string]provider.InternalProviderClient{
				"notadded": testProvider{
					caps: []provider.Capability{{
						Name: "fake",
					}},
				},
			},
			ShouldErr:    true,
			ErrorMessage: "unable to find provider for: builtin",
		},
		{
			Name:         "rule no conditions",
			testFileName: "invalid-rule-no-conditions.yaml",
			providerNameClient: map[string]provider.InternalProviderClient{
				"notadded": testProvider{
					caps: []provider.Capability{{
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
			providerNameClient: map[string]provider.InternalProviderClient{
				"notadded": testProvider{
					caps: []provider.Capability{{
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
			providerNameClient: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []provider.Capability{{
						Name: "fake",
					}},
				},
			},
			ExpectedRuleSet: map[string]engine.RuleSet{
				"konveyor-analysis": {
					Rules: []engine.Rule{
						{
							RuleMeta: engine.RuleMeta{
								RuleID:      "file-001",
								Description: "",
								Category:    &konveyor.Potential,
							},
							Perform: engine.Perform{Message: engine.Message{Text: &allGoOrJsonFiles, Links: []konveyor.Link{}}},
							When:    engine.ConditionEntry{},
						},
					},
				},
			},
			ExpectedProvider: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
			},
		},
		{
			Name:         "rule duplicate id",
			testFileName: "invalid-dup-rule-id.yaml",
			providerNameClient: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []provider.Capability{{
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
			providerNameClient: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []provider.Capability{{
						Name: "fake",
					}},
				},
			},
			ExpectedRuleSet: map[string]engine.RuleSet{
				"konveyor-analysis": {
					Rules: []engine.Rule{
						{
							RuleMeta: engine.RuleMeta{
								RuleID:      "file-001",
								Category:    &konveyor.Potential,
								Description: "",
							},
							Perform: engine.Perform{Message: engine.Message{Text: &allGoOrJsonFiles, Links: []konveyor.Link{}}},
							When:    engine.ConditionEntry{},
						},
					},
				},
			},
			ExpectedProvider: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
			},
		},
		{
			Name:         "test multiple actions, can set message and tag both",
			testFileName: "multiple-actions.yaml",
			providerNameClient: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
			},
			ExpectedProvider: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
			},
			ExpectedRuleSet: map[string]engine.RuleSet{
				"konveyor-analysis": {
					Rules: []engine.Rule{
						{
							RuleMeta: engine.RuleMeta{
								RuleID:      "file-001",
								Category:    &konveyor.Potential,
								Description: "",
							},
							Perform: engine.Perform{
								Message: engine.Message{
									Text:  &allGoFiles,
									Links: []konveyor.Link{},
								},
								Tag: []string{"test"},
							},
							When: engine.ConditionEntry{},
						},
					},
				},
			},
			ShouldErr: false,
		},
		{
			Name:         "no actions, at least one action must be set",
			testFileName: "no-actions.yaml",
			providerNameClient: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []provider.Capability{{
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
			providerNameClient: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
			},
			ExpectedProvider: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
			},
			ExpectedRuleSet: map[string]engine.RuleSet{
				"konveyor-analysis": {
					Rules: []engine.Rule{
						{
							RuleMeta: engine.RuleMeta{
								RuleID: "tag-001",
							},
							Perform: engine.Perform{
								Tag: []string{"test"},
							},
							When: engine.ConditionEntry{},
						},
					},
				},
			},
		},
		{
			Name:         "multiple-rulesets",
			testFileName: "folder-of-rulesets",
			providerNameClient: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []provider.Capability{{
						Name: "fake",
					}},
				},
			},
			ExpectedRuleSet: map[string]engine.RuleSet{
				"file-ruleset-a": {
					Rules: []engine.Rule{
						{
							RuleMeta: engine.RuleMeta{
								RuleID:      "file-001",
								Description: "",
								Category:    &konveyor.Potential,
							},
							Perform: engine.Perform{Message: engine.Message{Text: &allGoFiles, Links: []konveyor.Link{}}},
							When:    engine.ConditionEntry{},
						},
					},
				},
				"file-ruleset-b": {
					Rules: []engine.Rule{
						{
							RuleMeta: engine.RuleMeta{RuleID: "file-001",
								Description: "",
								Category:    &konveyor.Potential,
							},
							Perform: engine.Perform{Message: engine.Message{Text: &allGoFiles, Links: []konveyor.Link{}}},
							When:    engine.ConditionEntry{},
						},
					},
				},
			},
			ExpectedProvider: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
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
			providerNameClient: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []provider.Capability{{
						Name: "fake",
					}},
				},
			},
		},
		{
			Name:         "handle not-valid category",
			testFileName: "invalid-category.yaml",
			providerNameClient: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
				"notadded": testProvider{
					caps: []provider.Capability{{
						Name: "fake",
					}},
				},
			},
			ExpectedRuleSet: map[string]engine.RuleSet{
				"konveyor-analysis": {
					Rules: []engine.Rule{
						{
							RuleMeta: engine.RuleMeta{
								RuleID: "file-001",
								Labels: []string{
									"testing",
									"test",
								},
								Effort:      &effort,
								Description: "",
								Category:    &konveyor.Potential,
							},
							Perform: engine.Perform{
								Message: engine.Message{
									Text: &allGoFiles,
									Links: []konveyor.Link{
										{
											URL:   "https://go.dev",
											Title: "Golang",
										},
									},
								},
							},
							When: engine.ConditionEntry{},
						},
					},
				},
			},
			ExpectedProvider: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "file",
					}},
				},
			},
		},
		{
			Name:         "chaining should yield the same number of conditions as the rule",
			testFileName: "rule-chain-2.yaml",
			providerNameClient: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "filecontent",
					}},
				},
			},
			ExpectedProvider: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "filecontent",
					}},
				},
			},
			ExpectedRuleSet: map[string]engine.RuleSet{
				"konveyor-analysis": {
					Rules: []engine.Rule{
						{
							RuleMeta: engine.RuleMeta{
								RuleID:      "chaining-rule",
								Description: "",
								Category:    &konveyor.Potential,
							},
							Perform: engine.Perform{
								Message: engine.Message{
									Text:  &allGoFiles,
									Links: []konveyor.Link{},
								},
							},
							When: engine.AndCondition{
								Conditions: []engine.ConditionEntry{
									{
										From:      "",
										As:        "file",
										Ignorable: false,
										Not:       false,
									},
									{
										From:      "file",
										As:        "",
										Ignorable: false,
										Not:       false,
									},
								},
							},
						},
					},
				},
			},
		},
		{
			Name:         "no two conditions should have the same 'as' field within the same block",
			testFileName: "rule-chain-same-as.yaml",
			ShouldErr:    true,
			ErrorMessage: "condition cannot have multiple 'as' fields with the same name",
			providerNameClient: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "filecontent",
					}},
				},
			},
			ExpectedProvider: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "filecontent",
					}},
				},
			},
			ExpectedRuleSet: map[string]engine.RuleSet{
				"konveyor-analysis": {
					Rules: []engine.Rule{
						{
							RuleMeta: engine.RuleMeta{
								RuleID:      "chaining-rule",
								Description: "",
								Category:    &konveyor.Potential,
							},
							Perform: engine.Perform{
								Message: engine.Message{
									Text:  &allGoFiles,
									Links: []konveyor.Link{},
								},
							},
						},
					},
				},
			},
		},
		{
			Name:         "a condition should not have the same 'as' and 'from' fields",
			testFileName: "rule-chain-same-as-from.yaml",
			ShouldErr:    true,
			ErrorMessage: "condition cannot have the same value for fields 'from' and 'as'",
			providerNameClient: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "filecontent",
					}},
				},
			},
			ExpectedProvider: map[string]provider.InternalProviderClient{
				"builtin": testProvider{
					caps: []provider.Capability{{
						Name: "filecontent",
					}},
				},
			},
			ExpectedRuleSet: map[string]engine.RuleSet{
				"konveyor-analysis": {
					Rules: []engine.Rule{
						{
							RuleMeta: engine.RuleMeta{
								RuleID:      "chaining-rule",
								Description: "",
								Category:    &konveyor.Potential,
							},
							Perform: engine.Perform{
								Message: engine.Message{
									Text:  &allGoFiles,
									Links: []konveyor.Link{},
								},
							},
						},
					},
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
				return
			}
			if err == nil && tc.ShouldErr {
				t.Errorf("expected error but not none")
				return
			}
			if len(tc.ExpectedProvider) != 0 && len(clients) == 0 {
				t.Errorf("unable to get correct clients")
				return
			}
			if len(tc.ExpectedRuleSet) != 0 && len(ruleSets) == 0 {
				t.Errorf("unable to get correct ruleSets")
				return
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
						compareWhens(expectedRule.When, rule.When, t)
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

func compareWhens(w1 engine.Conditional, w2 engine.Conditional, t *testing.T) {
	if (w1 == nil && w2 != nil) || (w1 != nil && w2 == nil) {
		t.Errorf("rulesets did not have matching when field")
	}
	if and1, ok := w1.(engine.AndCondition); ok {
		and2, ok := w2.(engine.AndCondition)
		if !ok {
			t.Errorf("rulesets did not have matching when field")
		}
		compareConditions(and1.Conditions, and2.Conditions, t)
	} else if or1, ok := w1.(engine.OrCondition); ok {
		or2, ok := w2.(engine.OrCondition)
		if !ok {
			t.Errorf("rulesets did not have matching when field")
		}
		compareConditions(or1.Conditions, or2.Conditions, t)
	}

}

func compareConditions(cs1 []engine.ConditionEntry, cs2 []engine.ConditionEntry, t *testing.T) {
	if len(cs1) != len(cs2) {
		t.Errorf("rulesets did not have the same number of conditions")
	}
	for i := 0; i < len(cs1); i++ {
		c1 := cs1[i]
		c2 := cs2[i]
		if c1.As != c2.As {
			t.Errorf("rulesets did not have the same As field")
		}
		if c1.From != c2.From {
			t.Errorf("rulesets did not have the same From field")
		}
		if c1.Ignorable != c2.Ignorable {
			t.Errorf("rulesets did not have the same Ignorable field")
		}
		if c1.Not != c2.Not {
			t.Errorf("rulesets did not have the same Not field")
		}
	}
}

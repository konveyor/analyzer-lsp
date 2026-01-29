package parser_test

import (
	"context"
	"io"
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

func (t testProvider) Prepare(ctx context.Context, conditionsByCap []provider.ConditionsByCap) error {
	return nil
}

func (t testProvider) Capabilities() []provider.Capability {
	return t.caps
}

func (t testProvider) Init(ctx context.Context, log logr.Logger, config provider.InitConfig) (provider.ServiceClient, provider.InitConfig, error) {
	return nil, provider.InitConfig{}, nil
}

func (t testProvider) Evaluate(ctx context.Context, cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
	return provider.ProviderEvaluateResponse{}, nil
}

func (t testProvider) NotifyFileChanges(ctx context.Context, changes ...provider.FileChange) error {
	return nil
}

func (t testProvider) GetDependencies(ctx context.Context) (map[uri.URI][]*provider.Dep, error) {
	return nil, nil
}

func (t testProvider) GetDependenciesDAG(ctx context.Context) (map[uri.URI][]provider.DepDAGItem, error) {
	return nil, nil
}

func (t testProvider) ProviderInit(context.Context, []provider.InitConfig) ([]provider.InitConfig, error) {
	return nil, nil
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
			// With the fix, missing providers are gracefully skipped, not errors
			// The rule will be skipped since provider is unavailable
			ShouldErr:    false,
			ErrorMessage: "",
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

			ruleSets, clients, _, err := ruleParser.LoadRules(filepath.Join("testdata", tc.testFileName))
			if err != nil {
				if tc.ShouldErr && tc.ErrorMessage == err.Error() {
					return
				}
				t.Errorf("Got err: %v expected: should have error: %v or message: %v", err, tc.ShouldErr, tc.ErrorMessage)
				return
			}
			if tc.ShouldErr {
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

// TestCreateSchemaAndErrors tests OpenAPI schema generation and error types
func TestCreateSchemaAndErrors(t *testing.T) {
	// Test CreateSchema
	schema, err := ruleparser.CreateSchema()
	if err != nil || schema.MapOfSchemaOrRefValues == nil {
		t.Fatal("CreateSchema() failed")
	}
	if _, ok := schema.MapOfSchemaOrRefValues["rule"]; !ok {
		t.Error("Schema missing 'rule' definition")
	}
	if _, ok := schema.MapOfSchemaOrRefValues["rulesets"]; !ok {
		t.Error("Schema missing 'rulesets' definition")
	}

	// Test MissingProviderError
	err = ruleparser.MissingProviderError{Provider: "test-provider"}
	if err.Error() != "unable to find provider for: test-provider" {
		t.Errorf("MissingProviderError message incorrect: %v", err.Error())
	}

	// Test parserErrors.Error() - need to trigger it via LoadRules with error directory
	ruleParser := ruleparser.RuleParser{
		ProviderNameToClient: map[string]provider.InternalProviderClient{
			"builtin": testProvider{caps: []provider.Capability{{Name: "file"}}},
		},
		Log: logrusr.New(logrus.New()),
	}
	_, _, _, err = ruleParser.LoadRules(filepath.Join("testdata", "error-dir"))
	if err != nil && err.Error() != "" {
		// parserErrors.Error() was called
		t.Logf("parserErrors.Error() called: %v", err)
	}
}

// TestAdvancedRuleFeatures tests custom variables, dependencies, and complex conditions
func TestAdvancedRuleFeatures(t *testing.T) {
	logrusLog := logrus.New()

	testCases := []struct {
		name             string
		file             string
		providerCaps     map[string][]string
		noDependencyRule bool
		expectError      bool
		validateFunc     func(*testing.T, []engine.RuleSet)
	}{
		{
			name:         "advanced features",
			file:         "rule-advanced-features.yaml",
			providerCaps: map[string][]string{"builtin": {"file"}},
			validateFunc: func(t *testing.T, ruleSets []engine.RuleSet) {
				foundCustomVar, foundExtraFields, foundFlags := false, false, false
				for _, rs := range ruleSets {
					for _, r := range rs.Rules {
						if r.RuleID == "custom-var-001" && len(r.CustomVariables) > 0 {
							cv := r.CustomVariables[0]
							if cv.Name == "testVar" && cv.DefaultValue == "defaultVal" {
								foundCustomVar = true
							}
						}
						if r.RuleID == "extra-fields-001" {
							if r.Effort != nil && *r.Effort == 5 && len(r.Labels) == 2 {
								foundExtraFields = true
							}
						}
						if r.RuleID == "condition-flags-001" {
							foundFlags = true
						}
					}
				}
				if !foundCustomVar || !foundExtraFields || !foundFlags {
					t.Error("Not all advanced features found")
				}
			},
		},
		{
			name:         "dependency conditions",
			file:         "rule-dependency-tests.yaml",
			providerCaps: map[string][]string{"java": {"dependency"}},
			expectError:  true, // Has both valid and invalid rules
		},
		{
			name:             "dependency rules skipped",
			file:             "rule-dependency-tests.yaml",
			providerCaps:     map[string][]string{"java": {"dependency"}},
			noDependencyRule: true,
		},
		{
			name:         "capability mismatch",
			file:         "rule-dependency-tests.yaml",
			providerCaps: map[string][]string{"java": {"wrongcap"}},
			expectError:  true,
		},
		{
			name:         "invalid custom variables",
			file:         "rule-error-cases.yaml",
			providerCaps: map[string][]string{"builtin": {"file"}},
			// Errors are logged but LoadRules doesn't fail
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			providers := make(map[string]provider.InternalProviderClient)
			for name, caps := range tc.providerCaps {
				capList := make([]provider.Capability, len(caps))
				for i, c := range caps {
					capList[i] = provider.Capability{Name: c}
				}
				providers[name] = testProvider{caps: capList}
			}

			ruleParser := ruleparser.RuleParser{
				ProviderNameToClient: providers,
				Log:                  logrusr.New(logrusLog),
				NoDependencyRules:    tc.noDependencyRule,
			}

			ruleSets, _, _, err := ruleParser.LoadRules(filepath.Join("testdata", tc.file))
			if tc.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if tc.validateFunc != nil {
				tc.validateFunc(t, ruleSets)
			}
		})
	}
}

// TestRuleLoadingEdgeCases tests various edge cases in rule loading
func TestRuleLoadingEdgeCases(t *testing.T) {
	logrusLog := logrus.New()

	testCases := []struct {
		name         string
		path         string
		useLoadRule  bool
		expectError  bool
		validateFunc func(*testing.T, []engine.RuleSet)
	}{
		{
			name: "directory loading",
			path: "test-folder",
			validateFunc: func(t *testing.T, rs []engine.RuleSet) {
				if len(rs) == 0 {
					t.Error("Expected rules from directory")
				}
			},
		},
		{
			name:        "multiple errors in directory",
			path:        "error-dir",
			expectError: true,
		},
		{
			name:        "non-existent file",
			path:        "does-not-exist.yaml",
			useLoadRule: true,
			expectError: true,
		},
		{
			name:        "invalid yaml",
			path:        "invalid.yaml",
			useLoadRule: true,
		},
		{
			name: "ruleset with metadata",
			path: "ruleset-test",
			validateFunc: func(t *testing.T, rs []engine.RuleSet) {
				for _, ruleSet := range rs {
					if ruleSet.Name == "test-ruleset" {
						if ruleSet.Description != "Test ruleset with metadata" {
							t.Error("Ruleset description incorrect")
						}
						if len(ruleSet.Labels) == 0 || len(ruleSet.Tags) == 0 {
							t.Error("Expected labels and tags on ruleset")
						}
						return
					}
				}
				t.Error("test-ruleset not found")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ruleParser := ruleparser.RuleParser{
				ProviderNameToClient: map[string]provider.InternalProviderClient{
					"builtin": testProvider{caps: []provider.Capability{{Name: "file"}}},
				},
				Log: logrusr.New(logrusLog),
			}

			var ruleSets []engine.RuleSet
			var err error

			if tc.useLoadRule {
				var rules []engine.Rule
				rules, _, _, err = ruleParser.LoadRule(filepath.Join("testdata", tc.path))
				if err == nil && len(rules) > 0 {
					ruleSets = []engine.RuleSet{{Rules: rules}}
				}
			} else {
				ruleSets, _, _, err = ruleParser.LoadRules(filepath.Join("testdata", tc.path))
			}

			if tc.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if tc.validateFunc != nil && err == nil {
				tc.validateFunc(t, ruleSets)
			}
		})
	}
}

func TestAndConditionWithFilecontent(t *testing.T) {
	// Test that verifies parsing of a rule with AND condition containing
	// multiple builtin.filecontent patterns
	logrusLog := logrus.New()
	logrusLog.SetOutput(io.Discard)
	logrusLog.SetLevel(logrus.PanicLevel)

	ruleParser := ruleparser.RuleParser{
		ProviderNameToClient: map[string]provider.InternalProviderClient{
			"builtin": testProvider{
				caps: []provider.Capability{
					{Name: "filecontent"},
				},
			},
		},
		Log: logrusr.New(logrusLog),
	}

	rules, _, _, err := ruleParser.LoadRule(filepath.Join("testdata", "rule-and-filecontent.yaml"))
	if err != nil {
		t.Fatalf("Failed to load rule: %v", err)
	}

	if len(rules) != 1 {
		t.Fatalf("Expected 1 rule, got %d", len(rules))
	}

	rule := rules[0]

	andCond, ok := rule.When.(engine.AndCondition)
	if !ok {
		t.Fatalf("Expected rule to have AndCondition, got %T", rule.When)
	}

	if len(andCond.Conditions) != 2 {
		t.Fatalf("Expected 2 conditions in AND block, got %d", len(andCond.Conditions))
	}

	for i, cond := range andCond.Conditions {
		if cond.ProviderSpecificConfig == nil {
			t.Errorf("Condition %d has nil ProviderSpecificConfig", i)
		}
	}

	if rule.RuleID != "test-and-filecontent-001" {
		t.Errorf("Expected ruleID 'test-and-filecontent-001', got '%s'", rule.RuleID)
	}

	if rule.Description != "Test AND condition with multiple filecontent patterns" {
		t.Errorf("Unexpected description: %s", rule.Description)
	}

	if rule.Perform.Message.Text == nil {
		t.Fatal("Expected message text to be set")
	}
	if *rule.Perform.Message.Text != "This should work with AND conditions" {
		t.Errorf("Expected message 'This should work with AND conditions', got '%s'", *rule.Perform.Message.Text)
	}

	t.Logf("Successfully parsed rule with AND condition containing %d builtin.filecontent patterns", len(andCond.Conditions))
}

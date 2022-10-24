package engine

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/bombsimon/logrusr/v3"
	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
)

type testConditional struct {
	err   error
	ret   bool
	sleep bool
}

func (t testConditional) Evaluate(log logr.Logger, ctx map[string]interface{}) (ConditionResponse, error) {
	if t.sleep {
		time.Sleep(5 * time.Second)
	}
	return ConditionResponse{Passed: t.ret}, t.err
}

func createTestConditional(b bool, e error, sleep bool) Conditional {
	return &testConditional{
		err:   e,
		ret:   b,
		sleep: sleep,
	}
}

type testChainableConditionalAs struct {
	err           error
	ret           bool
	documentedKey string
	AsValue       interface{}
}

func (t testChainableConditionalAs) Evaluate(log logr.Logger, ctx map[string]interface{}) (ConditionResponse, error) {
	return ConditionResponse{
		Passed:              false,
		ConditionHitContext: []map[string]string{},
		TemplateContext: map[string]interface{}{
			t.documentedKey: t.AsValue,
		},
	}, t.err
}

type testChainableConditionalFrom struct {
	FromName      string
	DocumentedKey string
	FromValue     interface{}
}

func (t testChainableConditionalFrom) Evaluate(log logr.Logger, ctx map[string]interface{}) (ConditionResponse, error) {

	if v, ok := ctx[t.FromName]; ok {
		if m, ok := v.(map[string]interface{}); ok {
			if reflect.DeepEqual(m[t.DocumentedKey], t.FromValue) {
				return ConditionResponse{
					Passed:              false,
					ConditionHitContext: []map[string]string{},
					TemplateContext:     map[string]interface{}{},
				}, nil
			}
		}
	}
	return ConditionResponse{}, fmt.Errorf("unable to find from in context")
}

func TestEvaluateAndConditions(t *testing.T) {

	testCases := []struct {
		Name       string
		Conditions []ConditionEntry
		IsError    bool
		IsPassed   bool
	}{
		{
			Name: "Base Case",
			Conditions: []ConditionEntry{
				{
					ProviderSpecificConfig: createTestConditional(true, nil, false),
				},
			},
			IsPassed: true,
		},
		{
			Name: "And two inner conditions",
			Conditions: []ConditionEntry{
				{
					ProviderSpecificConfig: createTestConditional(true, nil, false),
				},
				{
					ProviderSpecificConfig: createTestConditional(true, nil, false),
				},
			},
			IsPassed: true,
		},
		{
			Name: "And two inner conditions failure",
			Conditions: []ConditionEntry{
				{
					ProviderSpecificConfig: createTestConditional(true, nil, false),
				},
				{
					ProviderSpecificConfig: createTestConditional(false, nil, false),
				},
			},
		},
		{
			Name: "And two conditions with nested conditions",
			Conditions: []ConditionEntry{
				{
					ProviderSpecificConfig: AndCondition{Conditions: []ConditionEntry{
						{
							From:                   "",
							As:                     "",
							ProviderSpecificConfig: createTestConditional(true, nil, false),
						},
						{
							From:                   "",
							As:                     "",
							ProviderSpecificConfig: createTestConditional(true, nil, false),
						},
					}},
				},
				{
					ProviderSpecificConfig: createTestConditional(true, nil, false),
				},
			},
			IsPassed: true,
		},
		{
			Name: "And two conditions with nested conditions failure",
			Conditions: []ConditionEntry{
				{
					ProviderSpecificConfig: AndCondition{
						Conditions: []ConditionEntry{
							{
								ProviderSpecificConfig: createTestConditional(false, nil, false),
							},
							{
								ProviderSpecificConfig: createTestConditional(true, nil, false),
							},
						},
					},
				},
				{
					ProviderSpecificConfig: createTestConditional(true, nil, false),
				},
			},
		},
	}

	logrusLog := logrus.New()
	log := logrusr.New(logrusLog)
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			rule := Rule{
				Perform: "testing",
				When:    AndCondition{Conditions: tc.Conditions},
			}
			ret, err := processRule(rule, log)
			if err != nil && !tc.IsError {
				t.Errorf("got err: %v, expected no error", err)
			}
			if ret.Passed != tc.IsPassed {
				t.Errorf("Expected to be: %v, but got: %v", tc.IsPassed, ret)
			}
		})
	}

}

func TestEvaluateOrConditions(t *testing.T) {

	testCases := []struct {
		Name       string
		Conditions []ConditionEntry
		IsError    bool
		IsPassed   bool
	}{
		{
			Name: "Base Case",
			Conditions: []ConditionEntry{
				{
					ProviderSpecificConfig: createTestConditional(true, nil, false),
				},
			},
			IsPassed: true,
		},
		{
			Name: "or two inner conditions",
			Conditions: []ConditionEntry{
				{
					ProviderSpecificConfig: createTestConditional(false, nil, false),
				},
				{
					ProviderSpecificConfig: createTestConditional(true, nil, false),
				},
			},
			IsPassed: true,
		},
		{
			Name: "or two inner conditions true first",
			Conditions: []ConditionEntry{
				{
					ProviderSpecificConfig: createTestConditional(true, nil, false),
				},
				{
					ProviderSpecificConfig: createTestConditional(false, nil, false),
				},
			},
			IsPassed: true,
		},
		{
			Name: "or two inner conditions failure",
			Conditions: []ConditionEntry{
				{
					ProviderSpecificConfig: createTestConditional(false, nil, false),
				},
				{
					ProviderSpecificConfig: createTestConditional(false, nil, false),
				},
			},
		},
		{
			Name: "And two conditions with nested conditions",
			Conditions: []ConditionEntry{
				{
					ProviderSpecificConfig: OrCondition{Conditions: []ConditionEntry{
						{
							ProviderSpecificConfig: createTestConditional(true, nil, false),
						},
						{
							ProviderSpecificConfig: createTestConditional(false, nil, false),
						},
					}},
				},
				{
					ProviderSpecificConfig: createTestConditional(false, nil, false),
				},
			},
			IsPassed: true,
		},
		{
			Name: "or two conditions with nested conditions failure",
			Conditions: []ConditionEntry{
				{
					ProviderSpecificConfig: OrCondition{
						Conditions: []ConditionEntry{
							{
								ProviderSpecificConfig: createTestConditional(false, nil, false),
							},
							{

								ProviderSpecificConfig: createTestConditional(false, nil, false),
							},
						},
					},
				},
				{
					ProviderSpecificConfig: createTestConditional(false, nil, false),
				},
			},
		},
	}

	logrusLog := logrus.New()
	log := logrusr.New(logrusLog)
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			rule := Rule{
				Perform: "testing",
				When:    OrCondition{tc.Conditions},
			}
			ret, err := processRule(rule, log)
			if err != nil && !tc.IsError {
				t.Errorf("got err: %v, expected no error", err)
			}
			if ret.Passed != tc.IsPassed {
				t.Errorf("Expected to be: %v, but got: %v", tc.IsPassed, ret)
			}
		})
	}
}

func TestChainConditions(t *testing.T) {

	testCases := []struct {
		Name       string
		Conditions []ConditionEntry
		IsError    bool
		IsPassed   bool
	}{
		{
			Name: "Test Basic single chain",
			Conditions: []ConditionEntry{
				{
					As: "testing",
					ProviderSpecificConfig: testChainableConditionalAs{
						documentedKey: "filepaths",
						AsValue:       []string{"test.yaml"},
					},
				},
				{
					From: "testing",
					ProviderSpecificConfig: testChainableConditionalFrom{
						FromName:      "testing",
						DocumentedKey: "filepaths",
						FromValue:     []string{"test.yaml"},
					},
				},
			},
		},
		{
			Name: "Test or chain As provided by one element in or block",
			Conditions: []ConditionEntry{
				{
					As: "testing",
					ProviderSpecificConfig: OrCondition{
						Conditions: []ConditionEntry{
							{
								ProviderSpecificConfig: testChainableConditionalAs{
									documentedKey: "filepaths",
									AsValue:       []string{"test.yaml"},
								},
							},
							{
								ProviderSpecificConfig: createTestConditional(false, nil, false),
							},
						},
					},
				},
				{
					From: "testing",
					ProviderSpecificConfig: testChainableConditionalFrom{
						FromName:      "testing",
						DocumentedKey: "filepaths",
						FromValue:     []string{"test.yaml"},
					},
				},
			},
		},
		{
			Name: "Test and chain As provided by one element in and block",
			Conditions: []ConditionEntry{
				{
					As: "testing",
					ProviderSpecificConfig: AndCondition{
						Conditions: []ConditionEntry{
							{
								ProviderSpecificConfig: testChainableConditionalAs{
									documentedKey: "filepaths",
									AsValue:       []string{"test.yaml"},
								},
							},
							{
								ProviderSpecificConfig: createTestConditional(false, nil, false),
							},
						},
					},
				},
				{
					From: "testing",
					ProviderSpecificConfig: testChainableConditionalFrom{
						FromName:      "testing",
						DocumentedKey: "filepaths",
						FromValue:     []string{"test.yaml"},
					},
				},
			},
		},
	}

	logrusLog := logrus.New()
	log := logrusr.New(logrusLog)
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			rule := Rule{
				Perform: "testing",
				When:    ChainCondition{tc.Conditions},
			}
			ret, err := processRule(rule, log)
			if err != nil && !tc.IsError {
				t.Errorf("got err: %v, expected no error", err)
			}
			if ret.Passed != tc.IsPassed {
				t.Errorf("Expected to be: %v, but got: %v", tc.IsPassed, ret)
			}
		})
	}
}

func TestRuleEngine(t *testing.T) {
	testCases := []struct {
		Name  string
		Rules []Rule
	}{
		{
			Name: "Test Running",
			Rules: []Rule{
				{
					Perform: "WOO",
					When:    createTestConditional(true, nil, true),
				},
				{
					Perform: "WOO - False",
					When: AndCondition{
						Conditions: []ConditionEntry{
							{
								ProviderSpecificConfig: createTestConditional(true, nil, true),
							},
							{
								ProviderSpecificConfig: createTestConditional(false, nil, true),
							},
						},
					},
				},
				{
					Perform: "WOO - False",
					When: AndCondition{
						Conditions: []ConditionEntry{
							{
								ProviderSpecificConfig: createTestConditional(true, nil, true),
							},
							{
								ProviderSpecificConfig: createTestConditional(false, nil, true),
							},
						},
					},
				},
			},
		},
	}

	logrusLog := logrus.New()
	log := logrusr.New(logrusLog)
	ruleEngine := CreateRuleEngine(context.Background(), 10, log)

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			start := time.Now()
			ruleEngine.RunRules(context.Background(), tc.Rules)
			// make sure that the the test takes only as long as we would expect w/ the sleeps
			if time.Since(start) >= 11*time.Second {
				t.Fail()
			}

		})
	}
}

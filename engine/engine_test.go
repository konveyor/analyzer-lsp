package engine

import (
	"context"
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

func (t testConditional) Evaluate(log logr.Logger) (ConditionResponse, error) {
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

func TestEvaluateAndConditions(t *testing.T) {

	testCases := []struct {
		Name       string
		Conditions []Conditional
		IsError    bool
		IsPassed   bool
	}{
		{
			Name: "Base Case",
			Conditions: []Conditional{
				createTestConditional(true, nil, false),
			},
			IsPassed: true,
		},
		{
			Name: "And two inner conditions",
			Conditions: []Conditional{
				createTestConditional(true, nil, false),
				createTestConditional(true, nil, false),
			},
			IsPassed: true,
		},
		{
			Name: "And two inner conditions failure",
			Conditions: []Conditional{
				createTestConditional(true, nil, false),
				createTestConditional(false, nil, false),
			},
		},
		{
			Name: "And two conditions with nested conditions",
			Conditions: []Conditional{
				AndCondition{Conditions: []Conditional{
					createTestConditional(true, nil, false),
					createTestConditional(true, nil, false),
				}},
				createTestConditional(true, nil, false),
			},
			IsPassed: true,
		},
		{
			Name: "And two conditions with nested conditions failure",
			Conditions: []Conditional{
				AndCondition{
					Conditions: []Conditional{
						createTestConditional(false, nil, false),
						createTestConditional(true, nil, false),
					},
				},
				createTestConditional(true, nil, false),
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
		Conditions []Conditional
		IsError    bool
		IsPassed   bool
	}{
		{
			Name: "Base Case",
			Conditions: []Conditional{
				createTestConditional(true, nil, false),
			},
			IsPassed: true,
		},
		{
			Name: "or two inner conditions",
			Conditions: []Conditional{
				createTestConditional(false, nil, false),
				createTestConditional(true, nil, false),
			},
			IsPassed: true,
		},
		{
			Name: "or two inner conditions true first",
			Conditions: []Conditional{
				createTestConditional(true, nil, false),
				createTestConditional(false, nil, false),
			},
			IsPassed: true,
		},
		{
			Name: "or two inner conditions failure",
			Conditions: []Conditional{
				createTestConditional(false, nil, false),
				createTestConditional(false, nil, false),
			},
		},
		{
			Name: "And two conditions with nested conditions",
			Conditions: []Conditional{
				OrCondition{Conditions: []Conditional{
					createTestConditional(true, nil, false),
					createTestConditional(false, nil, false),
				}},
				createTestConditional(false, nil, false),
			},
			IsPassed: true,
		},
		{
			Name: "or two conditions with nested conditions failure",
			Conditions: []Conditional{
				OrCondition{
					Conditions: []Conditional{
						createTestConditional(false, nil, false),
						createTestConditional(false, nil, false),
					},
				},
				createTestConditional(false, nil, false),
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
						Conditions: []Conditional{
							createTestConditional(true, nil, true),
							createTestConditional(false, nil, true),
						},
					},
				},
				{
					Perform: "WOO - False",
					When: AndCondition{
						Conditions: []Conditional{
							createTestConditional(true, nil, true),
							createTestConditional(false, nil, true),
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

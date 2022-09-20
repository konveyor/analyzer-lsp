package engine

import (
	"context"
	"testing"
	"time"
)

type testInerCondition struct {
	err error
	ret bool
}

func (t testInerCondition) Evaluate() (bool, error) {
	time.Sleep(5 * time.Second)
	return t.ret, t.err
}

func createTestInnerCondition(b bool, e error) InnerCondition {
	return &testInerCondition{
		err: e,
		ret: b,
	}
}

func TestEvaluateAndCondtions(t *testing.T) {

	testCases := []struct {
		Name       string
		Conditions []Condition
		IsError    bool
		IsPassed   bool
	}{
		{
			Name: "Base Case",
			Conditions: []Condition{
				{
					InnerCondition: createTestInnerCondition(true, nil),
				},
			},
			IsPassed: true,
		},
		{
			Name: "And two inner conditions",
			Conditions: []Condition{
				{
					InnerCondition: createTestInnerCondition(true, nil),
				},
				{
					InnerCondition: createTestInnerCondition(true, nil),
				},
			},
			IsPassed: true,
		},
		{
			Name: "And two inner conditions failure",
			Conditions: []Condition{
				{
					InnerCondition: createTestInnerCondition(true, nil),
				},
				{
					InnerCondition: createTestInnerCondition(false, nil),
				},
			},
		},
		{
			Name: "And two conditions with nested conditions",
			Conditions: []Condition{
				{
					When: &Conditional{
						And: []Condition{
							{
								InnerCondition: createTestInnerCondition(true, nil),
							},
							{
								InnerCondition: createTestInnerCondition(true, nil),
							},
						},
					},
				},
				{
					InnerCondition: createTestInnerCondition(true, nil),
				},
			},
			IsPassed: true,
		},
		{
			Name: "And two conditions with nested conditions failure",
			Conditions: []Condition{
				{
					When: &Conditional{
						And: []Condition{
							{
								InnerCondition: createTestInnerCondition(false, nil),
							},
							{
								InnerCondition: createTestInnerCondition(true, nil),
							},
						},
					},
				},
				{
					InnerCondition: createTestInnerCondition(true, nil),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			ret, err := evaluateAndCondtions(tc.Conditions)
			if err != nil && !tc.IsError {
				t.Errorf("got err: %v, expected no error", err)
			}
			if ret != tc.IsPassed {
				t.Errorf("Expected to be: %v, but got: %v", tc.IsPassed, ret)
			}
		})
	}

}

func TestEvaluateOrCondtions(t *testing.T) {

	testCases := []struct {
		Name       string
		Conditions []Condition
		IsError    bool
		IsPassed   bool
	}{
		{
			Name: "Base Case",
			Conditions: []Condition{
				{
					InnerCondition: createTestInnerCondition(true, nil),
				},
			},
			IsPassed: true,
		},
		{
			Name: "or two inner conditions",
			Conditions: []Condition{
				{
					InnerCondition: createTestInnerCondition(false, nil),
				},
				{
					InnerCondition: createTestInnerCondition(true, nil),
				},
			},
			IsPassed: true,
		},
		{
			Name: "or two inner conditions true first",
			Conditions: []Condition{
				{
					InnerCondition: createTestInnerCondition(true, nil),
				},
				{
					InnerCondition: createTestInnerCondition(false, nil),
				},
			},
			IsPassed: true,
		},
		{
			Name: "or two inner conditions failure",
			Conditions: []Condition{
				{
					InnerCondition: createTestInnerCondition(false, nil),
				},
				{
					InnerCondition: createTestInnerCondition(false, nil),
				},
			},
		},
		{
			Name: "And two conditions with nested conditions",
			Conditions: []Condition{
				{
					When: &Conditional{
						Or: []Condition{
							{
								InnerCondition: createTestInnerCondition(true, nil),
							},
							{
								InnerCondition: createTestInnerCondition(false, nil),
							},
						},
					},
				},
				{
					InnerCondition: createTestInnerCondition(false, nil),
				},
			},
			IsPassed: true,
		},
		{
			Name: "or two conditions with nested conditions failure",
			Conditions: []Condition{
				{
					When: &Conditional{
						And: []Condition{
							{
								InnerCondition: createTestInnerCondition(false, nil),
							},
							{
								InnerCondition: createTestInnerCondition(false, nil),
							},
						},
					},
				},
				{
					InnerCondition: createTestInnerCondition(false, nil),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			ret, err := evaluateOrConditions(tc.Conditions)
			if err != nil && !tc.IsError {
				t.Errorf("got err: %v, expected no error", err)
			}
			if ret != tc.IsPassed {
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
					When: Conditional{
						InnerCondition: createTestInnerCondition(true, nil),
					},
				},
				{
					Perform: "WOO - False",
					When: Conditional{
						And: []Condition{
							{
								InnerCondition: createTestInnerCondition(true, nil),
							},
							{
								InnerCondition: createTestInnerCondition(false, nil),
							},
						},
					},
				},
				{
					Perform: "WOO - False",
					When: Conditional{
						And: []Condition{
							{
								InnerCondition: createTestInnerCondition(true, nil),
							},
							{
								InnerCondition: createTestInnerCondition(false, nil),
							},
						},
					},
				},
			},
		},
	}

	ruleEngine := CreateRuleEngine(context.Background(), 10)

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			ruleEngine.RunRules(context.Background(), tc.Rules)

			t.Fail()
		})
	}
}

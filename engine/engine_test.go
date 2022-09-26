package engine

import (
	"context"
	"testing"
	"time"
)

type testInerCondition struct {
	err   error
	ret   bool
	sleep bool
}

func (t testInerCondition) Evaluate() (InnerConndtionResponse, error) {
	if t.sleep {
		time.Sleep(5 * time.Second)
	}
	return InnerConndtionResponse{Passed: t.ret}, t.err
}

func createTestInnerCondition(b bool, e error, sleep bool) InnerCondition {
	return &testInerCondition{
		err:   e,
		ret:   b,
		sleep: sleep,
	}
}

func TestEvaluateAndCondtions(t *testing.T) {

	testCases := []struct {
		Name       string
		Conditions []Condition
		IsError    bool
		IsPassed   bool
	}{
		// {
		// 	Name: "Base Case",
		// 	Conditions: []Condition{
		// 		{
		// 			InnerCondition: createTestInnerCondition(true, nil, false),
		// 		},
		// 	},
		// 	IsPassed: true,
		// },
		// {
		// 	Name: "And two inner conditions",
		// 	Conditions: []Condition{
		// 		{
		// 			InnerCondition: createTestInnerCondition(true, nil, false),
		// 		},
		// 		{
		// 			InnerCondition: createTestInnerCondition(true, nil, false),
		// 		},
		// 	},
		// 	IsPassed: true,
		// },
		{
			Name: "And two inner conditions failure",
			Conditions: []Condition{
				{
					InnerCondition: createTestInnerCondition(true, nil, false),
				},
				{
					InnerCondition: createTestInnerCondition(false, nil, false),
				},
			},
		},
		// {
		// 	Name: "And two conditions with nested conditions",
		// 	Conditions: []Condition{
		// 		{
		// 			When: &Conditional{
		// 				And: []Condition{
		// 					{
		// 						InnerCondition: createTestInnerCondition(true, nil, false),
		// 					},
		// 					{
		// 						InnerCondition: createTestInnerCondition(true, nil, false),
		// 					},
		// 				},
		// 			},
		// 		},
		// 		{
		// 			InnerCondition: createTestInnerCondition(true, nil, false),
		// 		},
		// 	},
		// 	IsPassed: true,
		// },
		// {
		// 	Name: "And two conditions with nested conditions failure",
		// 	Conditions: []Condition{
		// 		{
		// 			When: &Conditional{
		// 				And: []Condition{
		// 					{
		// 						InnerCondition: createTestInnerCondition(false, nil, false),
		// 					},
		// 					{
		// 						InnerCondition: createTestInnerCondition(true, nil, false),
		// 					},
		// 				},
		// 			},
		// 		},
		// 		{
		// 			InnerCondition: createTestInnerCondition(true, nil, false),
		// 		},
		// 	},
		// },
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			ret, err := evaluateAndCondtions(tc.Conditions)
			if err != nil && !tc.IsError {
				t.Errorf("got err: %v, expected no error", err)
			}
			if ret.Passed != tc.IsPassed {
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
					InnerCondition: createTestInnerCondition(true, nil, false),
				},
			},
			IsPassed: true,
		},
		{
			Name: "or two inner conditions",
			Conditions: []Condition{
				{
					InnerCondition: createTestInnerCondition(false, nil, false),
				},
				{
					InnerCondition: createTestInnerCondition(true, nil, false),
				},
			},
			IsPassed: true,
		},
		{
			Name: "or two inner conditions true first",
			Conditions: []Condition{
				{
					InnerCondition: createTestInnerCondition(true, nil, false),
				},
				{
					InnerCondition: createTestInnerCondition(false, nil, false),
				},
			},
			IsPassed: true,
		},
		{
			Name: "or two inner conditions failure",
			Conditions: []Condition{
				{
					InnerCondition: createTestInnerCondition(false, nil, false),
				},
				{
					InnerCondition: createTestInnerCondition(false, nil, false),
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
								InnerCondition: createTestInnerCondition(true, nil, false),
							},
							{
								InnerCondition: createTestInnerCondition(false, nil, false),
							},
						},
					},
				},
				{
					InnerCondition: createTestInnerCondition(false, nil, false),
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
								InnerCondition: createTestInnerCondition(false, nil, false),
							},
							{
								InnerCondition: createTestInnerCondition(false, nil, false),
							},
						},
					},
				},
				{
					InnerCondition: createTestInnerCondition(false, nil, false),
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
					When: Conditional{
						InnerCondition: createTestInnerCondition(true, nil, true),
					},
				},
				{
					Perform: "WOO - False",
					When: Conditional{
						And: []Condition{
							{
								InnerCondition: createTestInnerCondition(true, nil, true),
							},
							{
								InnerCondition: createTestInnerCondition(false, nil, true),
							},
						},
					},
				},
				{
					Perform: "WOO - False",
					When: Conditional{
						And: []Condition{
							{
								InnerCondition: createTestInnerCondition(true, nil, true),
							},
							{
								InnerCondition: createTestInnerCondition(false, nil, true),
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
			start := time.Now()
			ruleEngine.RunRules(context.Background(), tc.Rules)
			// make sure that the the test takes only as long as we would expect w/ the sleeps
			if time.Since(start) >= 11*time.Second {
				t.Fail()
			}

		})
	}
}

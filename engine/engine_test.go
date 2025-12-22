package engine

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/bombsimon/logrusr/v3"
	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/progress"
	"github.com/sirupsen/logrus"
	"go.lsp.dev/uri"
)

type testConditional struct {
	err   error
	ret   bool
	sleep bool
}

func (t testConditional) Evaluate(ctx context.Context, log logr.Logger, condCtx ConditionContext) (ConditionResponse, error) {
	if t.sleep {
		time.Sleep(5 * time.Second)
	}
	if t.ret {
		return ConditionResponse{Matched: t.ret, Incidents: []IncidentContext{{FileURI: "test"}}}, t.err
	} else {
		return ConditionResponse{Matched: t.ret}, t.err
	}
}

func (t testConditional) Ignorable() bool {
	return true
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
	documentedKey string
	AsValue       any
}

func (t testChainableConditionalAs) Evaluate(ctx context.Context, log logr.Logger, condCtx ConditionContext) (ConditionResponse, error) {
	return ConditionResponse{
		Matched: true,
		TemplateContext: map[string]any{
			t.documentedKey: t.AsValue,
		},
		Incidents: []IncidentContext{{}},
	}, t.err
}

func (t testChainableConditionalAs) Ignorable() bool {
	return true
}

type testChainableConditionalFrom struct {
	FromName      string
	DocumentedKey string
	FromValue     any
}

func (t testChainableConditionalFrom) Ignorable() bool {
	return true
}

func (t testChainableConditionalFrom) Evaluate(ctx context.Context, log logr.Logger, condCtx ConditionContext) (ConditionResponse, error) {
	if v, ok := condCtx.Template[t.FromName]; ok {
		if reflect.DeepEqual(v.Extras[t.DocumentedKey], t.FromValue) {
			return ConditionResponse{
				Matched:         true,
				TemplateContext: map[string]any{},
			}, nil
		}
	}
	return ConditionResponse{}, fmt.Errorf("unable to find from in context")
}

type testTemplateConditional struct{}

func (t testTemplateConditional) Evaluate(ctx context.Context, log logr.Logger, condCtx ConditionContext) (ConditionResponse, error) {
	lineNum := 10
	return ConditionResponse{
		Matched: true,
		Incidents: []IncidentContext{
			{
				FileURI:    "file:///test.java",
				LineNumber: &lineNum,
				Variables: map[string]any{
					"name": "Spring",
				},
			},
		},
	}, nil
}

func (t testTemplateConditional) Ignorable() bool {
	return true
}

func TestEvaluateAndConditions(t *testing.T) {

	testCases := []struct {
		Name       string
		Conditions []ConditionEntry
		IsError    bool
		IsMatched  bool
	}{
		{
			Name: "Base Case",
			Conditions: []ConditionEntry{
				{
					ProviderSpecificConfig: createTestConditional(false, nil, false),
				},
			},
			IsMatched: false,
		},
		{
			Name: "And two inner conditions",
			Conditions: []ConditionEntry{
				{
					ProviderSpecificConfig: createTestConditional(false, nil, false),
				},
				{
					ProviderSpecificConfig: createTestConditional(false, nil, false),
				},
			},
			IsMatched: false,
		},
		{
			Name: "And two inner conditions failure",
			Conditions: []ConditionEntry{
				{
					ProviderSpecificConfig: createTestConditional(false, nil, false),
				},
				{
					ProviderSpecificConfig: createTestConditional(true, nil, false),
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
							ProviderSpecificConfig: createTestConditional(false, nil, false),
						},
						{
							From:                   "",
							As:                     "",
							ProviderSpecificConfig: createTestConditional(false, nil, false),
						},
					}},
				},
				{
					ProviderSpecificConfig: createTestConditional(false, nil, false),
				},
			},
			IsMatched: false,
		},
		{
			Name: "And two conditions with nested conditions failure",
			Conditions: []ConditionEntry{
				{
					ProviderSpecificConfig: AndCondition{
						Conditions: []ConditionEntry{
							{
								ProviderSpecificConfig: createTestConditional(true, nil, false),
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
	testString := "testing"
	logrusLog := logrus.New()
	log := logrusr.New(logrusLog)
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			rule := Rule{
				Perform: Perform{
					Message: Message{
						Text: &testString,
					},
				},
				When: AndCondition{Conditions: tc.Conditions},
			}

			ret, err := processRule(context.TODO(), rule, ConditionContext{
				Template: make(map[string]ChainTemplate),
			}, log)
			if err != nil && !tc.IsError {
				t.Errorf("got err: %v, expected no error", err)
			}
			if ret.Matched != tc.IsMatched {
				t.Errorf("Expected to be: %v, but got: %v", tc.IsMatched, ret)
			}
		})
	}

}

func TestEvaluateOrConditions(t *testing.T) {

	testCases := []struct {
		Name       string
		Conditions []ConditionEntry
		IsError    bool
		IsMatched  bool
	}{
		{
			Name: "Base Case",
			Conditions: []ConditionEntry{
				{
					ProviderSpecificConfig: createTestConditional(false, nil, false),
				},
			},
			IsMatched: false,
		},
		{
			Name: "or two inner conditions",
			Conditions: []ConditionEntry{
				{
					ProviderSpecificConfig: createTestConditional(true, nil, false),
				},
				{
					ProviderSpecificConfig: createTestConditional(false, nil, false),
				},
			},
			IsMatched: true,
		},
		{
			Name: "or two inner conditions false first",
			Conditions: []ConditionEntry{
				{
					ProviderSpecificConfig: createTestConditional(false, nil, false),
				},
				{
					ProviderSpecificConfig: createTestConditional(true, nil, false),
				},
			},
			IsMatched: true,
		},
		{
			Name: "or two inner conditions failure",
			Conditions: []ConditionEntry{
				{
					ProviderSpecificConfig: createTestConditional(true, nil, false),
				},
				{
					ProviderSpecificConfig: createTestConditional(true, nil, false),
				},
			},
			IsMatched: true,
		},
		{
			Name: "And two conditions with nested conditions",
			Conditions: []ConditionEntry{
				{
					ProviderSpecificConfig: OrCondition{Conditions: []ConditionEntry{
						{
							ProviderSpecificConfig: createTestConditional(false, nil, false),
						},
						{
							ProviderSpecificConfig: createTestConditional(true, nil, false),
						},
					}},
				},
				{
					ProviderSpecificConfig: createTestConditional(true, nil, false),
				},
			},
			IsMatched: true,
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
	testString := "testing"
	logrusLog := logrus.New()
	log := logrusr.New(logrusLog)
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			rule := Rule{
				Perform: Perform{
					Message: Message{
						Text: &testString,
					},
				},
				When: OrCondition{tc.Conditions},
			}
			ret, err := processRule(context.TODO(), rule, ConditionContext{
				Template: make(map[string]ChainTemplate),
			}, log)
			if err != nil && !tc.IsError {
				t.Errorf("got err: %v, expected no error", err)
			}
			if ret.Matched != tc.IsMatched {
				t.Errorf("Expected to be: %v, but got: %v", tc.IsMatched, ret)
			}
		})
	}
}

func TestChainConditions(t *testing.T) {

	testCases := []struct {
		Name       string
		Conditions []ConditionEntry
		IsError    bool
		IsMatched  bool
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
			IsMatched: true,
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
								ProviderSpecificConfig: createTestConditional(true, nil, false),
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
			IsMatched: true,
		},
		{
			Name: "Test and chain As provided and block",
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
								ProviderSpecificConfig: createTestConditional(true, nil, false),
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
			IsMatched: true,
		},
		{
			Name: "Test and chain As provided by one element in as block",
			Conditions: []ConditionEntry{
				{
					ProviderSpecificConfig: AndCondition{
						Conditions: []ConditionEntry{
							{
								As: "testing",
								ProviderSpecificConfig: OrCondition{
									Conditions: []ConditionEntry{
										{
											As: "testing",
											ProviderSpecificConfig: testChainableConditionalAs{
												documentedKey: "filepaths",
												AsValue:       []string{"test.yaml"},
											},
										},
									},
								},
							},
							{
								ProviderSpecificConfig: createTestConditional(true, nil, false),
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
			IsMatched: true,
		},
	}
	testString := "testing"
	logrusLog := logrus.New()
	log := logrusr.New(logrusLog)
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			rule := Rule{
				Perform: Perform{
					Message: Message{
						Text: &testString,
					},
				},
				When: OrCondition{tc.Conditions},
			}
			ret, err := processRule(context.TODO(), rule, ConditionContext{
				Template: make(map[string]ChainTemplate),
			}, log)
			if err != nil && !tc.IsError {
				t.Errorf("got err: %v, expected no error", err)
			}
			if ret.Matched != tc.IsMatched {
				t.Errorf("Expected to be: %v, but got: %v", tc.IsMatched, ret)
			}
		})
	}
}

type testCodeSnipper struct{}

func (t *testCodeSnipper) GetCodeSnip(uri.URI, Location) (string, error) {
	time.Sleep(10 * time.Second)
	return "test code snip", nil
}

func TestRuleEngine(t *testing.T) {
	woo := "WOO"
	wooFalse := "WOO - False"
	testCases := []struct {
		Name  string
		Rules []RuleSet
	}{
		{
			Name: "Test Running",
			Rules: []RuleSet{
				{
					Rules: []Rule{
						{
							Perform: Perform{Message: Message{Text: &woo}},
							When:    createTestConditional(false, nil, true),
						},
						{
							Perform: Perform{Message: Message{Text: &wooFalse}},
							When: AndCondition{
								Conditions: []ConditionEntry{
									{
										ProviderSpecificConfig: createTestConditional(false, nil, true),
									},
									{
										ProviderSpecificConfig: createTestConditional(true, nil, true),
									},
								},
							},
						},
						{
							Perform: Perform{Message: Message{Text: &wooFalse}},
							When: AndCondition{
								Conditions: []ConditionEntry{
									{
										ProviderSpecificConfig: createTestConditional(false, nil, true),
									},
									{
										ProviderSpecificConfig: createTestConditional(true, nil, true),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			Name: "test 11 rules",
			Rules: []RuleSet{
				{
					Rules: []Rule{
						{
							Perform: Perform{Message: Message{Text: &woo}},
							When:    createTestConditional(false, nil, false),
						},
						{
							Perform: Perform{Message: Message{Text: &wooFalse}},
							When: AndCondition{
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
							Perform: Perform{Message: Message{Text: &wooFalse}},
							When: AndCondition{
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
							Perform: Perform{Message: Message{Text: &wooFalse}},
							When: AndCondition{
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
							Perform: Perform{Message: Message{Text: &wooFalse}},
							When: AndCondition{
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
							Perform: Perform{Message: Message{Text: &wooFalse}},
							When: AndCondition{
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
							Perform: Perform{Message: Message{Text: &wooFalse}},
							When: AndCondition{
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
							Perform: Perform{Message: Message{Text: &wooFalse}},
							When: AndCondition{
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
							Perform: Perform{Message: Message{Text: &wooFalse}},
							When: AndCondition{
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
							Perform: Perform{Message: Message{Text: &wooFalse}},
							When: AndCondition{
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
							Perform: Perform{Message: Message{Text: &wooFalse}},
							When: AndCondition{
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
					},
				},
			},
		},
		{
			// Before the change this would take 10 seconds per rule so a 100 seconds
			// Now should pass 10 seconds
			Name: "test 10 rules with code snip",
			Rules: []RuleSet{
				{
					Rules: []Rule{
						{
							Perform: Perform{Message: Message{Text: &woo}},
							When:    createTestConditional(true, nil, false),
							Snipper: &testCodeSnipper{},
						},
						{
							Perform: Perform{Message: Message{Text: &wooFalse}},
							Snipper: &testCodeSnipper{},
							When:    createTestConditional(true, nil, false),
						},
						{
							Perform: Perform{Message: Message{Text: &wooFalse}},
							Snipper: &testCodeSnipper{},
							When:    createTestConditional(true, nil, false),
						},
						{
							Perform: Perform{Message: Message{Text: &wooFalse}},
							Snipper: &testCodeSnipper{},
							When:    createTestConditional(true, nil, false),
						},
						{
							Perform: Perform{Message: Message{Text: &wooFalse}},
							Snipper: &testCodeSnipper{},
							When:    createTestConditional(true, nil, false),
						},
						{
							Perform: Perform{Message: Message{Text: &wooFalse}},
							Snipper: &testCodeSnipper{},
							When:    createTestConditional(true, nil, false),
						},
						{
							Perform: Perform{Message: Message{Text: &wooFalse}},
							Snipper: &testCodeSnipper{},
							When:    createTestConditional(true, nil, false),
						},
						{
							Perform: Perform{Message: Message{Text: &wooFalse}},
							Snipper: &testCodeSnipper{},
							When:    createTestConditional(true, nil, false),
						},
						{
							Perform: Perform{Message: Message{Text: &wooFalse}},
							Snipper: &testCodeSnipper{},
							When:    createTestConditional(true, nil, false),
						},
						{
							Perform: Perform{Message: Message{Text: &wooFalse}},
							Snipper: &testCodeSnipper{},
							When:    createTestConditional(true, nil, false),
						},
						{
							Perform: Perform{Message: Message{Text: &wooFalse}},
							Snipper: &testCodeSnipper{},
							When:    createTestConditional(true, nil, false),
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

func Test_parseTagsFromPerformString(t *testing.T) {
	tests := []struct {
		name      string
		tagString string
		want      []string
		wantErr   bool
	}{
		{
			name:      "tc1",
			tagString: "test1,test2,test3,test4",
			want:      []string{"test1", "test2", "test3", "test4"},
		},
		{
			name:      "tc2",
			tagString: "test1-tag,",
			want:      []string{"test1-tag"},
		},
		{
			name:      "tc3",
			tagString: "test1",
			want:      []string{"test1"},
		},
		{
			name:      "tc4",
			tagString: "Category=test1,test2,test3,test4",
			want:      []string{"test1", "test2", "test3", "test4"},
		},
		{
			name:      "tc5",
			tagString: "Category=test1,",
			want:      []string{"test1"},
		},
		{
			name:      "tc6",
			tagString: "test1, test2, test3, test4",
			want:      []string{"test1", "test2", "test3", "test4"},
		},
		{
			name:      "tc6",
			tagString: "test tag1, test tag2",
			want:      []string{"test tag1", "test tag2"},
		},
		{
			name:      "tc7",
			tagString: "Category==test1,test2,test3,test4",
			want:      nil,
			wantErr:   true,
		},
		{
			name:      "tc8 - spaces in the tag values",
			tagString: "Category 1=test 1,test 2,test 3,test 4",
			want:      []string{"test 1", "test 2", "test 3", "test 4"},
			wantErr:   false,
		},
		{
			name:      "tc9 - parentheses in the tag values",
			tagString: "Category (1)=test (1),test (2),test (3),",
			want:      []string{"test (1)", "test (2)", "test (3)"},
			wantErr:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTagsFromPerformString(tt.tagString)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseTagsFromPerformString() = %v, want %v", got, tt.want)
			}
			if tt.wantErr != (err != nil) {
				t.Errorf("parseTagsFromPerformString() = error %v, want %v", err != nil, tt.wantErr)
			}
		})
	}
}

func TestRunRulesWithProgressReporter(t *testing.T) {
	logrus.SetLevel(logrus.ErrorLevel)
	logrusLog := logrus.New()
	logrusLog.SetOutput(io.Discard)
	logrusLog.SetLevel(logrus.PanicLevel)
	log := logrusr.New(logrusLog)

	ctx := context.Background()
	ruleEngine := CreateRuleEngine(ctx, 10, log)
	defer ruleEngine.Stop()

	// Create a channel reporter to capture progress events
	reporter := progress.NewChannelReporter(ctx)
	defer reporter.Close()

	// Collect events in background
	events := []progress.ProgressEvent{}
	eventsDone := make(chan struct{})
	go func() {
		for event := range reporter.Events() {
			events = append(events, event)
		}
		close(eventsDone)
	}()

	// Create simple test rules
	msg := "test"
	rules := []RuleSet{
		{
			Name: "test-ruleset",
			Rules: []Rule{
				{
					Perform: Perform{Message: Message{Text: &msg}},
					When:    createTestConditional(true, nil, false),
				},
				{
					Perform: Perform{Message: Message{Text: &msg}},
					When:    createTestConditional(true, nil, false),
				},
			},
		},
	}

	// Run rules with progress reporter
	ruleEngine.RunRulesWithOptions(ctx, rules, []RunOption{
		WithProgressReporter(reporter),
	})

	reporter.Close()
	<-eventsDone

	// Verify we got progress events
	if len(events) == 0 {
		t.Fatal("Expected progress events, got none")
	}

	// Check for start event
	foundStart := false
	for _, event := range events {
		if event.Stage == progress.StageRuleExecution && event.Current == 0 {
			foundStart = true
			break
		}
	}
	if !foundStart {
		t.Error("Expected start event with Current=0")
	}

	// Check for completion event
	foundComplete := false
	for _, event := range events {
		if event.Stage == progress.StageComplete {
			foundComplete = true
			if event.Percent != 100.0 {
				t.Errorf("Expected completion event with Percent=100.0, got %f", event.Percent)
			}
			break
		}
	}
	if !foundComplete {
		t.Error("Expected completion event")
	}
}

func TestRunRulesWithoutProgressReporter(t *testing.T) {
	logrus.SetLevel(logrus.ErrorLevel)
	logrusLog := logrus.New()
	logrusLog.SetOutput(io.Discard)
	logrusLog.SetLevel(logrus.PanicLevel)
	log := logrusr.New(logrusLog)

	ctx := context.Background()
	ruleEngine := CreateRuleEngine(ctx, 10, log)
	defer ruleEngine.Stop()

	// Create simple test rules
	msg := "test"
	rules := []RuleSet{
		{
			Name: "test-ruleset",
			Rules: []Rule{
				{
					Perform: Perform{Message: Message{Text: &msg}},
					When:    createTestConditional(true, nil, false),
				},
			},
		},
	}

	// Run rules without progress reporter (should not panic)
	results := ruleEngine.RunRules(ctx, rules)

	if len(results) == 0 {
		t.Error("Expected results from RunRules")
	}
}

func TestConcurrentRunsWithSeparateProgressReporters(t *testing.T) {
	logrus.SetLevel(logrus.ErrorLevel)
	logrusLog := logrus.New()
	logrusLog.SetOutput(io.Discard)
	logrusLog.SetLevel(logrus.PanicLevel)
	log := logrusr.New(logrusLog)

	ctx := context.Background()
	ruleEngine := CreateRuleEngine(ctx, 10, log)
	defer ruleEngine.Stop()

	// Create two separate reporters
	ctx1, cancel1 := context.WithCancel(ctx)
	defer cancel1()
	reporter1 := progress.NewChannelReporter(ctx1)

	ctx2, cancel2 := context.WithCancel(ctx)
	defer cancel2()
	reporter2 := progress.NewChannelReporter(ctx2)

	// Collect events separately
	events1 := []progress.ProgressEvent{}
	events2 := []progress.ProgressEvent{}

	done1 := make(chan struct{})
	done2 := make(chan struct{})

	go func() {
		for event := range reporter1.Events() {
			events1 = append(events1, event)
		}
		close(done1)
	}()

	go func() {
		for event := range reporter2.Events() {
			events2 = append(events2, event)
		}
		close(done2)
	}()

	// Create different test rules for each run
	msg1 := "rule-1"
	msg2 := "rule-2"
	rules1 := []RuleSet{
		{
			Name: "ruleset-1",
			Rules: []Rule{
				{
					Perform: Perform{Message: Message{Text: &msg1}},
					When:    createTestConditional(true, nil, false),
				},
			},
		},
	}

	rules2 := []RuleSet{
		{
			Name: "ruleset-2",
			Rules: []Rule{
				{
					Perform: Perform{Message: Message{Text: &msg2}},
					When:    createTestConditional(true, nil, false),
				},
			},
		},
	}

	// Run both concurrently with separate reporters
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		ruleEngine.RunRulesWithOptions(ctx1, rules1, []RunOption{
			WithProgressReporter(reporter1),
		})
		reporter1.Close()
	}()

	go func() {
		defer wg.Done()
		ruleEngine.RunRulesWithOptions(ctx2, rules2, []RunOption{
			WithProgressReporter(reporter2),
		})
		reporter2.Close()
	}()

	wg.Wait()
	<-done1
	<-done2

	// Verify both got events
	if len(events1) == 0 {
		t.Error("Expected events for reporter1")
	}
	if len(events2) == 0 {
		t.Error("Expected events for reporter2")
	}

	// Both should have received completion events since they ran successfully
	hasComplete1 := false
	for _, event := range events1 {
		if event.Stage == progress.StageComplete {
			hasComplete1 = true
			break
		}
	}
	if !hasComplete1 {
		t.Error("Expected reporter1 to receive completion event")
	}

	hasComplete2 := false
	for _, event := range events2 {
		if event.Stage == progress.StageComplete {
			hasComplete2 = true
			break
		}
	}
	if !hasComplete2 {
		t.Error("Expected reporter2 to receive completion event")
	}
}

func TestRunTaggingRules(t *testing.T) {
	logrus.SetLevel(logrus.ErrorLevel)
	logrusLog := logrus.New()
	logrusLog.SetOutput(io.Discard)
	logrusLog.SetLevel(logrus.PanicLevel)
	log := logrusr.New(logrusLog)

	ctx := context.Background()
	ruleEngine := CreateRuleEngine(ctx, 10, log).(*ruleEngine)
	defer ruleEngine.Stop()

	effort5 := 5

	tests := []struct {
		name      string
		infoRules []ruleMessage
		checkFunc func(t *testing.T, resultContext ConditionContext, rs *konveyor.RuleSet)
	}{
		{
			name: "basic tag creation",
			infoRules: []ruleMessage{
				{
					rule: Rule{
						RuleMeta: RuleMeta{
							RuleID: "tag-rule-1",
						},
						Perform: Perform{
							Tag: []string{"Java", "Migration"},
						},
						When: createTestConditional(true, nil, false),
					},
					ruleSetName: "test-ruleset",
				},
			},
			checkFunc: func(t *testing.T, resultContext ConditionContext, rs *konveyor.RuleSet) {
				if !resultContext.Tags["Java"].(bool) {
					t.Error("Expected 'Java' tag in context")
				}
				if !resultContext.Tags["Migration"].(bool) {
					t.Error("Expected 'Migration' tag in context")
				}
				if len(rs.Tags) != 2 {
					t.Errorf("Expected 2 tags in ruleset, got %d", len(rs.Tags))
				}
				if len(rs.Insights) != 1 {
					t.Errorf("Expected 1 insight, got %d", len(rs.Insights))
				}
			},
		},
		{
			name: "tag with effort creates violation",
			infoRules: []ruleMessage{
				{
					rule: Rule{
						RuleMeta: RuleMeta{
							RuleID: "tag-rule-with-effort",
							Effort: &effort5,
						},
						Perform: Perform{
							Tag: []string{"HighEffort"},
						},
						When: createTestConditional(true, nil, false),
					},
					ruleSetName: "test-ruleset",
				},
			},
			checkFunc: func(t *testing.T, resultContext ConditionContext, rs *konveyor.RuleSet) {
				if len(rs.Violations) != 1 {
					t.Errorf("Expected 1 violation for tag with effort, got %d", len(rs.Violations))
				}
				if len(rs.Insights) != 0 {
					t.Errorf("Expected 0 insights for tag with effort, got %d", len(rs.Insights))
				}
			},
		},
		{
			name: "template tags with variables",
			infoRules: []ruleMessage{
				{
					rule: Rule{
						RuleMeta: RuleMeta{
							RuleID: "template-tag-rule",
						},
						Perform: Perform{
							Tag: []string{"Framework={{name}}"},
						},
						When: &testTemplateConditional{},
					},
					ruleSetName: "test-ruleset",
				},
			},
			checkFunc: func(t *testing.T, resultContext ConditionContext, rs *konveyor.RuleSet) {
				if val, ok := resultContext.Tags["Spring"]; !ok || !val.(bool) {
					t.Errorf("Expected 'Spring' tag in context, got tags: %v", resultContext.Tags)
				}
			},
		},
		{
			name: "unmatched tagging rule",
			infoRules: []ruleMessage{
				{
					rule: Rule{
						RuleMeta: RuleMeta{
							RuleID: "unmatched-tag-rule",
						},
						Perform: Perform{
							Tag: []string{"ShouldNotExist"},
						},
						When: createTestConditional(false, nil, false),
					},
					ruleSetName: "test-ruleset",
				},
			},
			checkFunc: func(t *testing.T, resultContext ConditionContext, rs *konveyor.RuleSet) {
				if len(rs.Unmatched) != 1 {
					t.Errorf("Expected 1 unmatched rule, got %d", len(rs.Unmatched))
				}
				if rs.Unmatched[0] != "unmatched-tag-rule" {
					t.Errorf("Expected unmatched rule 'unmatched-tag-rule', got %s", rs.Unmatched[0])
				}
			},
		},
		{
			name: "error in tagging rule",
			infoRules: []ruleMessage{
				{
					rule: Rule{
						RuleMeta: RuleMeta{
							RuleID: "error-tag-rule",
						},
						Perform: Perform{
							Tag: []string{"ErrorTag"},
						},
						When: createTestConditional(true, fmt.Errorf("test error"), false),
					},
					ruleSetName: "test-ruleset",
				},
			},
			checkFunc: func(t *testing.T, resultContext ConditionContext, rs *konveyor.RuleSet) {
				if len(rs.Errors) != 1 {
					t.Errorf("Expected 1 error, got %d", len(rs.Errors))
				}
				if _, ok := rs.Errors["error-tag-rule"]; !ok {
					t.Error("Expected error for 'error-tag-rule'")
				}
			},
		},
		{
			name: "tag deduplication across rules",
			infoRules: []ruleMessage{
				{
					rule: Rule{
						RuleMeta: RuleMeta{
							RuleID: "tag-rule-1",
						},
						Perform: Perform{
							Tag: []string{"DuplicateTag"},
						},
						When: createTestConditional(true, nil, false),
					},
					ruleSetName: "test-ruleset",
				},
				{
					rule: Rule{
						RuleMeta: RuleMeta{
							RuleID: "tag-rule-2",
						},
						Perform: Perform{
							Tag: []string{"DuplicateTag"},
						},
						When: createTestConditional(true, nil, false),
					},
					ruleSetName: "test-ruleset",
				},
			},
			checkFunc: func(t *testing.T, resultContext ConditionContext, rs *konveyor.RuleSet) {
				if len(rs.Tags) != 1 {
					t.Errorf("Expected 1 unique tag, got %d: %v", len(rs.Tags), rs.Tags)
				}
				if rs.Tags[0] != "DuplicateTag" {
					t.Errorf("Expected 'DuplicateTag', got %s", rs.Tags[0])
				}
			},
		},
		{
			name: "UsesHasTags sorting",
			infoRules: []ruleMessage{
				{
					rule: Rule{
						RuleMeta: RuleMeta{
							RuleID:      "has-tags-rule",
							UsesHasTags: true,
						},
						Perform: Perform{
							Tag: []string{"DependentTag"},
						},
						When: createTestConditional(true, nil, false),
					},
					ruleSetName: "test-ruleset",
				},
				{
					rule: Rule{
						RuleMeta: RuleMeta{
							RuleID:      "normal-rule",
							UsesHasTags: false,
						},
						Perform: Perform{
							Tag: []string{"NormalTag"},
						},
						When: createTestConditional(true, nil, false),
					},
					ruleSetName: "test-ruleset",
				},
			},
			checkFunc: func(t *testing.T, resultContext ConditionContext, rs *konveyor.RuleSet) {
				foundNormal := false
				foundDependent := false
				for _, tag := range rs.Tags {
					if tag == "NormalTag" {
						foundNormal = true
					}
					if tag == "DependentTag" {
						foundDependent = true
					}
				}
				if !foundNormal {
					t.Error("Expected 'NormalTag' to be created")
				}
				if !foundDependent {
					t.Error("Expected 'DependentTag' to be created")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conditionContext := ConditionContext{
				Tags:     make(map[string]any),
				Template: make(map[string]ChainTemplate),
			}

			mapRuleSets := map[string]*konveyor.RuleSet{
				"test-ruleset": ruleEngine.createRuleSet(RuleSet{Name: "test-ruleset"}),
			}

			resultContext := ruleEngine.runTaggingRules(ctx, tt.infoRules, mapRuleSets, conditionContext, nil)

			tt.checkFunc(t, resultContext, mapRuleSets["test-ruleset"])
		})
	}
}

func TestCreateViolation(t *testing.T) {
	logrus.SetLevel(logrus.ErrorLevel)
	logrusLog := logrus.New()
	logrusLog.SetOutput(io.Discard)
	logrusLog.SetLevel(logrus.PanicLevel)
	log := logrusr.New(logrusLog)

	ctx := context.Background()

	tests := []struct {
		name      string
		setupFunc func(t *testing.T) (*ruleEngine, ConditionResponse, Rule, func())
		checkFunc func(t *testing.T, violation konveyor.Violation, err error)
	}{
		{
			name: "basic violation creation",
			setupFunc: func(t *testing.T) (*ruleEngine, ConditionResponse, Rule, func()) {
				ruleEngine := CreateRuleEngine(ctx, 10, log).(*ruleEngine)
				lineNum := 10
				msg := "Test violation"
				conditionResponse := ConditionResponse{
					Matched: true,
					Incidents: []IncidentContext{
						{
							FileURI:    "file:///test.java",
							LineNumber: &lineNum,
							Variables: map[string]any{
								"class": "TestClass",
							},
						},
					},
				}
				rule := Rule{
					RuleMeta: RuleMeta{
						RuleID:      "test-rule",
						Description: "Test rule description",
					},
					Perform: Perform{
						Message: Message{
							Text: &msg,
						},
					},
				}
				return ruleEngine, conditionResponse, rule, func() { ruleEngine.Stop() }
			},
			checkFunc: func(t *testing.T, violation konveyor.Violation, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if len(violation.Incidents) != 1 {
					t.Errorf("Expected 1 incident, got %d", len(violation.Incidents))
				}
				if violation.Description != "Test rule description" {
					t.Errorf("Expected description 'Test rule description', got '%s'", violation.Description)
				}
				if violation.Incidents[0].Message != "Test violation" {
					t.Errorf("Expected message 'Test violation', got '%s'", violation.Incidents[0].Message)
				}
			},
		},
		{
			name: "incident limit",
			setupFunc: func(t *testing.T) (*ruleEngine, ConditionResponse, Rule, func()) {
				ruleEngine := CreateRuleEngine(ctx, 10, log, WithIncidentLimit(2)).(*ruleEngine)
				lineNum1, lineNum2, lineNum3 := 10, 20, 30
				msg := "Test violation"
				conditionResponse := ConditionResponse{
					Matched: true,
					Incidents: []IncidentContext{
						{
							FileURI:    "file:///test1.java",
							LineNumber: &lineNum1,
							Variables:  map[string]any{},
						},
						{
							FileURI:    "file:///test2.java",
							LineNumber: &lineNum2,
							Variables:  map[string]any{},
						},
						{
							FileURI:    "file:///test3.java",
							LineNumber: &lineNum3,
							Variables:  map[string]any{},
						},
					},
				}
				rule := Rule{
					RuleMeta: RuleMeta{
						RuleID: "test-rule",
					},
					Perform: Perform{
						Message: Message{
							Text: &msg,
						},
					},
				}
				return ruleEngine, conditionResponse, rule, func() { ruleEngine.Stop() }
			},
			checkFunc: func(t *testing.T, violation konveyor.Violation, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if len(violation.Incidents) != 2 {
					t.Errorf("Expected 2 incidents (limited), got %d", len(violation.Incidents))
				}
			},
		},
		{
			name: "code snip limit",
			setupFunc: func(t *testing.T) (*ruleEngine, ConditionResponse, Rule, func()) {
				ruleEngine := CreateRuleEngine(ctx, 10, log, WithCodeSnipLimit(1)).(*ruleEngine)
				lineNum1, lineNum2 := 10, 20
				msg := "Test violation"
				conditionResponse := ConditionResponse{
					Matched: true,
					Incidents: []IncidentContext{
						{
							FileURI:    "file:///test.java",
							LineNumber: &lineNum1,
							Variables:  map[string]any{},
							CodeLocation: &Location{
								StartPosition: Position{Line: 9, Character: 0},
								EndPosition:   Position{Line: 10, Character: 0},
							},
						},
						{
							FileURI:    "file:///test.java",
							LineNumber: &lineNum2,
							Variables:  map[string]any{},
							CodeLocation: &Location{
								StartPosition: Position{Line: 19, Character: 0},
								EndPosition:   Position{Line: 20, Character: 0},
							},
						},
					},
				}
				rule := Rule{
					RuleMeta: RuleMeta{
						RuleID: "test-rule",
					},
					Perform: Perform{
						Message: Message{
							Text: &msg,
						},
					},
				}
				return ruleEngine, conditionResponse, rule, func() { ruleEngine.Stop() }
			},
			checkFunc: func(t *testing.T, violation konveyor.Violation, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				codeSnipCount := 0
				for _, incident := range violation.Incidents {
					if incident.CodeSnip != "" {
						codeSnipCount++
					}
				}
				if codeSnipCount > 1 {
					t.Errorf("Expected at most 1 code snip due to limit, got %d", codeSnipCount)
				}
			},
		},
		{
			name: "message template with variables",
			setupFunc: func(t *testing.T) (*ruleEngine, ConditionResponse, Rule, func()) {
				ruleEngine := CreateRuleEngine(ctx, 10, log).(*ruleEngine)
				lineNum := 10
				msg := "Found class {{class}} at line {{lineNumber}}"
				conditionResponse := ConditionResponse{
					Matched: true,
					Incidents: []IncidentContext{
						{
							FileURI:    "file:///test.java",
							LineNumber: &lineNum,
							Variables: map[string]any{
								"class": "TestClass",
							},
						},
					},
				}
				rule := Rule{
					RuleMeta: RuleMeta{
						RuleID: "test-rule",
					},
					Perform: Perform{
						Message: Message{
							Text: &msg,
						},
					},
				}
				return ruleEngine, conditionResponse, rule, func() { ruleEngine.Stop() }
			},
			checkFunc: func(t *testing.T, violation konveyor.Violation, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				expectedMsg := "Found class TestClass at line 10"
				if violation.Incidents[0].Message != expectedMsg {
					t.Errorf("Expected message '%s', got '%s'", expectedMsg, violation.Incidents[0].Message)
				}
			},
		},
		{
			name: "message template with dollar brace placeholders",
			setupFunc: func(t *testing.T) (*ruleEngine, ConditionResponse, Rule, func()) {
				ruleEngine := CreateRuleEngine(ctx, 10, log).(*ruleEngine)
				lineNum := 10
				msg := "Use the Quarkus BOM to omit version specification:\n<groupId>${{quarkus.platform.group-id}}</groupId>\n<artifactId>${{quarkus.platform.artifact-id}}</artifactId>\n<version>${{quarkus.platform.version}}</version>"
				conditionResponse := ConditionResponse{
					Matched: true,
					Incidents: []IncidentContext{
						{
							FileURI:    "file:///pom.xml",
							LineNumber: &lineNum,
							Variables:  map[string]any{},
						},
					},
				}
				rule := Rule{
					RuleMeta: RuleMeta{
						RuleID: "test-rule",
					},
					Perform: Perform{
						Message: Message{
							Text: &msg,
						},
					},
				}
				return ruleEngine, conditionResponse, rule, func() { ruleEngine.Stop() }
			},
			checkFunc: func(t *testing.T, violation konveyor.Violation, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				expectedMsg := "Use the Quarkus BOM to omit version specification:\n<groupId>${quarkus.platform.group-id}</groupId>\n<artifactId>${quarkus.platform.artifact-id}</artifactId>\n<version>${quarkus.platform.version}</version>"
				if violation.Incidents[0].Message != expectedMsg {
					t.Errorf("Expected message '%s', got '%s'", expectedMsg, violation.Incidents[0].Message)
				}
			},
		},
		{
			name: "message template mixing mustache variables and dollar brace placeholders",
			setupFunc: func(t *testing.T) (*ruleEngine, ConditionResponse, Rule, func()) {
				ruleEngine := CreateRuleEngine(ctx, 10, log).(*ruleEngine)
				lineNum := 10
				msg := "Found dependency {{dependency}} - replace with:\n<groupId>${{quarkus.group-id}}</groupId>\n<artifactId>{{artifact}}</artifactId>"
				conditionResponse := ConditionResponse{
					Matched: true,
					Incidents: []IncidentContext{
						{
							FileURI:    "file:///pom.xml",
							LineNumber: &lineNum,
							Variables: map[string]any{
								"dependency": "javax.enterprise",
								"artifact":   "quarkus-arc",
							},
						},
					},
				}
				rule := Rule{
					RuleMeta: RuleMeta{
						RuleID: "test-rule",
					},
					Perform: Perform{
						Message: Message{
							Text: &msg,
						},
					},
				}
				return ruleEngine, conditionResponse, rule, func() { ruleEngine.Stop() }
			},
			checkFunc: func(t *testing.T, violation konveyor.Violation, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				expectedMsg := "Found dependency javax.enterprise - replace with:\n<groupId>${quarkus.group-id}</groupId>\n<artifactId>quarkus-arc</artifactId>"
				if violation.Incidents[0].Message != expectedMsg {
					t.Errorf("Expected message '%s', got '%s'", expectedMsg, violation.Incidents[0].Message)
				}
			},
		},
		{
			name: "duplicate incident filtering",
			setupFunc: func(t *testing.T) (*ruleEngine, ConditionResponse, Rule, func()) {
				ruleEngine := CreateRuleEngine(ctx, 10, log).(*ruleEngine)
				lineNum := 10
				msg := "Duplicate test"
				conditionResponse := ConditionResponse{
					Matched: true,
					Incidents: []IncidentContext{
						{
							FileURI:    "file:///test.java",
							LineNumber: &lineNum,
							Variables:  map[string]any{},
						},
						{
							FileURI:    "file:///test.java",
							LineNumber: &lineNum,
							Variables:  map[string]any{},
						},
					},
				}
				rule := Rule{
					RuleMeta: RuleMeta{
						RuleID: "test-rule",
					},
					Perform: Perform{
						Message: Message{
							Text: &msg,
						},
					},
				}
				return ruleEngine, conditionResponse, rule, func() { ruleEngine.Stop() }
			},
			checkFunc: func(t *testing.T, violation konveyor.Violation, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if len(violation.Incidents) != 1 {
					t.Errorf("Expected 1 incident (duplicates filtered), got %d", len(violation.Incidents))
				}
			},
		},
		{
			name: "label deduplication",
			setupFunc: func(t *testing.T) (*ruleEngine, ConditionResponse, Rule, func()) {
				ruleEngine := CreateRuleEngine(ctx, 10, log).(*ruleEngine)
				lineNum := 10
				msg := "Test"
				conditionResponse := ConditionResponse{
					Matched: true,
					Incidents: []IncidentContext{
						{
							FileURI:    "file:///test.java",
							LineNumber: &lineNum,
							Variables:  map[string]any{},
						},
					},
				}
				rule := Rule{
					RuleMeta: RuleMeta{
						RuleID: "test-rule",
						Labels: []string{"label1", "label2", "label1", "label3", "label2"},
					},
					Perform: Perform{
						Message: Message{
							Text: &msg,
						},
					},
				}
				return ruleEngine, conditionResponse, rule, func() { ruleEngine.Stop() }
			},
			checkFunc: func(t *testing.T, violation konveyor.Violation, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if len(violation.Labels) != 3 {
					t.Errorf("Expected 3 unique labels, got %d: %v", len(violation.Labels), violation.Labels)
				}
			},
		},
		{
			name: "incident selector filtering",
			setupFunc: func(t *testing.T) (*ruleEngine, ConditionResponse, Rule, func()) {
				ruleEngine := CreateRuleEngine(ctx, 10, log, WithIncidentSelector("kind=class")).(*ruleEngine)
				lineNum1, lineNum2 := 10, 20
				msg := "Test"
				conditionResponse := ConditionResponse{
					Matched: true,
					Incidents: []IncidentContext{
						{
							FileURI:    "file:///test1.java",
							LineNumber: &lineNum1,
							Variables: map[string]any{
								"kind": "class",
							},
						},
						{
							FileURI:    "file:///test2.java",
							LineNumber: &lineNum2,
							Variables: map[string]any{
								"kind": "method",
							},
						},
					},
				}
				rule := Rule{
					RuleMeta: RuleMeta{
						RuleID: "test-rule",
					},
					Perform: Perform{
						Message: Message{
							Text: &msg,
						},
					},
				}
				return ruleEngine, conditionResponse, rule, func() { ruleEngine.Stop() }
			},
			checkFunc: func(t *testing.T, violation konveyor.Violation, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if len(violation.Incidents) != 1 {
					t.Errorf("Expected 1 incident after selector filtering, got %d", len(violation.Incidents))
				}
				if violation.Incidents[0].Variables["kind"] != "class" {
					t.Errorf("Expected filtered incident to have kind=class")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ruleEngine, conditionResponse, rule, cleanup := tt.setupFunc(t)
			defer cleanup()

			violation, err := ruleEngine.createViolation(ctx, conditionResponse, rule, nil)

			tt.checkFunc(t, violation, err)
		})
	}
}

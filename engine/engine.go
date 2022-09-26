package engine

import (
	"context"
	"fmt"
	"sync"
)

type RuleEngine interface {
	RunRules(context context.Context, rules []Rule)
	Stop()
}

type ruleMessage struct {
	rule       Rule
	returnChan chan response
}

type response struct {
	conditionResponse InnerConndtionResponse
	err               error
	rule              Rule
}

type ruleEngine struct {
	// Buffereed channel where Rule Processors are watching
	ruleProcessing chan ruleMessage
	cancelFunc     context.CancelFunc
}

func CreateRuleEngine(ctx context.Context, workers int) RuleEngine {
	// Only allow for 10 rules to be processed at once.
	ruleProcessor := make(chan ruleMessage, 10)

	ctx, cancelFunc := context.WithCancel(ctx)

	for i := 0; i < workers; i++ {
		go processRuleWorker(ctx, ruleProcessor)
	}

	return &ruleEngine{
		ruleProcessing: ruleProcessor,
		cancelFunc:     cancelFunc,
	}
}

func (r *ruleEngine) Stop() {
	r.cancelFunc()
}

func processRuleWorker(ctx context.Context, ruleMessages chan ruleMessage) {
	for {
		select {
		case m := <-ruleMessages:
			bo, err := processRule(m.rule)
			m.returnChan <- response{
				conditionResponse: bo,
				err:               err,
				rule:              m.rule,
			}
		case <-ctx.Done():
			return
		}
	}
}

// This will run the rules async, fanning them out, fanning them in, and then generating the results. will block until completed.
func (r *ruleEngine) RunRules(ctx context.Context, rules []Rule) {
	// determine if we should run

	ctx, cancelFunc := context.WithCancel(ctx)

	// Need a better name for this thing
	ret := make(chan response)

	wg := &sync.WaitGroup{}
	for _, rule := range rules {
		fmt.Printf("\nhere")
		wg.Add(1)
		r.ruleProcessing <- ruleMessage{
			rule:       rule,
			returnChan: ret,
		}
	}

	responses := []response{}
	// Handle returns
	go func() {
		for {
			select {
			case response := <-ret:
				if response.conditionResponse.Passed {
					responses = append(responses, response)

				} else {
					// Log that rule did not pass
					fmt.Printf("Rule did not pass")
				}
				responses = append(responses, response)
				wg.Done()
			case <-ctx.Done():
				// at this point we should just return the function, we may want to close the wait group too.
				return
			}
		}
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	// Wait for all the rules to process
	select {
	case <-done:
		fmt.Printf("done waiting for rules to be processed")
	case <-ctx.Done():
		fmt.Printf("Context canceled running of rules")
	}
	// Cannel running go-routine
	cancelFunc()

}

func processRule(rule Rule) (InnerConndtionResponse, error) {

	// Here is what a worker should run when getting a rule.
	// For now, lets not fan out the running of conditions.

	if rule.When.InnerCondition != nil {
		//IF there is an inner conndtion here, that means there is only a single condition to run.

		//Use output
		return rule.When.InnerCondition.Evaluate()

	} else {
		if len(rule.When.And) > 0 {
			return evaluateAndCondtions(rule.When.And)
		} else if len(rule.When.Or) > 0 {
			return evaluateOrConditions(rule.When.Or)
		} else {
			return InnerConndtionResponse{}, fmt.Errorf("invalid when condition")
		}
	}
}

func evaluateAndCondtions(conditions []Condition) (InnerConndtionResponse, error) {
	// Make sure we allow for short circt.

	if len(conditions) == 0 {
		return InnerConndtionResponse{}, fmt.Errorf("condtions must not be empty while evaluationg")
	}

	fullResponse := InnerConndtionResponse{Passed: true}
	for _, c := range conditions {
		if c.When != nil {
			var response InnerConndtionResponse
			var err error
			if len(c.When.And) > 0 {
				response, err = evaluateAndCondtions(c.When.And)
			} else if len(c.When.Or) > 0 {
				response, err = evaluateOrConditions(c.When.Or)
			} else {
				return response, fmt.Errorf("invalid when clause")
			}
			if err != nil {
				return response, err
			}

			// Short cirtcut loop if one and condition fails
			if !response.Passed {
				fmt.Printf("\nhere in !response.Passed: %v\n", response)
				return response, err
			}
		} else {
			response, err := c.InnerCondition.Evaluate()
			if !response.Passed || err != nil {
				return response, err
			}
			fullResponse.ConditionHitContext = append(fullResponse.ConditionHitContext, response.ConditionHitContext...)
		}
	}

	return fullResponse, nil
}

func evaluateOrConditions(conditions []Condition) (InnerConndtionResponse, error) {
	for _, c := range conditions {
		if c.When != nil {
			var response InnerConndtionResponse
			var err error
			if len(c.When.And) > 0 {
				response, err = evaluateAndCondtions(c.When.And)
			} else if len(c.When.Or) > 0 {
				response, err = evaluateOrConditions(c.When.Or)
			} else {
				return response, fmt.Errorf("invalid when clause")
			}

			// Short cirtcut loop if one and condition passes we can move on
			// We may not want to do this,
			if response.Passed {
				return response, err
			}
		} else {
			response, err := c.InnerCondition.Evaluate()
			if response.Passed {
				return response, err
			}
			if err != nil {
				return InnerConndtionResponse{}, err
			}
		}
	}

	return InnerConndtionResponse{}, nil
}

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
	Rule       Rule
	ret        chan response
	cancelFunc context.CancelFunc
}

type response struct {
	bo   bool
	err  error
	rule Rule
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
			bo, err := processRule(m.Rule)
			m.ret <- response{
				bo:   bo,
				err:  err,
				rule: m.Rule,
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
			Rule: rule,
			ret:  ret,
		}
	}

	responses := []response{}
	// Handle returns
	go func() {
		for {
			select {
			case response := <-ret:
				fmt.Printf("\nhere: %#v", response)
				responses = append(responses, response)
				wg.Done()
			case <-ctx.Done():
				// at this point we should just return the function, we may want to close the wait group too.
				return
			}
		}
	}()

	// Wait for all the rules to process
	wg.Wait()
	fmt.Printf("\nhere after done")
	// Cannel running go-routine
	cancelFunc()
	fmt.Printf("\nresponses: %#v", responses)
}

func processRule(rule Rule) (bool, error) {

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
			return false, fmt.Errorf("invalid when condition")
		}
	}
}

func evaluateAndCondtions(conditions []Condition) (bool, error) {
	// Make sure we allow for short circt.

	if len(conditions) == 0 {
		return false, fmt.Errorf("condtions must not be empty while evaluationg")
	}

	for _, c := range conditions {
		if c.When != nil {
			var triggerRule bool
			var err error
			if len(c.When.And) > 0 {
				triggerRule, err = evaluateAndCondtions(c.When.And)
			} else if len(c.When.Or) > 0 {
				triggerRule, err = evaluateOrConditions(c.When.Or)
			} else {
				return false, fmt.Errorf("invalid when clause")
			}
			if err != nil {
				return false, err
			}

			// Short cirtcut loop if one and condition fails
			if !triggerRule {
				return false, err

			}

		} else {
			triggerRule, err := c.InnerCondition.Evaluate()
			if !triggerRule || err != nil {
				return false, err
			}
		}
	}

	return true, nil

}

func evaluateOrConditions(conditions []Condition) (bool, error) {
	for _, c := range conditions {
		if c.When != nil {
			var triggerRule bool
			var err error
			if len(c.When.And) > 0 {
				triggerRule, err = evaluateAndCondtions(c.When.And)
			} else if len(c.When.Or) > 0 {
				triggerRule, err = evaluateOrConditions(c.When.Or)
			} else {
				return false, fmt.Errorf("invalid when clause")
			}

			// Short cirtcut loop if one and condition passes we can move on
			if triggerRule {
				return true, err

			}
		} else {
			triggerRule, err := c.InnerCondition.Evaluate()
			if triggerRule {
				return true, err
			}
			if err != nil {
				return false, err
			}
		}
	}

	return false, nil
}

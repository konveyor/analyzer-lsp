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
	conditionResponse CondtionResponse
	err               error
	rule              Rule
}

type ruleEngine struct {
	// Buffered channel where Rule Processors are watching
	ruleProcessing chan ruleMessage
	cancelFunc     context.CancelFunc
}

func CreateRuleEngine(ctx context.Context, workers int) RuleEngine {
	// Only allow for 10 rules to be waiting in the buffer at once.
	// Adding more workers will increase the number of rules running at once.
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
				if !response.conditionResponse.Passed {
					responses = append(responses, response)

				} else {
					// Log that rule did not pass
					fmt.Printf("Rule did not error")
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
		fmt.Printf("\ndone waiting for rules to be processed\n")
	case <-ctx.Done():
		fmt.Printf("Context canceled running of rules")
	}
	// Cannel running go-routine
	cancelFunc()

	// TODO: Here we need to process the rule reponses.
	fmt.Printf("\nresponses: %#v", responses)

}

func processRule(rule Rule) (CondtionResponse, error) {

	// Here is what a worker should run when getting a rule.
	// For now, lets not fan out the running of conditions.
	return rule.When.Evaluate()

}

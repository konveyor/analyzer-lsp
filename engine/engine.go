package engine

import (
	"context"
	"fmt"
	"sync"

	"gopkg.in/yaml.v2"

	"github.com/go-logr/logr"
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
	ConditionResponse ConditionResponse `yaml:"conditionResponse"`
	Err               error             `yaml:"err"`
	Rule              Rule              `yaml:"rule"`
}

type ruleEngine struct {
	// Buffered channel where Rule Processors are watching
	ruleProcessing chan ruleMessage
	cancelFunc     context.CancelFunc
	logger         logr.Logger
}

func CreateRuleEngine(ctx context.Context, workers int, log logr.Logger) RuleEngine {
	// Only allow for 10 rules to be waiting in the buffer at once.
	// Adding more workers will increase the number of rules running at once.
	ruleProcessor := make(chan ruleMessage, 10)

	ctx, cancelFunc := context.WithCancel(ctx)

	for i := 0; i < workers; i++ {
		logger := log.WithValues("workder", i)
		go processRuleWorker(ctx, ruleProcessor, logger)
	}

	return &ruleEngine{
		ruleProcessing: ruleProcessor,
		cancelFunc:     cancelFunc,
		logger:         log,
	}
}

func (r *ruleEngine) Stop() {
	r.cancelFunc()
}

func processRuleWorker(ctx context.Context, ruleMessages chan ruleMessage, logger logr.Logger) {
	for {
		select {
		case m := <-ruleMessages:
			logger.V(5).Info("taking rule")
			bo, err := processRule(m.rule, logger)
			m.returnChan <- response{
				ConditionResponse: bo,
				Err:               err,
				Rule:              m.rule,
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
	r.logger.V(5).Info("All rules added buffer, waiting for engine to complete")

	responses := []response{}
	// Handle returns
	go func() {
		for {
			select {
			case response := <-ret:
				if !response.ConditionResponse.Passed {
					responses = append(responses, response)
				} else {
					// Log that rule did not pass
					r.logger.V(5).Info("rule was evaluated, and we did not find a violation", "response", response)

				}
				wg.Done()
			case <-ctx.Done():
				// At this point we should just return the function, we may want to close the wait group too.
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
		r.logger.V(2).Info("done processing all the rules")
	case <-ctx.Done():
		r.logger.V(1).Info("processing of rules was canceled")
	}
	// Cannel running go-routine
	cancelFunc()
	b, err := yaml.Marshal(responses)
	if err != nil {
		fmt.Println("error:", err)
	}
	// TODO: Here we need to process the rule reponses.
	fmt.Print(string(b))
}

func processRule(rule Rule, log logr.Logger) (ConditionResponse, error) {
	// Here is what a worker should run when getting a rule.
	// For now, lets not fan out the running of conditions.
	return rule.When.Evaluate(log)

}

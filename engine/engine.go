package engine

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"gopkg.in/yaml.v2"

	"github.com/cbroglie/mustache"
	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/hubapi"
	"github.com/konveyor/analyzer-lsp/provider/lib"
	"github.com/konveyor/analyzer-lsp/tracing"
)

type RuleEngine interface {
	RunRules(context context.Context, rules []RuleSet) []hubapi.RuleSet
	Stop()
}

type ruleMessage struct {
	rule        Rule
	ruleSetName string
	ctx         ConditionContext
	returnChan  chan response
}

type response struct {
	ConditionResponse ConditionResponse `yaml:"conditionResponse"`
	Err               error             `yaml:"err"`
	Rule              Rule              `yaml:"rule"`
	RuleSetName       string
}

type ruleEngine struct {
	// Buffered channel where Rule Processors are watching
	ruleProcessing chan ruleMessage
	cancelFunc     context.CancelFunc
	logger         logr.Logger

	wg *sync.WaitGroup
}

func CreateRuleEngine(ctx context.Context, workers int, log logr.Logger) RuleEngine {
	// Only allow for 10 rules to be waiting in the buffer at once.
	// Adding more workers will increase the number of rules running at once.
	ruleProcessor := make(chan ruleMessage, 10)

	ctx, cancelFunc := context.WithCancel(ctx)
	wg := &sync.WaitGroup{}

	for i := 0; i < workers; i++ {
		logger := log.WithValues("worker", i)
		wg.Add(1)
		go processRuleWorker(ctx, ruleProcessor, logger, wg)
	}

	return &ruleEngine{
		ruleProcessing: ruleProcessor,
		cancelFunc:     cancelFunc,
		logger:         log,
		wg:             wg,
	}
}

func (r *ruleEngine) Stop() {
	r.cancelFunc()
	r.logger.V(5).Info("rule engine stopping")
	r.wg.Wait()
}

func processRuleWorker(ctx context.Context, ruleMessages chan ruleMessage, logger logr.Logger, wg *sync.WaitGroup) {
	for {
		select {
		case m := <-ruleMessages:
			logger.V(5).Info("taking rule")
			m.ctx.Template = make(map[string]lib.ChainTemplate)
			bo, err := processRule(ctx, m.rule, m.ctx, logger)
			logger.V(5).Info("finished rule", "response", bo, "error", err)
			m.returnChan <- response{
				ConditionResponse: bo,
				Err:               err,
				Rule:              m.rule,
				RuleSetName:       m.ruleSetName,
			}
		case <-ctx.Done():
			logger.V(5).Info("stopping rule worker")
			wg.Done()
			return
		}
	}
}

func (r *ruleEngine) createRuleSet(ruleSet RuleSet) hubapi.RuleSet {
	rs := hubapi.RuleSet{
		Name:        ruleSet.Name,
		Description: ruleSet.Description,
		Tags:        ruleSet.Tags,
		Violations:  map[string]hubapi.Violation{},
	}

	if ruleSet.Source != nil {
		rs.Source = &hubapi.RuleSetTechnology{
			ID:           ruleSet.Source.ID,
			VersionRange: ruleSet.Source.VersionRange,
		}
	}

	if ruleSet.Target != nil {
		rs.Target = &hubapi.RuleSetTechnology{
			ID:           ruleSet.Target.ID,
			VersionRange: ruleSet.Target.VersionRange,
		}
	}
	return rs
}

// This will run the meta rules first, synchronously, generating metadata to pass on further as context to other rules
// then runs remaining rules async, fanning them out, fanning them in, finally generating the results. will block until completed.
func (r *ruleEngine) RunRules(ctx context.Context, ruleSets []RuleSet) []hubapi.RuleSet {
	// determine if we should run

	ctx, cancelFunc := context.WithCancel(ctx)

	// filter rules that generate metadata indexed by ruleset name, they run first
	metaRules := []ruleMessage{}
	mapRuleSets := map[string]hubapi.RuleSet{}
	ruleMessages := []ruleMessage{}
	for _, ruleSet := range ruleSets {
		mapRuleSets[ruleSet.Name] = r.createRuleSet(ruleSet)
		for _, rule := range ruleSet.Rules {
			if rule.Perform.Tag == nil {
				ruleMessages = append(ruleMessages, ruleMessage{
					rule:        rule,
					ruleSetName: ruleSet.Name,
				})
			} else {
				metaRules = append(metaRules, ruleMessage{
					rule:        rule,
					ruleSetName: ruleSet.Name,
				})
			}
		}
	}

	ruleContext := r.runMetaRules(ctx, metaRules, mapRuleSets)

	// Need a better name for this thing
	ret := make(chan response)

	wg := &sync.WaitGroup{}
	// Handle returns
	go func() {
		for {
			select {
			case response := <-ret:
				func() {
					r.logger.Info("rule returned", "rule", response)
					defer wg.Done()
					if response.Err != nil {
						r.logger.Error(response.Err, "failed to evaluate rule", "ruleID", response.Rule.RuleID)
					} else if response.ConditionResponse.Matched {
						violation, err := r.createViolation(response.ConditionResponse, response.Rule)
						if err != nil {
							r.logger.Error(err, "unable to create violation from response")
						}
						rs, ok := mapRuleSets[response.RuleSetName]
						if !ok {
							r.logger.Info("this should never happen that we don't find the ruleset")
						}
						rs.Violations[response.Rule.RuleID] = violation
					} else {
						// Log that rule did not pass
						r.logger.V(5).Info("rule was evaluated, and we did not find a violation", "response", response)
					}
				}()
			case <-ctx.Done():
				// At this point we should just return the function, we may want to close the wait group too.
				return
			}
		}
	}()

	for _, rule := range ruleMessages {
		wg.Add(1)
		rule.returnChan = ret
		rule.ctx = ruleContext
		r.ruleProcessing <- rule
	}
	r.logger.V(5).Info("All rules added buffer, waiting for engine to complete")

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
	responses := []hubapi.RuleSet{}
	for _, ruleSet := range mapRuleSets {
		responses = append(responses, ruleSet)
	}
	// Cannel running go-routine
	cancelFunc()
	b, err := yaml.Marshal(responses)
	if err != nil {
		r.logger.Error(err, "unable to marshal responses")
	}
	// TODO: Here we need to process the rule reponses.
	r.logger.V(5).Info(string(b))
	return responses
}

// runMetaRules filters and runs info rules synchronously
// returns list of non-info rules, a context to pass to them
func (r *ruleEngine) runMetaRules(ctx context.Context, infoRules []ruleMessage, mapRuleSets map[string]hubapi.RuleSet) ConditionContext {
	context := ConditionContext{
		Tags:     make(map[string]interface{}),
		Template: make(map[string]lib.ChainTemplate),
	}
	for _, ruleMessage := range infoRules {
		rule := ruleMessage.rule
		response, err := processRule(ctx, rule, context, r.logger)
		if err != nil {
			r.logger.Error(err, "failed to evaluate rule", "ruleID", rule.RuleID)
		} else if response.Matched {
			r.logger.V(5).Info("info rule was matched", "ruleID", rule.RuleID)
			for _, tag := range rule.Perform.Tag {
				context.Tags[tag] = true
			}
			violation, err := r.createViolation(response, rule)
			if err != nil {
				r.logger.Error(err, "unable to create violation from response")
			}
			violation.Tags = append(violation.Tags, rule.Perform.Tag...)
			rs, ok := mapRuleSets[ruleMessage.ruleSetName]
			if !ok {
				r.logger.Info("this should never happen that we don't find the ruleset")
			}
			rs.Violations[rule.RuleID] = violation
		}
	}
	return context
}

func processRule(ctx context.Context, rule Rule, ruleCtx ConditionContext, log logr.Logger) (ConditionResponse, error) {
	ctx, span := tracing.StartNewSpan(
		ctx, "process-rule", attribute.Key("rule").String(rule.RuleID))
	defer span.End()
	// Here is what a worker should run when getting a rule.
	// For now, lets not fan out the running of conditions.
	return rule.When.Evaluate(ctx, log, ruleCtx)

}

func (r *ruleEngine) createViolation(conditionResponse ConditionResponse, rule Rule) (hubapi.Violation, error) {
	incidents := []hubapi.Incident{}
	for _, m := range conditionResponse.Incidents {

		incident := hubapi.Incident{
			URI:    m.FileURI,
			Extras: m.Extras,
		}
		links := []hubapi.Link{}
		if len(m.Links) > 0 {
			for _, l := range m.Links {
				links = append(links, hubapi.Link{
					URL:   l.URL,
					Title: l.Title,
				})
			}
		}
		// extras, err := json.Marshal(m.Extras)
		// if err != nil {
		// 	return hubapi.Violation{}, err
		// }
		if rule.Perform.Message != nil {
			templateString, err := r.createPerformString(*rule.Perform.Message, m.Extras)
			if err != nil {
				r.logger.Error(err, "unable to create template string")
			}
			incident.Message = templateString
		}

		incidents = append(incidents, incident)
	}

	return hubapi.Violation{
		Description: rule.Description,
		Labels:      rule.Labels,
		Category:    rule.Category,
		Incidents:   incidents,
		Extras:      []byte{},
		Effort:      rule.Effort,
		Links:       rule.Links,
	}, nil
}

func (r *ruleEngine) createPerformString(messageTemplate string, ctx map[string]interface{}) (string, error) {
	return mustache.Render(messageTemplate, ctx)
}

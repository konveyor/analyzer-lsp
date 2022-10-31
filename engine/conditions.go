package engine

import (
	"fmt"

	"github.com/go-logr/logr"
)

var _ Conditional = AndCondition{}
var _ Conditional = OrCondition{}
var _ Conditional = ChainCondition{}

type ConditionResponse struct {
	Passed bool `yaml:"passed"`
	// For each time the condition is hit, add all of the context.
	// keys here, will be used in the message.
	ConditionHitContext []map[string]string    `yaml:"conditionHitContext"`
	TemplateContext     map[string]interface{} `yaml:",inline"`
}

type ConditionEntry struct {
	From                   string
	As                     string
	ProviderSpecificConfig Conditional
}

type Conditional interface {
	Evaluate(log logr.Logger, ctx map[string]interface{}) (ConditionResponse, error)
}

type Rule struct {
	Perform string      `yaml:"perform,omitempty"`
	When    Conditional `yaml:"when,omitempty"`
}

type AndCondition struct {
	Conditions []ConditionEntry `yaml:"and"`
}

func (a AndCondition) Evaluate(log logr.Logger, ctx map[string]interface{}) (ConditionResponse, error) {

	if len(a.Conditions) == 0 {
		return ConditionResponse{}, fmt.Errorf("conditions must not be empty while evaluating")
	}

	fullResponse := ConditionResponse{
		Passed:              true,
		ConditionHitContext: []map[string]string{},
		TemplateContext:     map[string]interface{}{},
	}
	for _, c := range a.Conditions {
		response, err := c.ProviderSpecificConfig.Evaluate(log, ctx)
		if err != nil {
			return ConditionResponse{}, err
		}

		if !response.Passed {
			fullResponse.Passed = false
		}

		copy(fullResponse.ConditionHitContext, response.ConditionHitContext)

		for k, v := range response.TemplateContext {
			fullResponse.TemplateContext[k] = v
		}
	}

	return fullResponse, nil
}

type OrCondition struct {
	Conditions []ConditionEntry `yaml:"or"`
}

func (o OrCondition) Evaluate(log logr.Logger, ctx map[string]interface{}) (ConditionResponse, error) {
	if len(o.Conditions) == 0 {
		return ConditionResponse{}, fmt.Errorf("conditions must not be empty while evaluationg")
	}

	// We need to append template context, and not short circut.
	fullResponse := ConditionResponse{
		Passed:              false,
		ConditionHitContext: []map[string]string{},
		TemplateContext:     map[string]interface{}{},
	}
	for _, c := range o.Conditions {
		response, err := c.ProviderSpecificConfig.Evaluate(log, ctx)
		if err != nil {
			return ConditionResponse{}, err
		}
		if !fullResponse.Passed && response.Passed {
			fullResponse.Passed = true
		}

		copy(fullResponse.ConditionHitContext, response.ConditionHitContext)

		for k, v := range response.TemplateContext {
			fullResponse.TemplateContext[k] = v
		}

	}

	return fullResponse, nil
}

type ChainCondition struct {
	Conditions []ConditionEntry `yaml:"chain"`
}

func (ch ChainCondition) Evaluate(log logr.Logger, ctx map[string]interface{}) (ConditionResponse, error) {

	if len(ch.Conditions) == 0 {
		return ConditionResponse{}, fmt.Errorf("conditions must not be empty while evaluating")
	}

	fullResponse := ConditionResponse{Passed: true}
	var hitContext []map[string]string
	var passed bool
	for _, c := range ch.Conditions {
		var response ConditionResponse
		var err error

		if _, ok := ctx[c.From]; !ok && c.From != "" {
			// Short circut w/ error here
			// TODO: determine if this is the right thing, I am assume the full rule should fail here
			return ConditionResponse{}, fmt.Errorf("unable to find context value: %v", c.From)
		}

		response, err = c.ProviderSpecificConfig.Evaluate(log, ctx)
		if err != nil {
			return fullResponse, err
		}

		if c.As != "" {
			ctx[c.As] = response.TemplateContext
		}
		passed = response.Passed
		// TODO, we need to make this like appendable I think?
		hitContext = response.ConditionHitContext
	}
	fullResponse.Passed = passed
	fullResponse.TemplateContext = ctx
	fullResponse.ConditionHitContext = hitContext

	return fullResponse, nil
}

package engine

import (
	"fmt"

	"github.com/go-logr/logr"
)

var _ Conditional = AndCondition{}
var _ Conditional = OrCondition{}
var _ Conditional = ChainCondition{}

type ConditionResponse struct {
	Matched bool `yaml:"matched"`
	// For each time the condition is hit, add all of the context.
	// keys here, will be used in the message.
	Incidents       []IncidentContext      `yaml:"incidents"`
	TemplateContext map[string]interface{} `yaml:",inline"`
}

type ConditionContext struct {
	Tags     map[string]interface{} `yaml:"tags"`
	Template map[string]interface{} `yaml:"template"`
}

type ConditionEntry struct {
	From                   string
	As                     string
	Ignorable              bool
	Not                    bool
	ProviderSpecificConfig Conditional
}

type IncidentContext struct {
	FileURI string                 `yaml:"fileURI"`
	Effort  *int                   `yaml:"effort"`
	Extras  map[string]interface{} `yaml:"extras"`
	Links   []ExternalLinks        `yaml:"externalLink"`
}

type ExternalLinks struct {
	URL   string `yaml:"url"`
	Title string `yaml:"title"`
}

type Conditional interface {
	Evaluate(log logr.Logger, ctx ConditionContext) (ConditionResponse, error)
}

type Rule struct {
	RuleID      string      `yaml:"ruleID,omitempty"`
	Description string      `yaml:"description,omitempty"`
	Category    string      `yaml:"category,omitempty"`
	Perform     Perform     `yaml:",inline"`
	When        Conditional `yaml:"when,omitempty"`
}

type Perform struct {
	Message *string  `yaml:"message,omitempty"`
	Tag     []string `yaml:"tag,omitempty"`
}

func (p *Perform) Validate() error {
	if p.Message != nil && p.Tag != nil {
		return fmt.Errorf("cannot perform message and tag both")
	}
	if p.Message == nil && p.Tag == nil {
		return fmt.Errorf("either message or tag must be set")
	}
	return nil
}

type AndCondition struct {
	Conditions []ConditionEntry `yaml:"and"`
}

func (a AndCondition) Evaluate(log logr.Logger, ctx ConditionContext) (ConditionResponse, error) {
	if len(a.Conditions) == 0 {
		return ConditionResponse{}, fmt.Errorf("conditions must not be empty while evaluating")
	}

	fullResponse := ConditionResponse{
		Matched:         true,
		Incidents:       []IncidentContext{},
		TemplateContext: map[string]interface{}{},
	}
	for _, c := range a.Conditions {
		response, err := c.ProviderSpecificConfig.Evaluate(log, ctx)
		if err != nil {
			return ConditionResponse{}, err
		}

		matched := response.Matched
		if c.Not {
			matched = !matched

		}
		if !matched {
			fullResponse.Matched = false
		}

		if !c.Ignorable {
			fullResponse.Incidents = append(fullResponse.Incidents, response.Incidents...)
		}

		for k, v := range response.TemplateContext {
			fullResponse.TemplateContext[k] = v
		}
	}

	return fullResponse, nil
}

type OrCondition struct {
	Conditions []ConditionEntry `yaml:"or"`
}

func (o OrCondition) Evaluate(log logr.Logger, ctx ConditionContext) (ConditionResponse, error) {
	if len(o.Conditions) == 0 {
		return ConditionResponse{}, fmt.Errorf("conditions must not be empty while evaluationg")
	}

	// We need to append template context, and not short circut.
	fullResponse := ConditionResponse{
		Matched:         false,
		Incidents:       []IncidentContext{},
		TemplateContext: map[string]interface{}{},
	}
	for _, c := range o.Conditions {
		response, err := c.ProviderSpecificConfig.Evaluate(log, ctx)
		if err != nil {
			return ConditionResponse{}, err
		}
		matched := response.Matched
		if c.Not {
			matched = !matched
		}
		if matched {
			fullResponse.Matched = true
		}

		if !c.Ignorable {
			fullResponse.Incidents = append(fullResponse.Incidents, response.Incidents...)
		}

		for k, v := range response.TemplateContext {
			fullResponse.TemplateContext[k] = v
		}

	}
	return fullResponse, nil
}

type ChainCondition struct {
	Conditions []ConditionEntry `yaml:"chain"`
}

func (ch ChainCondition) Evaluate(log logr.Logger, ctx ConditionContext) (ConditionResponse, error) {

	if len(ch.Conditions) == 0 {
		return ConditionResponse{}, fmt.Errorf("conditions must not be empty while evaluating")
	}

	fullResponse := ConditionResponse{Matched: true}
	incidents := []IncidentContext{}
	var matched bool
	for _, c := range ch.Conditions {
		var response ConditionResponse
		var err error

		if _, ok := ctx.Template[c.From]; !ok && c.From != "" {
			// Short circut w/ error here
			// TODO: determine if this is the right thing, I am assume the full rule should fail here
			return ConditionResponse{}, fmt.Errorf("unable to find context value: %v", c.From)
		}

		response, err = c.ProviderSpecificConfig.Evaluate(log, ctx)
		if err != nil {
			return fullResponse, err
		}

		if c.As != "" {
			ctx.Template[c.As] = response.TemplateContext
		}
		matched = response.Matched
		if c.Not {
			matched = !matched
		}
		if !c.Ignorable {
			incidents = append(incidents, response.Incidents...)
		}
	}
	fullResponse.Matched = matched
	fullResponse.TemplateContext = ctx.Template
	fullResponse.Incidents = incidents

	return fullResponse, nil
}

func (ce ConditionEntry) Evaluate(log logr.Logger, ctx ConditionContext) (ConditionResponse, error) {
	response, err := ce.ProviderSpecificConfig.Evaluate(log, ctx)
	if err != nil {
		return ConditionResponse{}, err
	}

	matched := response.Matched
	if ce.Not {
		matched = !matched
	}

	response.Matched = matched
	return response, nil
}

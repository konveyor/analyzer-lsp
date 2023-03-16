package engine

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/hubapi"
	"github.com/konveyor/analyzer-lsp/provider/lib"
	"github.com/konveyor/analyzer-lsp/tracing"
)

var _ Conditional = AndCondition{}
var _ Conditional = OrCondition{}

type ConditionResponse struct {
	Matched bool `yaml:"matched"`
	// For each time the condition is hit, add all of the context.
	// keys here, will be used in the message.
	Incidents       []IncidentContext      `yaml:"incidents"`
	TemplateContext map[string]interface{} `yaml:",inline"`
}

type ConditionContext struct {
	Tags     map[string]interface{}       `yaml:"tags"`
	Template map[string]lib.ChainTemplate `yaml:"template"`
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
	Links   []hubapi.Link          `yaml:"externalLink"`
}

type Conditional interface {
	Evaluate(ctx context.Context, log logr.Logger, condCtx ConditionContext) (ConditionResponse, error)
}

type RuleSet struct {
	Name        string             `json:"name,omitempty"`
	Description string             `json:"description,omitempty"`
	Source      *RuleSetTechnology `json:"source,omitempty"`
	Target      *RuleSetTechnology `json:"target,omitempty"`
	Tags        []string           `json:"tags,omitempty"`
	Rules       []Rule             `json:"rules,omitempty"`
}

type RuleSetTechnology struct {
	ID           string `json:"id,omitempty"`
	VersionRange string `json:"versionRange,omitempty"`
}

type Rule struct {
	RuleID      string           `yaml:"ruleID,omitempty" json:"ruleID,omitempty"`
	Description string           `yaml:"description,omitempty" json:"description,omitempty"`
	Category    *hubapi.Category `yaml:"category,omitempty" json:"category,omitempty"`
	Links       []hubapi.Link    `yaml:"links,omitempty" json:"links,omitempty"`
	Labels      []string         `yaml:"labels,omitempty" json:"labels,omitempty"`
	Effort      *int             `json:"effort,omitempty"`
	Perform     Perform          `yaml:",inline" json:"perform,omitempty"`
	When        Conditional      `yaml:"when,omitempty" json:"when,omitempty"`
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

func (a AndCondition) Evaluate(ctx context.Context, log logr.Logger, condCtx ConditionContext) (ConditionResponse, error) {
	ctx, span := tracing.StartNewSpan(ctx, "and-condition")
	defer span.End()

	if len(a.Conditions) == 0 {
		return ConditionResponse{}, fmt.Errorf("conditions must not be empty while evaluating")
	}

	fullResponse := ConditionResponse{
		Matched:         true,
		Incidents:       []IncidentContext{},
		TemplateContext: map[string]interface{}{},
	}
	for _, c := range a.Conditions {
		if _, ok := condCtx.Template[c.From]; !ok && c.From != "" {
			// Short circut w/ error here
			// TODO: determine if this is the right thing, I am assume the full rule should fail here
			return ConditionResponse{}, fmt.Errorf("unable to find context value: %v", c.From)
		}
		response, err := c.ProviderSpecificConfig.Evaluate(ctx, log, condCtx)
		if err != nil {
			return ConditionResponse{}, err
		}
		if c.As != "" {
			condCtx.Template[c.As] = lib.ChainTemplate{
				Filepaths: incidentsToFilepaths(response.Incidents),
				Extras:    response.TemplateContext,
			}
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

func (o OrCondition) Evaluate(ctx context.Context, log logr.Logger, condCtx ConditionContext) (ConditionResponse, error) {
	ctx, span := tracing.StartNewSpan(ctx, "or-condition")
	defer span.End()

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
		if _, ok := condCtx.Template[c.From]; !ok && c.From != "" {
			// Short circut w/ error here
			// TODO: determine if this is the right thing, I am assume the full rule should fail here
			return ConditionResponse{}, fmt.Errorf("unable to find context value: %v", c.From)
		}

		response, err := c.ProviderSpecificConfig.Evaluate(ctx, log, condCtx)
		if err != nil {
			return ConditionResponse{}, err
		}

		if c.As != "" {
			condCtx.Template[c.As] = lib.ChainTemplate{
				Filepaths: incidentsToFilepaths(response.Incidents),
				Extras:    response.TemplateContext,
			}
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

func (ce ConditionEntry) Evaluate(ctx context.Context, log logr.Logger, condCtx ConditionContext) (ConditionResponse, error) {
	response, err := ce.ProviderSpecificConfig.Evaluate(ctx, log, condCtx)
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

func incidentsToFilepaths(incident []IncidentContext) []string {
	filepaths := []string{}
	for _, ic := range incident {
		filepaths = append(filepaths, ic.FileURI)
	}
	return filepaths
}

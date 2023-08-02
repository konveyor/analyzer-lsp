package engine

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/tracing"
	"go.lsp.dev/uri"
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
	Tags     map[string]interface{}   `yaml:"tags"`
	Template map[string]ChainTemplate `yaml:"template"`
}

type ConditionEntry struct {
	From                   string
	As                     string
	Ignorable              bool
	Not                    bool
	ProviderSpecificConfig Conditional
}

type IncidentContext struct {
	FileURI      uri.URI                `yaml:"fileURI"`
	Effort       *int                   `yaml:"effort"`
	LineNumber   *int                   `yaml:"lineNumber,omitempty"`
	Variables    map[string]interface{} `yaml:"variables"`
	Links        []konveyor.Link        `yaml:"externalLink"`
	CodeLocation *Location              `yaml:"location,omitempty"`
}

type Location struct {
	StartPosition Position `yaml:"startPosition"`
	EndPosition   Position `yaml:"endPosition"`
}

type Position struct {
	/*Line defined:
	 * Line position in a document (zero-based).
	 * If a line number is greater than the number of lines in a document, it defaults back to the number of lines in the document.
	 * If a line number is negative, it defaults to 0.
	 */
	Line int `yaml:"line"`

	/*Character defined:
	 * Character offset on a line in a document (zero-based). Assuming that the line is
	 * represented as a string, the `character` value represents the gap between the
	 * `character` and `character + 1`.
	 *
	 * If the character value is greater than the line length it defaults back to the
	 * line length.
	 * If a line number is negative, it defaults to 0.
	 */
	Character int `yaml:"character"`
}

type Conditional interface {
	Evaluate(ctx context.Context, log logr.Logger, condCtx ConditionContext) (ConditionResponse, error)
}

type CodeSnip interface {
	GetCodeSnip(uri.URI, Location) (string, error)
}

type RuleSet struct {
	Name        string   `json:"name,omitempty" yaml:"name,omitempty"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	Labels      []string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Tags        []string `json:"tags,omitempty" yaml:"tags,omitempty"`
	Rules       []Rule   `json:"rules,omitempty" yaml:"rules,omitempty"`
}

type Rule struct {
	RuleMeta        `yaml:",inline" json:",inline"`
	Perform         Perform          `yaml:",inline" json:"perform,omitempty"`
	When            Conditional      `yaml:"when,omitempty" json:"when,omitempty"`
	Snipper         CodeSnip         `yaml:"-" json:"-"`
	CustomVariables []CustomVariable `yaml:"customVariables,omitempty" json:"customVariables,omitempty"`
}

type RuleMeta struct {
	RuleID      string             `yaml:"ruleID,omitempty" json:"ruleID,omitempty"`
	Description string             `yaml:"description,omitempty" json:"description,omitempty"`
	Category    *konveyor.Category `yaml:"category,omitempty" json:"category,omitempty"`
	Labels      []string           `yaml:"labels,omitempty" json:"labels,omitempty"`
	Effort      *int               `json:"effort,omitempty"`
}

func (r *RuleMeta) GetLabels() []string {
	return r.Labels
}

// RuleSelector selects rules based on rule metadata
type RuleSelector interface {
	Matches(*RuleMeta) (bool, error)
}

type CustomVariable struct {
	Pattern            *regexp.Regexp `yaml:"pattern"`
	Name               string         `yaml:"name"`
	DefaultValue       string         `yaml:"defaultValue"`
	NameOfCaptureGroup string         `yaml:"nameOfCaptureGroup"`
}

type Perform struct {
	Message Message  `yaml:",inline"`
	Tag     []string `yaml:"tag,omitempty"`
}

type Message struct {
	Text  *string         `yaml:"message,omitempty"`
	Links []konveyor.Link `yaml:"links,omitempty"`
}

func (p *Perform) Validate() error {
	if p.Message.Text == nil && p.Tag == nil {
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
	conditions := sortConditionEntries(a.Conditions)
	for _, c := range conditions {
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
			condCtx.Template[c.As] = ChainTemplate{
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
	conditions := sortConditionEntries(o.Conditions)
	for _, c := range conditions {
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
			condCtx.Template[c.As] = ChainTemplate{
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
		if strings.HasPrefix(string(ic.FileURI), uri.FileScheme) {
			filepaths = append(filepaths, ic.FileURI.Filename())
		} else {
			filepaths = append(filepaths, string(ic.FileURI))
		}
	}
	return filepaths
}

func sortConditionEntries(entries []ConditionEntry) []ConditionEntry {
	sorted := []ConditionEntry{}
	for _, e := range entries {
		// entries without chaining or that begin a chain come first
		if e.From == "" {
			sorted = append(sorted, gatherChain(e, entries)...)
		}
	}

	return sorted
}

func gatherChain(start ConditionEntry, entries []ConditionEntry) []ConditionEntry {
	chain := []ConditionEntry{start}
	for _, d := range entries {
		if start.As == d.From && start.As != "" {
			chain = append(chain, gatherChain(d, entries)...)
		}
	}
	return chain
}

// Chain Templates are used by rules and providers to pass context around during rule execution.
type ChainTemplate struct {
	Filepaths []string               `yaml:"filepaths"`
	Extras    map[string]interface{} `yaml:"extras"`
}

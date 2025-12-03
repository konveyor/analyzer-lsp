package engine

import (
	"bufio"
	"context"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"go.lsp.dev/uri"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"

	"github.com/cbroglie/mustache"
	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine/internal"
	"github.com/konveyor/analyzer-lsp/engine/labels"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/progress"
	"github.com/konveyor/analyzer-lsp/tracing"
)

// RuleEngine is the interface for running analysis rules
type RuleEngine interface {
	// RunRules runs the given rulesets with optional selectors
	RunRules(context context.Context, rules []RuleSet, selectors ...RuleSelector) []konveyor.RuleSet

	// RunRulesWithOptions runs the given rulesets with run-specific options (e.g., progress reporter) and selectors
	RunRulesWithOptions(context context.Context, rules []RuleSet, opts []RunOption, selectors ...RuleSelector) []konveyor.RuleSet

	// RunRulesScoped runs the given rulesets with a scope and optional selectors
	RunRulesScoped(ctx context.Context, ruleSets []RuleSet, scopes Scope, selectors ...RuleSelector) []konveyor.RuleSet

	// RunRulesScopedWithOptions runs the given rulesets with a scope, run-specific options, and selectors
	RunRulesScopedWithOptions(ctx context.Context, ruleSets []RuleSet, scopes Scope, opts []RunOption, selectors ...RuleSelector) []konveyor.RuleSet

	// Stop stops the rule engine and waits for all workers to complete
	Stop()
}

type ruleMessage struct {
	rule             Rule
	ruleSetName      string
	conditionContext ConditionContext
	scope            Scope
	returnChan       chan response
	carrier          propagation.TextMapCarrier
}

type response struct {
	Violation   *konveyor.Violation `yaml:"violation"`
	Err         error               `yaml:"err"`
	Rule        Rule                `yaml:"rule"`
	RuleSetName string
}

type ruleEngine struct {
	// Buffered channel where Rule Processors are watching
	ruleProcessing chan ruleMessage
	cancelFunc     context.CancelFunc
	logger         logr.Logger

	wg *sync.WaitGroup

	incidentLimit    int
	codeSnipLimit    int
	contextLines     int
	incidentSelector string
	locationPrefixes []string
	encoding         string
}

type Option func(engine *ruleEngine)

// RunOption configures options for a specific RunRules invocation
type RunOption func(*runConfig)

// runConfig holds configuration for a specific run
type runConfig struct {
	progressReporter progress.ProgressReporter
}

// WithProgressReporter sets the progress reporter for this run
func WithProgressReporter(reporter progress.ProgressReporter) RunOption {
	return func(cfg *runConfig) {
		cfg.progressReporter = reporter
	}
}

func WithIncidentLimit(i int) Option {
	return func(engine *ruleEngine) {
		engine.incidentLimit = i
	}
}

func WithContextLines(i int) Option {
	return func(engine *ruleEngine) {
		engine.contextLines = i
	}
}

func WithCodeSnipLimit(i int) Option {
	return func(engine *ruleEngine) {
		engine.codeSnipLimit = i
	}
}

func WithIncidentSelector(selector string) Option {
	return func(engine *ruleEngine) {
		engine.incidentSelector = selector
	}
}

func WithLocationPrefixes(location []string) Option {
	return func(engine *ruleEngine) {
		engine.locationPrefixes = location
	}
}

func WithEncoding(encoding string) Option {
	return func(engine *ruleEngine) {
		engine.encoding = encoding
	}
}

func CreateRuleEngine(ctx context.Context, workers int, log logr.Logger, options ...Option) RuleEngine {
	// Only allow for 10 rules to be waiting in the buffer at once.
	// Adding more workers will increase the number of rules running at once.
	ruleProcessor := make(chan ruleMessage, 10)

	ctx, cancelFunc := context.WithCancel(ctx)
	wg := &sync.WaitGroup{}

	r := &ruleEngine{
		ruleProcessing: ruleProcessor,
		cancelFunc:     cancelFunc,
		logger:         log,
		wg:             wg,
	}
	for _, o := range options {
		o(r)
	}
	for i := range workers {
		logger := log.WithValues("worker", i)
		wg.Add(1)
		go r.processRuleWorker(ctx, ruleProcessor, logger, wg)
	}

	return r
}

func (r *ruleEngine) Stop() {
	r.cancelFunc()
	r.logger.V(5).Info("rule engine stopping")
	r.wg.Wait()
}

// reportProgress sends a progress event to the given reporter
func reportProgress(reporter progress.ProgressReporter, event progress.ProgressEvent) {
	if reporter != nil {
		reporter.Report(event)
	}
}

func (r *ruleEngine) processRuleWorker(ctx context.Context, ruleMessages chan ruleMessage, logger logr.Logger, wg *sync.WaitGroup) {
	prop := otel.GetTextMapPropagator()
	for {
		select {
		case m := <-ruleMessages:
			logger.V(5).Info("taking rule", "ruleset", m.ruleSetName, "rule", m.rule.RuleID)
			newLogger := logger.WithValues("ruleID", m.rule.RuleID)
			//We createa new rule context for a every rule run, here we need to apply the scope
			m.conditionContext.Template = make(map[string]ChainTemplate)
			if m.scope != nil {
				m.scope.AddToContext(&m.conditionContext)
			}
			logger.Info("Adding Carrier span info to context")
			ctx = prop.Extract(ctx, m.carrier)

			conditionResponse, err := processRule(ctx, m.rule, m.conditionContext, newLogger)
			newLogger.V(5).Info("finished rule", "found", len(conditionResponse.Incidents), "matched", conditionResponse.Matched, "error", err)
			response := response{
				Err:         err,
				Rule:        m.rule,
				RuleSetName: m.ruleSetName,
			}
			if conditionResponse.Matched && len(conditionResponse.Incidents) > 0 {
				violation, err := r.createViolation(ctx, conditionResponse, m.rule, m.scope)
				if err != nil {
					response.Err = err
				} else if len(violation.Incidents) == 0 {
					newLogger.V(5).Info("rule was evaluated and incidents were filtered out to make it unmatched")
				} else {
					response.Violation = &violation
				}
			}
			m.returnChan <- response
		case <-ctx.Done():
			logger.V(5).Info("stopping rule worker")
			wg.Done()
			return
		}
	}
}

func (r *ruleEngine) createRuleSet(ruleSet RuleSet) *konveyor.RuleSet {
	rs := &konveyor.RuleSet{
		Name:        ruleSet.Name,
		Description: ruleSet.Description,
		Tags:        []string{},
		Violations:  map[string]konveyor.Violation{},
		Insights:    map[string]konveyor.Violation{},
		Errors:      map[string]string{},
		Unmatched:   []string{},
		Skipped:     []string{},
	}
	return rs
}

// This will run tagging rules first, synchronously, generating tags to pass on further as context to other rules
// then runs remaining rules async, fanning them out, fanning them in, finally generating the results. will block until completed.
func (r *ruleEngine) RunRules(ctx context.Context, ruleSets []RuleSet, selectors ...RuleSelector) []konveyor.RuleSet {
	return r.RunRulesWithOptions(ctx, ruleSets, nil, selectors...)
}

func (r *ruleEngine) RunRulesWithOptions(ctx context.Context, ruleSets []RuleSet, opts []RunOption, selectors ...RuleSelector) []konveyor.RuleSet {
	return r.RunRulesScopedWithOptions(ctx, ruleSets, nil, opts, selectors...)
}

func (r *ruleEngine) RunRulesScoped(ctx context.Context, ruleSets []RuleSet, scopes Scope, selectors ...RuleSelector) []konveyor.RuleSet {
	return r.RunRulesScopedWithOptions(ctx, ruleSets, scopes, nil, selectors...)
}

func (r *ruleEngine) RunRulesScopedWithOptions(ctx context.Context, ruleSets []RuleSet, scopes Scope, opts []RunOption, selectors ...RuleSelector) []konveyor.RuleSet {
	// Build run configuration
	cfg := &runConfig{
		progressReporter: progress.NewNoopReporter(), // Default to no-op
	}
	for _, opt := range opts {
		opt(cfg)
	}
	// determine if we should run

	conditionContext := ConditionContext{
		Tags:     make(map[string]any),
		Template: make(map[string]ChainTemplate),
	}
	if scopes != nil {
		r.logger.Info("using scopes", "scope", scopes.Name())
		err := scopes.AddToContext(&conditionContext)
		if err != nil {
			r.logger.Error(err, "unable to apply scopes to ruleContext")
			// Call this, even though it is not used, to make sure that
			// we don't leak anything.
			return []konveyor.RuleSet{}
		}
		r.logger.Info("added scopes to condition context", "scopes", scopes, "conditionContext", conditionContext)
	}
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	r.logger.Info("inject span info", "carrier", carrier)
	ctx, cancelFunc := context.WithCancel(ctx)

	taggingRules, otherRules, mapRuleSets := r.filterRules(ruleSets, selectors...)

	ruleContext := r.runTaggingRules(ctx, taggingRules, mapRuleSets, conditionContext, scopes)

	// Report total number of rules to process
	totalRules := len(otherRules)
	reportProgress(cfg.progressReporter, progress.ProgressEvent{
		Stage:   progress.StageRuleExecution,
		Current: 0,
		Total:   totalRules,
		Message: fmt.Sprintf("Starting rule execution: %d rules to process", totalRules),
	})

	// Need a better name for this thing
	ret := make(chan response)

	var matchedRules int32
	var unmatchedRules int32
	var failedRules int32

	wg := &sync.WaitGroup{}
	// Handle returns
	go func() {
		for {
			select {
			case response := <-ret:
				func() {
					log := r.logger.WithValues("ruleID", response.Rule.RuleID)
					log.Info("rule returned", "ruleID", response.Rule.RuleID)
					defer wg.Done()
					if response.Err != nil {
						atomic.AddInt32(&failedRules, 1)
						log.Error(response.Err, "failed to evaluate rule")

						if rs, ok := mapRuleSets[response.RuleSetName]; ok {
							rs.Errors[response.Rule.RuleID] = response.Err.Error()
						}
						// response.Violation will be nil when the condition response is unmatched or there are zero incidents
					} else if response.Violation != nil {
						atomic.AddInt32(&matchedRules, 1)
						rs, ok := mapRuleSets[response.RuleSetName]
						if !ok {
							log.Info("this should never happen that we don't find the ruleset")
							return
						}
						// when a rule has 0 effort, we should create an insight instead
						if response.Rule.Effort == nil || *response.Rule.Effort == 0 {
							rs.Insights[response.Rule.RuleID] = *response.Violation
						} else {
							rs.Violations[response.Rule.RuleID] = *response.Violation
						}
					} else {
						atomic.AddInt32(&unmatchedRules, 1)
						// Log that rule did not pass
						r.logger.V(5).Info("rule was evaluated, and we did not find a violation", "ruleID", response.Rule.RuleID)

						if rs, ok := mapRuleSets[response.RuleSetName]; ok {
							rs.Unmatched = append(rs.Unmatched, response.Rule.RuleID)
						}
					}
					r.logger.V(5).Info("rule response received", "total", len(otherRules), "failed", failedRules, "matched", matchedRules, "unmatched", unmatchedRules)

					// Report progress after each rule completes
					completed := int(matchedRules + unmatchedRules + failedRules)
					reportProgress(cfg.progressReporter, progress.ProgressEvent{
						Stage:   progress.StageRuleExecution,
						Current: completed,
						Total:   totalRules,
						Message: response.Rule.RuleID,
					})

				}()
			case <-ctx.Done():
				// At this point we should just return the function, we may want to close the wait group too.
				return
			}
		}
	}()

	for _, rule := range otherRules {
		newContext := ruleContext.Copy()
		newContext.RuleID = rule.rule.RuleID
		wg.Add(1)
		rule.returnChan = ret
		rule.conditionContext = newContext
		rule.scope = scopes
		rule.carrier = carrier
		r.ruleProcessing <- rule
	}
	r.logger.V(5).Info("All rules added buffer, waiting for engine to complete", "size", len(otherRules))

	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	// Wait for all the rules to process
	select {
	case <-done:
		r.logger.V(2).Info("done processing all the rules")
		// Report completion
		reportProgress(cfg.progressReporter, progress.ProgressEvent{
			Stage:   progress.StageComplete,
			Current: totalRules,
			Total:   totalRules,
			Message: "Rule execution complete",
		})
	case <-ctx.Done():
		r.logger.V(1).Info("processing of rules was canceled")
	}
	responses := []konveyor.RuleSet{}
	for _, ruleSet := range mapRuleSets {
		if ruleSet != nil {
			responses = append(responses, *ruleSet)
		}
	}
	// Cannel running go-routine
	cancelFunc()
	return responses
}

// filterRules splits rules into tagging and other rules
func (r *ruleEngine) filterRules(ruleSets []RuleSet, selectors ...RuleSelector) ([]ruleMessage, []ruleMessage, map[string]*konveyor.RuleSet) {
	// filter rules that generate tags, they run first
	taggingRules := []ruleMessage{}
	mapRuleSets := map[string]*konveyor.RuleSet{}
	// all rules except meta
	otherRules := []ruleMessage{}
	for _, ruleSet := range ruleSets {
		mapRuleSets[ruleSet.Name] = r.createRuleSet(ruleSet)
		for _, rule := range ruleSet.Rules {
			// labels on ruleset apply to all rules in it
			rule.Labels = append(rule.Labels, ruleSet.Labels...)
			// skip rule when doesn't match any selector
			if !matchesAllSelectors(rule.RuleMeta, selectors...) {
				mapRuleSets[ruleSet.Name].Skipped = append(mapRuleSets[ruleSet.Name].Skipped, rule.RuleID)
				r.logger.V(5).Info("one or more selectors did not match for rule, skipping", "ruleID", rule.RuleID)
				continue
			}

			if rule.Perform.Tag == nil {
				otherRules = append(otherRules, ruleMessage{
					rule:        rule,
					ruleSetName: ruleSet.Name,
				})
			} else {
				taggingRules = append(taggingRules, ruleMessage{
					rule:        rule,
					ruleSetName: ruleSet.Name,
				})
				// if both message and tag are set, split message part into a new rule if effort is non-zero
				// if effort is zero, we do not want to create a violation but only tag and an insight
				if rule.Perform.Message.Text != nil && rule.Effort != nil && *rule.Effort != 0 {
					// because split rules will share ruleID, we need to add the tags to the labels here
					for _, tag := range rule.Perform.Tag {
						rule.Labels = append(rule.Labels, fmt.Sprintf("tag=%s", tag))
					}
					rule.Perform.Tag = nil
					otherRules = append(otherRules, ruleMessage{
						rule:        rule,
						ruleSetName: ruleSet.Name,
					})
				}
			}
		}
	}
	return taggingRules, otherRules, mapRuleSets
}

// runTaggingRules filters and runs info rules synchronously
// returns list of non-info rules, a context to pass to them
func (r *ruleEngine) runTaggingRules(ctx context.Context, infoRules []ruleMessage, mapRuleSets map[string]*konveyor.RuleSet, context ConditionContext, scope Scope) ConditionContext {
	//  move all rules that have HasTags to the end of the list as they depend on other tagging rules
	sort.Slice(infoRules, func(i int, j int) bool {
		return !infoRules[i].rule.UsesHasTags && infoRules[j].rule.UsesHasTags
	})
	// track unique tags per ruleset
	rulesetTagsCache := map[string]map[string]bool{}
	for _, ruleMessage := range infoRules {
		rule := ruleMessage.rule
		ruleCtx := context.Copy()
		ruleCtx.RuleID = rule.RuleID
		response, err := processRule(ctx, rule, ruleCtx, r.logger)
		if err != nil {
			r.logger.Error(err, "failed to evaluate rule", "ruleID", rule.RuleID)
			if rs, ok := mapRuleSets[ruleMessage.ruleSetName]; ok {
				rs.Errors[rule.RuleID] = err.Error()
			}
		} else if response.Matched && len(response.Incidents) > 0 {
			r.logger.V(5).Info("info rule was matched", "ruleID", rule.RuleID)
			// create an insight for this tag
			violation, err := r.createViolation(ctx, response, rule, scope)
			if err != nil {
				r.logger.Error(err, "unable to create violation from response", "ruleID", rule.RuleID)
			}
			if len(violation.Incidents) == 0 {
				r.logger.V(5).Info("rule was evaluated and incidents were filtered out to make it unmatched", "ruleID", rule.RuleID)
				continue
			}
			tags := map[string]bool{}
			for _, tagString := range rule.Perform.Tag {
				if strings.Contains(tagString, "{{") && strings.Contains(tagString, "}}") {
					for _, incident := range response.Incidents {
						// If this is the case then we neeed to use the reponse variables to get the tag
						variables := make(map[string]any)
						maps.Copy(variables, incident.Variables)
						if incident.LineNumber != nil {
							variables["lineNumber"] = *incident.LineNumber
						}
						templateString, err := r.createPerformString(tagString, variables)
						if err != nil {
							r.logger.Error(err, "unable to create tag string", "ruleID", rule.RuleID)
							continue
						}
						tags[templateString] = true
					}
				} else {
					tags[tagString] = true
				}
				for t := range tags {
					tags, err := parseTagsFromPerformString(t)
					if err != nil {
						r.logger.Error(err, "unable to create tags", "ruleID", rule.RuleID)
						continue
					}
					for _, tag := range tags {
						context.Tags[tag] = true
					}
				}
			}
			rs, ok := mapRuleSets[ruleMessage.ruleSetName]
			if !ok {
				r.logger.Info("this should never happen that we don't find the ruleset")
			} else {
				if _, ok := rulesetTagsCache[rs.Name]; !ok {
					rulesetTagsCache[rs.Name] = make(map[string]bool)
				}
				for tag := range tags {
					if _, ok := rulesetTagsCache[rs.Name][tag]; !ok {
						rulesetTagsCache[rs.Name][tag] = true
						rs.Tags = append(rs.Tags, tag)
					}
				}
				mapRuleSets[ruleMessage.ruleSetName] = rs
			}
			if rs, ok := mapRuleSets[ruleMessage.ruleSetName]; ok {
				violation.Category = nil
				// Add all tags to violation labels
				for tag := range tags {
					violation.Labels = append(violation.Labels, fmt.Sprintf("tag=%s", tag))
				}
				if violation.Effort != nil && *violation.Effort > 0 {
					// we need to tie these incidents back to tags that created them
					// don't create insight for effort > 0
					rs.Violations[rule.RuleID] = violation
				} else {
					rs.Insights[rule.RuleID] = violation
				}
			}
		} else {
			r.logger.Info("info rule not matched", "rule", rule.RuleID)
			if rs, ok := mapRuleSets[ruleMessage.ruleSetName]; ok {
				rs.Unmatched = append(rs.Unmatched, rule.RuleID)
			}
		}
	}
	return context
}

func parseTagsFromPerformString(tagString string) ([]string, error) {
	tags := []string{}
	pattern := regexp.MustCompile(`^(?:[\w- \(\)]+=){0,1}([\w- \(\)]+(?:, *[\w- \(\),]+)*),?$`)
	if !pattern.MatchString(tagString) {
		return nil, fmt.Errorf("unexpected tag string %s", tagString)
	}
	for _, groups := range pattern.FindAllStringSubmatch(tagString, -1) {
		for _, tag := range strings.Split(groups[1], ",") {
			if tag != "" {
				tags = append(tags, strings.Trim(tag, " "))
			}
		}
	}
	return tags, nil
}

func processRule(ctx context.Context, rule Rule, ruleCtx ConditionContext, log logr.Logger) (ConditionResponse, error) {
	log.WithName("process-rule").Info("processing rule", "ruleID", rule.RuleID)
	ctx, span := tracing.StartNewSpan(
		ctx, "process-rule", attribute.Key("rule").String(rule.RuleID))
	defer span.End()
	// Here is what a worker should run when getting a rule.
	// For now, lets not fan out the running of conditions.
	return rule.When.Evaluate(ctx, log, ruleCtx)

}

func (r *ruleEngine) getRelativePathForViolation(fileURI uri.URI) (uri.URI, error) {
	var sourceLocation string
	if fileURI != "" {
		u, err := url.ParseRequestURI(string(fileURI))
		if err != nil || u.Scheme != uri.FileScheme {
			return fileURI, nil
		}
		file := fileURI.Filename()
		// get the correct source
		for _, locationPrefix := range r.locationPrefixes {
			if strings.Contains(file, locationPrefix) {
				sourceLocation = locationPrefix
				break
			}
		}
		absPath, err := filepath.Abs(sourceLocation)
		if err != nil {
			return fileURI, nil
		}
		// given a relative path for source
		if absPath != sourceLocation {
			relPath := filepath.Join(sourceLocation, strings.TrimPrefix(file, absPath))
			newURI := fmt.Sprintf("file:///%s", filepath.Join(strings.TrimPrefix(relPath, "/")))
			return uri.URI(newURI), nil
		}
	}
	return fileURI, nil
}

func (r *ruleEngine) createViolation(ctx context.Context, conditionResponse ConditionResponse, rule Rule, scope Scope) (konveyor.Violation, error) {
	incidents := []konveyor.Incident{}
	fileCodeSnipCount := map[string]int{}
	incidentsSet := map[string]struct{}{} // Set of incidents
	var incidentSelector *labels.LabelSelector[internal.VariableLabelSelector]
	var err error
	if r.incidentSelector != "" {
		incidentSelector, err = labels.NewLabelSelector[internal.VariableLabelSelector](r.incidentSelector, internal.MatchVariables)
		if err != nil {
			return konveyor.Violation{}, err
		}
	}
	for _, m := range conditionResponse.Incidents {
		// Exit loop, we don't care about any incidents past the filter.
		if r.incidentLimit != 0 && len(incidents) == r.incidentLimit {
			break
		}
		// If we should remove the incident because the provider didn't filter it
		// and the user asked for a certain scope of incidents.
		if scope != nil && scope.FilterResponse(m) {
			continue
		}
		trimmedUri, err := r.getRelativePathForViolation(m.FileURI)
		if err != nil {
			return konveyor.Violation{}, err
		}

		for val := range m.Variables {
			if val == "file" {
				m.Variables["file"] = trimmedUri
			}
		}

		incident := konveyor.Incident{
			URI:        trimmedUri,
			LineNumber: m.LineNumber,
			// This allows us to change m.Variables and it will be set
			// because it is a pointer.
			Variables: m.Variables,
		}
		if m.LineNumber != nil {
			lineNumber := *m.LineNumber
			incident.LineNumber = &lineNumber
		}
		// Some violations may not have a location in code.
		limitSnip := (r.codeSnipLimit != 0 && fileCodeSnipCount[string(m.FileURI)] == r.codeSnipLimit)
		if !limitSnip {
			codeSnip, err := r.getCodeLocation(ctx, m, rule)
			if err != nil {
				r.logger.V(6).Error(err, "unable to get code location")
			} else if codeSnip == "" {
				r.logger.V(3).Info("no code snippet returned", "rule", rule)
			} else {
				incident.CodeSnip = codeSnip
			}
			fileCodeSnipCount[string(m.FileURI)] += 1
		}

		if len(rule.CustomVariables) > 0 {
			var originalCodeSnip string
			re := regexp.MustCompile(`^(\s*[0-9]+  )?(.*)`)
			scanner := bufio.NewScanner(strings.NewReader(incident.CodeSnip))
			for scanner.Scan() {
				if incident.LineNumber != nil && strings.HasPrefix(strings.TrimSpace(scanner.Text()), fmt.Sprintf("%v", *incident.LineNumber)) {
					originalCodeSnip = strings.TrimSpace(re.ReplaceAllString(scanner.Text(), "$2"))
					r.logger.V(5).Info("found originalCodeSnip", "lineNuber", incident.LineNumber, "original", originalCodeSnip)
					break
				}
			}

			for _, cv := range rule.CustomVariables {
				match := cv.Pattern.FindStringSubmatch(originalCodeSnip)
				if cv.NameOfCaptureGroup != "" && cv.Pattern.SubexpIndex(cv.NameOfCaptureGroup) >= 0 &&
					cv.Pattern.SubexpIndex(cv.NameOfCaptureGroup) < len(match) {

					m.Variables[cv.Name] = strings.TrimSpace(match[cv.Pattern.SubexpIndex(cv.NameOfCaptureGroup)])
					continue

				} else {
					switch len(match) {
					case 0:
						m.Variables[cv.Name] = cv.DefaultValue
						continue
					case 1:
						m.Variables[cv.Name] = strings.TrimSpace(match[0])
						continue
					case 2:
						m.Variables[cv.Name] = strings.TrimSpace(match[1])
					}
				}
			}
		}

		if rule.Perform.Message.Text != nil {
			variables := make(map[string]any)
			maps.Copy(variables, m.Variables)
			if m.LineNumber != nil {
				variables["lineNumber"] = *m.LineNumber
			}
			templateString, err := r.createPerformString(*rule.Perform.Message.Text, variables)
			if err != nil {
				r.logger.Error(err, "unable to create template string")
			}
			incident.Message = templateString
		}

		incidentLineNumber := -1
		if incident.LineNumber != nil {
			incidentLineNumber = *incident.LineNumber
		}

		// Deterime if we can filter out based on incident selector.
		if r.incidentSelector != "" {
			v := internal.VariableLabelSelector(incident.Variables)
			b, err := incidentSelector.Matches(v)
			if err != nil {
				r.logger.Error(err, "unable to determine if incident should filter out, defautl to adding", "ruleID", rule.RuleID)
			}
			if !b {
				r.logger.Info("filtering out incident based on incident selector", "ruleID", rule.RuleID)
				continue
			}
		}

		incidentString := fmt.Sprintf("%s-%s-%d", incident.URI, incident.Message, incidentLineNumber) // Formating a unique string for an incident

		// Adding it to list  and set if no duplicates found
		if _, isDuplicate := incidentsSet[incidentString]; !isDuplicate {
			incidents = append(incidents, incident)
			incidentsSet[incidentString] = struct{}{}
		}

	}

	rule.Labels = deduplicateLabels(rule.Labels)

	return konveyor.Violation{
		Description: rule.Description,
		Labels:      rule.Labels,
		Category:    rule.Category,
		Incidents:   incidents,
		Extras:      []byte{},
		Effort:      rule.Effort,
		Links:       rule.Perform.Message.Links,
	}, nil
}

func (r *ruleEngine) getCodeLocation(_ context.Context, m IncidentContext, rule Rule) (codeSnip string, err error) {
	if m.CodeLocation == nil {
		r.logger.V(6).Info("unable to get the code snip", "URI", m.FileURI)
		return "", nil
	}

	// We need to move this up, because the code only lives in the
	// provider's
	if rule.Snipper != nil {
		return rule.Snipper.GetCodeSnip(m.FileURI, *m.CodeLocation)
	}

	if strings.HasPrefix(string(m.FileURI), uri.FileScheme) {
		//Find the file, open it in a buffer.
		var content []byte
		var err error
		if r.encoding != "" {
			content, err = OpenFileWithEncoding(m.FileURI.Filename(), r.encoding)
			if err != nil {
				r.logger.V(5).Error(err, "failed to convert file encoding, using original content", "file", m.FileURI.Filename())
				content, err = os.ReadFile(m.FileURI.Filename())
				if err != nil {
					return "", err
				}
			}
		} else {
			content, err = os.ReadFile(m.FileURI.Filename())
			if err != nil {
				return "", err
			}
		}

		scanner := bufio.NewScanner(strings.NewReader(string(content)))
		lineNumber := 0
		codeSnip := ""
		paddingSize := len(strconv.Itoa(m.CodeLocation.EndPosition.Line + r.contextLines))
		for scanner.Scan() {
			if (lineNumber - r.contextLines) == m.CodeLocation.EndPosition.Line {
				codeSnip = codeSnip + fmt.Sprintf("%*d  %v", paddingSize, lineNumber+1, scanner.Text())
				break
			}
			if (lineNumber + r.contextLines) >= m.CodeLocation.StartPosition.Line {
				codeSnip = codeSnip + fmt.Sprintf("%*d  %v\n", paddingSize, lineNumber+1, scanner.Text())
			}
			lineNumber += 1
		}
		return codeSnip, nil
	}

	// if it is not a file ask the provider
	return "", nil
}

func (r *ruleEngine) createPerformString(messageTemplate string, ctx map[string]any) (string, error) {
	return mustache.Render(messageTemplate, ctx)
}

// matchesAllSelectors returns false when any one of the selectors does not match
// selectors can be of different types e.g. label-selector, or category-selector
// when multiple selectors are present, we want all of them to match to filter-in the rule.
func matchesAllSelectors(m RuleMeta, selectors ...RuleSelector) bool {
	for _, s := range selectors {
		got, err := s.Matches(&m)
		if err != nil || !got {
			return false
		}
	}
	return true
}

func deduplicateLabels(labels []string) []string {
	present := map[string]bool{}
	uniquelabels := []string{}

	for _, label := range labels {
		if !present[label] {
			present[label] = true
			uniquelabels = append(uniquelabels, label)
		}
	}

	return uniquelabels

}

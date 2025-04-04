package engine

import (
	"bufio"
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
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
	"github.com/konveyor/analyzer-lsp/tracing"
)

type RuleEngine interface {
	RunRules(context context.Context, rules []RuleSet, selectors ...RuleSelector) []konveyor.RuleSet
	RunRulesScoped(ctx context.Context, ruleSets []RuleSet, scopes Scope, selectors ...RuleSelector) []konveyor.RuleSet
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

	incidentLimit    int
	codeSnipLimit    int
	contextLines     int
	incidentSelector string
	locationPrefixes []string
}

type Option func(engine *ruleEngine)

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

func CreateRuleEngine(ctx context.Context, workers int, log logr.Logger, options ...Option) RuleEngine {
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

	r := &ruleEngine{
		ruleProcessing: ruleProcessor,
		cancelFunc:     cancelFunc,
		logger:         log,
		wg:             wg,
	}
	for _, o := range options {
		o(r)
	}
	return r
}

func (r *ruleEngine) Stop() {
	r.cancelFunc()
	r.logger.V(5).Info("rule engine stopping")
	r.wg.Wait()
}

func processRuleWorker(ctx context.Context, ruleMessages chan ruleMessage, logger logr.Logger, wg *sync.WaitGroup) {
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

			bo, err := processRule(ctx, m.rule, m.conditionContext, newLogger)
			logger.V(5).Info("finished rule", "found", len(bo.Incidents), "error", err, "rule", m.rule.RuleID)
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
	return r.RunRulesScoped(ctx, ruleSets, nil, selectors...)
}

func (r *ruleEngine) RunRulesScoped(ctx context.Context, ruleSets []RuleSet, scopes Scope, selectors ...RuleSelector) []konveyor.RuleSet {
	conditionContext := ConditionContext{
		Tags:     make(map[string]interface{}),
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

	taggingRules, dependentTaggingRules, violationRules, rulesetsMap := r.filterRules(ruleSets, selectors...)

	totalRules := len(taggingRules) + len(dependentTaggingRules) + len(violationRules)
	r.logger.V(5).Info("initiating rule processing",
		"totalRules", totalRules, "taggingRules", totalRules-len(violationRules), "violationRules", len(violationRules))

	// Cannel running go-routine
	defer cancelFunc()
	tags, err := r.runRulesBatch(
		ctx,
		"initialTaggingRules",
		int32(totalRules),
		taggingRules,
		scopes,
		conditionContext,
		carrier,
		rulesetsMap,
	)
	if err != nil {
		r.logger.Error(err, "context canceled when processing initial tagging rules")
		return flattenRulesets(rulesetsMap)
	}
	r.logger.V(5).Info("processed initial tagging rules", "totalRules", totalRules, "totalProcessed", len(taggingRules))
	mergeTagsIntoMap(conditionContext.Tags, tags)
	tags, err = r.runRulesBatch(
		ctx,
		"dependentTaggingRules",
		int32(totalRules),
		dependentTaggingRules,
		scopes,
		conditionContext,
		carrier,
		rulesetsMap,
	)
	if err != nil {
		r.logger.Error(err, "context canceled when processing dependent tagging rules")
		return flattenRulesets(rulesetsMap)
	}
	r.logger.V(5).Info("processed dependent tagging rules", "totalRules", totalRules, "totalProcessed", len(dependentTaggingRules)+len(taggingRules))
	mergeTagsIntoMap(conditionContext.Tags, tags)
	_, err = r.runRulesBatch(
		ctx,
		"violationRules",
		int32(totalRules),
		violationRules,
		scopes,
		conditionContext,
		carrier,
		rulesetsMap,
	)
	if err != nil {
		r.logger.Error(err, "context canceled when processing violation rules")
	}
	r.logger.V(5).Info("processed violation rules", "totalRules", totalRules, "totalProcessed", totalRules)
	return flattenRulesets(rulesetsMap)
}

// runRulesBatch runs a batch of rules violation or otherwise
// returns tags if there were any tagging rules in the batch
// returns error if context was canceled during rule execution
func (r *ruleEngine) runRulesBatch(
	ctx context.Context,
	batchName string,
	totalRules int32,
	rules []ruleMessage,
	scopes Scope,
	ruleContext ConditionContext,
	carrier propagation.MapCarrier,
	rulesetsMap map[string]*konveyor.RuleSet,
) (map[string]interface{}, error) {
	ruleResponseChan := make(chan response)

	var matchedRules int32
	var unmatchedRules int32
	var failedRules int32

	// tagging rules create new tag
	allTags := map[string]interface{}{}
	tagsChan := make(chan string)

	wg := &sync.WaitGroup{}

	// handle tags generated during handling of returns
	go func() {
		for {
			select {
			case tag := <-tagsChan:
				allTags[tag] = true
				wg.Done()
			case <-ctx.Done():
				return
			}
		}
	}()

	// Handle returns
	go func() {
		for {
			select {
			case response := <-ruleResponseChan:
				func() {
					r.logger.Info("rule returned", "ruleID", response.Rule.RuleID)
					defer wg.Done()
					if response.Err != nil {
						atomic.AddInt32(&failedRules, 1)
						r.logger.Error(response.Err, "failed to evaluate rule", "ruleID", response.Rule.RuleID)

						if rs, ok := rulesetsMap[response.RuleSetName]; ok {
							rs.Errors[response.Rule.RuleID] = response.Err.Error()
						}
					} else if response.ConditionResponse.Matched && len(response.ConditionResponse.Incidents) > 0 {
						r.logger.V(5).Info("rule matched", "ruleID", response.Rule.RuleID, "batch", batchName)
						violation, err := r.createViolation(ctx, response.ConditionResponse, response.Rule, scopes)
						if err != nil {
							r.logger.Error(err, "unable to create violation from response", "ruleID", response.Rule.RuleID)
						}
						if len(violation.Incidents) == 0 {
							r.logger.V(5).Info("rule was evaluated but all incidents were filtered out", "ruleID", response.Rule.RuleID)
							atomic.AddInt32(&unmatchedRules, 1)
							if rs, ok := rulesetsMap[response.RuleSetName]; ok {
								rs.Unmatched = append(rs.Unmatched, response.Rule.RuleID)
							}
							return
						}
						atomic.AddInt32(&matchedRules, 1)
						rs, ok := rulesetsMap[response.RuleSetName]
						if !ok {
							r.logger.Info("this should never happen that we don't find the ruleset")
							return
						}
						if response.Rule.Perform.Tag != nil {
							// handle a tagging rule (info rule)
							// we create 'insight' for these and add tagsMap
							// insight is a violation with zero effort
							violation.Effort = nil
							violation.Category = nil
							tagsMap := map[string]bool{}
							for _, tagString := range response.Rule.Perform.Tag {
								if strings.Contains(tagString, "{{") && strings.Contains(tagString, "}}") {
									for _, incident := range response.ConditionResponse.Incidents {
										// If this is the case then we neeed to use the reponse variables to get the tag
										variables := make(map[string]interface{})
										for key, value := range incident.Variables {
											variables[key] = value
										}
										if incident.LineNumber != nil {
											variables["lineNumber"] = *incident.LineNumber
										}
										templateString, err := r.createPerformString(tagString, variables)
										if err != nil {
											r.logger.Error(err, "unable to create tag string", "ruleID", response.Rule.RuleID)
											continue
										}
										tagsMap[templateString] = true
									}
								} else {
									tagsMap[tagString] = true
								}
								for t := range tagsMap {
									rs.Tags = append(rs.Tags, t)
									tags, err := parseTagsFromPerformString(t)
									if err != nil {
										r.logger.Error(err, "unable to create tags", "ruleID", response.Rule.RuleID)
										continue
									}
									for _, tag := range tags {
										// we have to make sure we wait
										// until all tags are processed
										wg.Add(1)
										tagsChan <- tag
									}
								}
							}
							// we need to tie these incidents back to tags that created them
							for tag := range tagsMap {
								violation.Labels = append(violation.Labels, fmt.Sprintf("tag=%s", tag))
							}
						}
						// when a rule has 0 effort, we should create an insight instead
						if response.Rule.Effort == nil || *response.Rule.Effort == 0 {
							rs.Insights[response.Rule.RuleID] = violation
						} else {
							rs.Violations[response.Rule.RuleID] = violation
						}
					} else {
						atomic.AddInt32(&unmatchedRules, 1)
						r.logger.V(5).Info("rule was evaluated, and we did not find a violation", "ruleID", response.Rule.RuleID)
						if rs, ok := rulesetsMap[response.RuleSetName]; ok {
							rs.Unmatched = append(rs.Unmatched, response.Rule.RuleID)
						}
					}
					r.logger.V(5).Info("rule response received", "batch", batchName, "total", totalRules, "failed", failedRules, "matched", matchedRules, "unmatched", unmatchedRules)
				}()
			case <-ctx.Done():
				return
			}
		}
	}()

	for _, rule := range rules {
		newContext := ruleContext.Copy()
		newContext.RuleID = rule.rule.RuleID
		wg.Add(1)
		rule.returnChan = ruleResponseChan
		rule.conditionContext = newContext
		rule.scope = scopes
		rule.carrier = carrier
		r.ruleProcessing <- rule
	}
	r.logger.V(5).Info("Rules in the current batch added to buffer, waiting for engine to complete", "size", len(rules))

	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	// Wait for all the rules in this batch to process
	select {
	case <-done:
		r.logger.V(2).Info("done processing rules in the current batch", "batch", batchName)
	case <-ctx.Done():
		r.logger.V(1).Info("processing of rules was canceled", "batch", batchName)
		return allTags, ctx.Err()
	}
	return allTags, nil
}

// filterRules splits rules into three groups
// first, tagging rules that do not depend on pther tagging rules
// second, tagging rules that depend on other tagging rules
// third, violation rules that may depend on tagging rules
func (r *ruleEngine) filterRules(ruleSets []RuleSet, selectors ...RuleSelector) ([]ruleMessage, []ruleMessage, []ruleMessage, map[string]*konveyor.RuleSet) {
	// filter rules that generate tags, they run first
	initialTaggingRules := []ruleMessage{}
	// dependent tagging rules depend on above rules
	dependentTaggingRules := []ruleMessage{}
	// all rules except meta
	otherRules := []ruleMessage{}

	mapRuleSets := map[string]*konveyor.RuleSet{}
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
				if rule.UsesHasTags {
					dependentTaggingRules = append(dependentTaggingRules, ruleMessage{
						rule:        rule,
						ruleSetName: ruleSet.Name,
					})
				} else {
					initialTaggingRules = append(initialTaggingRules, ruleMessage{
						rule:        rule,
						ruleSetName: ruleSet.Name,
					})
				}
				// if both message and tag are set, split message part into a new rule if effort is non-zero
				// if effort is zero, we do not want to create a violation but only tag and an insight
				if rule.Perform.Message.Text != nil && rule.Effort != nil && *rule.Effort != 0 {
					rule.Perform.Tag = nil
					otherRules = append(
						otherRules,
						ruleMessage{
							rule:        rule,
							ruleSetName: ruleSet.Name,
						},
					)
				}
			}
		}
	}
	return initialTaggingRules, dependentTaggingRules, otherRules, mapRuleSets
}

func flattenRulesets(rulesetsMap map[string]*konveyor.RuleSet) []konveyor.RuleSet {
	flattened := []konveyor.RuleSet{}
	for _, ruleSet := range rulesetsMap {
		if ruleSet != nil {
			rs := *ruleSet
			// deduplicate tags
			rsTagsMap := map[string]interface{}{}
			dedupedTags := []string{}
			for _, tag := range rs.Tags {
				if _, ok := rsTagsMap[tag]; !ok {
					dedupedTags = append(dedupedTags, tag)
					rsTagsMap[tag] = true
				}
			}
			rs.Tags = dedupedTags
			flattened = append(flattened, rs)
		}
	}
	return flattened
}

func mergeTagsIntoMap(target map[string]interface{}, source map[string]interface{}) {
	for tag := range source {
		target[tag] = true
	}
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
			if err != nil || codeSnip == "" {
				r.logger.V(6).Error(err, "unable to get code location")
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
			variables := make(map[string]interface{})
			for key, value := range m.Variables {
				variables[key] = value
			}
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
				r.logger.Error(err, "unable to determine if incident should filter out, defautl to adding")
			}
			if !b {
				r.logger.V(8).Info("filtering out incident based on incident selector")
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
		readFile, err := os.Open(m.FileURI.Filename())
		if err != nil {
			r.logger.V(5).Error(err, "Unable to read file")
			return "", err
		}
		defer readFile.Close()

		scanner := bufio.NewScanner(readFile)
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

func (r *ruleEngine) createPerformString(messageTemplate string, ctx map[string]interface{}) (string, error) {
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

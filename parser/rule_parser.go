package parser

import (
	"fmt"
	"os"
	path "path/filepath"
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/engine/labels"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	"gopkg.in/yaml.v2"
)

const (
	RULE_SET_GOLDEN_FILE_NAME = "ruleset.yaml"
)

var defaultRuleSet = &engine.RuleSet{
	Name: "konveyor-analysis",
}

type parserErrors struct {
	errs []error
}

func (e parserErrors) Error() string {
	s := ""
	for i, e := range e.errs {
		if i == 0 {
			s = e.Error()
		}
		s = fmt.Sprintf("%s\n%s", s, e.Error())
	}
	return s
}

type RuleParser struct {
	ProviderNameToClient map[string]provider.InternalProviderClient
	Log                  logr.Logger
	NoDependencyRules    bool
	DepLabelSelector     *labels.LabelSelector[*provider.Dep]
}

func (r *RuleParser) loadRuleSet(dir string) *engine.RuleSet {
	goldenFile := path.Join(dir, RULE_SET_GOLDEN_FILE_NAME)
	info, err := os.Stat(goldenFile)
	if err != nil {
		r.Log.V(8).Error(err, "unable to load rule set")
		return nil
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	content, err := os.ReadFile(goldenFile)
	if err != nil {
		r.Log.V(8).Error(err, "unable to load rule set")
		return nil
	}

	set := engine.RuleSet{}

	// Assume that there is a rule set header.
	err = yaml.Unmarshal(content, &set)

	if err != nil {
		r.Log.V(8).Error(err, "unable to load rule set")
		return nil
	}
	if len(set.Rules) != 0 {
		r.Log.V(8).Error(fmt.Errorf("rules should not be added in the ruleset"), "unable to load rule set")
		return nil
	}

	return &set
}

// This will load the rules from the filestytem, using the provided provider clients
func (r *RuleParser) LoadRules(filepath string) ([]engine.RuleSet, map[string]provider.InternalProviderClient, error) {
	// Load Rules from file containing rules.
	info, err := os.Stat(filepath)
	if err != nil {
		return nil, nil, err
	}

	// If a single file, then it must have the ruleset metadata.
	if info.Mode().IsRegular() {
		rules, m, err := r.LoadRule(filepath)
		if err != nil {
			r.Log.V(8).Error(err, "unable to load rule set")
			return nil, nil, err
		}

		ruleSet := r.loadRuleSet(path.Dir(filepath))
		// if nil, use the default rule set
		if ruleSet == nil {
			ruleSet = defaultRuleSet
		}
		ruleSet.Rules = rules

		return []engine.RuleSet{*ruleSet}, m, err
	}

	var ruleSets []engine.RuleSet
	clientMap := map[string]provider.InternalProviderClient{}
	// If this takes too long, we should consider moving this to async.
	files, err := os.ReadDir(filepath)
	if err != nil {
		return nil, nil, err
	}
	var ruleSet *engine.RuleSet
	rules := []engine.Rule{}
	foundTree := false
	parserErr := &parserErrors{}
	for _, f := range files {
		info, err := os.Stat(path.Join(filepath, f.Name()))
		if err != nil {
			parserErr.errs = append(parserErr.errs, err)
			continue
		}
		if info.IsDir() {
			foundTree = true
			r, m, err := r.LoadRules(path.Join(filepath, f.Name()))
			if err != nil {
				parserErr.errs = append(parserErr.errs, err)
				continue
			}
			ruleSets = append(ruleSets, r...)
			for k, v := range m {
				clientMap[k] = v
			}
			// If a dir, all the info should be gotten from the regular files
			// found under this tree
			continue
		}
		if info.Mode().IsRegular() {
			if f.Name() == RULE_SET_GOLDEN_FILE_NAME {
				ruleSet = r.loadRuleSet(filepath)
				continue
			}
			r, m, err := r.LoadRule(path.Join(filepath, f.Name()))
			if err != nil {
				parserErr.errs = append(parserErr.errs, err)
				continue
			}
			for k, v := range m {
				clientMap[k] = v
			}
			rules = append(rules, r...)
			continue
		}
	}

	if ruleSet == nil && !foundTree {
		return nil, nil, fmt.Errorf("unable to find %v", RULE_SET_GOLDEN_FILE_NAME)
	}
	if ruleSet != nil {
		ruleSet.Rules = rules
		ruleSets = append(ruleSets, *ruleSet)
	}
	// Return nil if there are no captured errors
	if len(parserErr.errs) == 0 {
		return ruleSets, clientMap, nil
	}
	return ruleSets, clientMap, parserErr
}

func (r *RuleParser) LoadRule(filepath string) ([]engine.Rule, map[string]provider.InternalProviderClient, error) {
	content, err := os.ReadFile(filepath)
	if err != nil {
		return nil, nil, err
	}
	// Determine if the content has a ruleset header.
	// if not, only for a given folder does a ruleset header have to exist.
	ruleMap := []map[string]interface{}{}

	// Assume that there is a rule set header.
	err = yaml.Unmarshal(content, &ruleMap)
	if err != nil {
		r.Log.V(8).Error(err, fmt.Sprintf("unable to load rule set - failed to convert file: %s to yaml", filepath))

		return nil, nil, fmt.Errorf("unable to convert file: %s to yaml", filepath)
	}

	// rules that provide metadata
	infoRules := []engine.Rule{}
	// all rules
	rules := []engine.Rule{}
	ruleIDMap := map[string]*struct{}{}
	providers := map[string]provider.InternalProviderClient{}
	rulesParsed := 0
	for _, ruleMap := range ruleMap {
		ruleID, ok := ruleMap["ruleID"].(string)
		if !ok {
			return nil, nil, fmt.Errorf("unable to find ruleID in rule")
		}

		if _, ok := ruleIDMap[ruleID]; ok {
			return nil, nil, fmt.Errorf("duplicated rule id: %v", ruleID)
		}
		if e, ok := validateRuleID(ruleID); !ok {
			r.Log.Info("invalid rule", "reason", e)
			continue
		}

		// Rules contain When blocks and actions
		// When is where we need to handle conditions
		actions := []string{"message", "tag"}

		perform := engine.Perform{}
		for _, action := range actions {
			if val, exists := ruleMap[action]; exists {
				switch action {
				case "message":
					message, ok := val.(string)
					if !ok {
						return nil, nil, fmt.Errorf("message must be a string")
					}

					linkArray, ok := ruleMap["links"].([]interface{})
					if !ok {
						r.Log.V(8).WithValues("ruleID", ruleID).Info("unable to find linkArray")
					}

					links := []konveyor.Link{}
					for _, linkMap := range linkArray {
						m, ok := linkMap.(map[interface{}]interface{})
						if !ok {
							r.Log.V(8).WithValues("ruleID", ruleID).Info("unable to find link url")
						}
						link := konveyor.Link{}
						link.URL, ok = m["url"].(string)
						if !ok {
							r.Log.V(8).WithValues("ruleID", ruleID).Info("unable to find link url")
						}
						link.Title, ok = m["title"].(string)
						if !ok {
							r.Log.V(8).WithValues("ruleID", ruleID).Info("unable to find link title")
						}

						links = append(links, link)
					}
					perform.Message.Links = links
					perform.Message.Text = &message
				case "tag":
					tagList, ok := val.([]interface{})
					if !ok {
						return nil, nil, fmt.Errorf("tag must be a list of strings")
					}
					for _, tagVal := range tagList {
						tag, ok := tagVal.(string)
						if !ok {
							return nil, nil, fmt.Errorf("tag value must be a string")
						}
						perform.Tag = append(perform.Tag, tag)
					}
				}
			}
		}

		if err := perform.Validate(); err != nil {
			return nil, nil, err
		}

		rule := engine.Rule{
			Perform: perform,
			RuleMeta: engine.RuleMeta{
				RuleID: ruleID,
			},
		}

		r.addRuleFields(&rule, ruleMap)

		whenMap, ok := ruleMap["when"].(map[interface{}]interface{})
		if !ok {
			return nil, nil, fmt.Errorf("a Rule must have a single condition")
		}

		var from string
		var as string
		var ignorable bool
		var not bool
		fromRaw, ok := whenMap["from"]
		if ok {
			delete(whenMap, "from")
			from, ok = fromRaw.(string)
			if !ok {
				return nil, nil, fmt.Errorf("from must be a string literal, not %v", fromRaw)
			}
		}
		asRaw, ok := whenMap["as"]
		if ok {
			delete(whenMap, "as")
			as, ok = asRaw.(string)
			if !ok {
				return nil, nil, fmt.Errorf("as must be a string literal, not %v", asRaw)
			}
		}
		ignorableRaw, ok := whenMap["ignore"]
		if ok {
			delete(whenMap, "ignore")
			ignorable, ok = ignorableRaw.(bool)
			if !ok {
				return nil, nil, fmt.Errorf("ignore must be a boolean, not %v", ignorableRaw)
			}
		}
		// IF there is a not, then we assume a single condition at this level and store it to be used in the default case.
		// There may be a better way of doing this.
		notKeywordRaw, ok := whenMap["not"]
		if ok {
			// Delete from map after getting the value, so that when we range over the when map it does not have to be handeled again.
			delete(whenMap, "not")
			not, ok = notKeywordRaw.(bool)
			if !ok {
				return nil, nil, fmt.Errorf("not must be a boolean, not %v", notKeywordRaw)
			}
		}

		noConditions := false
		for k, value := range whenMap {
			key, ok := k.(string)
			if !ok {
				return nil, nil, fmt.Errorf("condition key must be a string")
			}
			switch key {
			case "or":
				//Handle when clause
				m, ok := value.([]interface{})
				if !ok {
					return nil, nil, fmt.Errorf("invalid type for or clause, must be an array")
				}
				conditions, provs, err := r.getConditions(m)
				if err != nil {
					return nil, nil, err
				}
				if len(conditions) == 0 {
					noConditions = true
				}

				rule.When = engine.OrCondition{Conditions: conditions}
				snippers := []engine.CodeSnip{}
				for k, prov := range provs {
					if snip, ok := prov.(engine.CodeSnip); ok {
						snippers = append(snippers, snip)
					}
					providers[k] = prov
				}
				if len(snippers) > 0 {
					rule.Snipper = provider.CodeSnipProvider{
						Providers: snippers,
					}
				}
			case "and":
				//Handle when clause
				m, ok := value.([]interface{})
				if !ok {
					return nil, nil, fmt.Errorf("invalid type for and clause, must be an array")
				}
				conditions, provs, err := r.getConditions(m)
				if err != nil {
					return nil, nil, err
				}
				if len(conditions) == 0 {
					noConditions = true
				}
				rule.When = engine.AndCondition{Conditions: conditions}
				snippers := []engine.CodeSnip{}
				for k, prov := range provs {
					if snip, ok := prov.(engine.CodeSnip); ok {
						snippers = append(snippers, snip)
					}
					providers[k] = prov
				}
				if len(snippers) > 0 {
					rule.Snipper = provider.CodeSnipProvider{
						Providers: snippers,
					}
				}
			case "":
				return nil, nil, fmt.Errorf("must have at least one condition")
			default:
				// Handle provider
				s := strings.Split(key, ".")
				if len(s) != 2 {
					return nil, nil, fmt.Errorf("condition must be of the form {provider}.{capability}")
				}
				providerKey, capability := s[0], s[1]

				condition, provider, err := r.getConditionForProvider(providerKey, capability, value)
				if err != nil {
					return nil, nil, err
				}
				if condition == nil {
					continue
				}

				c := engine.ConditionEntry{
					From:                   from,
					As:                     as,
					ProviderSpecificConfig: condition,
					Ignorable:              ignorable,
					Not:                    not,
				}
				rule.When = c
				if snipper, ok := provider.(engine.CodeSnip); ok {
					rule.Snipper = snipper
				}
				providers[providerKey] = provider
			}
		}
		if noConditions || rule.When == nil {
			r.Log.V(5).Info("skipping rule no conditions found", "rule", rule.RuleID)
			continue
		}

		ruleIDMap[rule.RuleID] = nil
		if rule.Perform.Tag != nil {
			infoRules = append(infoRules, rule)
		} else {
			rules = append(rules, rule)
		}
		rulesParsed++
		r.Log.V(5).Info("rules parsed", "parsed", rulesParsed)
	}

	return append(infoRules, rules...), providers, nil
}

func validateRuleID(ruleID string) (string, bool) {
	if strings.Contains(ruleID, "\n") {
		return "rule id can not contain string", false

	}
	if strings.Contains(ruleID, ";") {
		return "rule id can not contain semi-colon", false
	}
	return "", true
}

func (r *RuleParser) addRuleFields(rule *engine.Rule, ruleMap map[string]interface{}) {
	labels, ok := ruleMap["labels"].([]interface{})
	if !ok {
		r.Log.V(8).WithValues("ruleID", rule.RuleID).Info("unable to find labels")
	}
	ls := []string{}
	for _, label := range labels {
		s, ok := label.(string)
		if !ok {
			r.Log.V(8).WithValues("ruleID", rule.RuleID).Info("unable to find label")
		}
		ls = append(ls, s)
	}

	rule.Labels = ls

	description, ok := ruleMap["description"].(string)
	if !ok {
		r.Log.V(8).WithValues("ruleID", rule.RuleID).Info("unable to find description")
	}
	rule.Description = description

	if rule.Perform.Message.Text != nil {
		category, ok := ruleMap["category"].(string)
		if !ok {
			r.Log.V(8).WithValues("ruleID", rule.RuleID).Info("unable to find category")
		}
		c := konveyor.Category(strings.ToLower(category))
		if c != konveyor.Potential && c != konveyor.Mandatory && c != konveyor.Optional {
			r.Log.V(8).WithValues("ruleID", rule.RuleID).Info(fmt.Sprintf("unable to find category: %v, defaulting to %v", c, konveyor.Potential))
			rule.Category = &konveyor.Potential
		} else {
			rule.Category = &c
		}
	}

	effort, ok := ruleMap["effort"].(int)
	if !ok {
		r.Log.V(8).WithValues("ruleID", rule.RuleID).Info("unable to find effort")
		rule.Effort = nil
	} else {
		rule.Effort = &effort
	}

	if customVars, ok := ruleMap["customVariables"]; ok {
		var customVarsList []interface{}
		var ok bool
		if customVarsList, ok = customVars.([]interface{}); !ok {
			r.Log.V(5).WithValues("ruleID", rule.RuleID).Info("unable to get custom variables")
			return
		}
		s := []engine.CustomVariable{}
		for _, customVarMapInterface := range customVarsList {
			customVarMap, ok := customVarMapInterface.(map[interface{}]interface{})
			if !ok {
				r.Log.V(5).WithValues("ruleID", rule.RuleID).Info("unable to get custom variables")
				continue
			}
			custVar := engine.CustomVariable{}
			err := r.addCustomVarFields(customVarMap, &custVar)
			if err != nil {
				r.Log.V(5).WithValues("ruleID", rule.RuleID).Error(err, "unable to get custom variables")
				continue
			}
			s = append(s, custVar)
		}
		rule.CustomVariables = s
	}
}

func (r *RuleParser) addCustomVarFields(m map[interface{}]interface{}, customVar *engine.CustomVariable) error {
	if name, ok := m["name"]; ok {
		nameString, ok := name.(string)
		if !ok {
			return fmt.Errorf("unable to get name as string")

		}
		customVar.Name = nameString
	}
	if defaultVal, ok := m["defaultValue"]; ok {
		defaultValString, ok := defaultVal.(string)
		if !ok {
			return fmt.Errorf("unable to get defaultValue as string")
		}
		customVar.DefaultValue = defaultValString
	}

	if capGroup, ok := m["nameOfCaptureGroup"]; ok {
		capGroupString, ok := capGroup.(string)
		if !ok {
			return fmt.Errorf("unable to capture group as string")
		}
		customVar.NameOfCaptureGroup = capGroupString
	}

	if r, ok := m["pattern"]; ok {
		patternString, ok := r.(string)
		if !ok {
			return fmt.Errorf("unable to get pattern as string")
		}
		reg, err := regexp.Compile(patternString)
		if err != nil {
			return err
		}
		customVar.Pattern = reg
	}

	return nil
}

func (r *RuleParser) getConditions(conditionsInterface []interface{}) ([]engine.ConditionEntry, map[string]provider.InternalProviderClient, error) {
	conditions := []engine.ConditionEntry{}
	providers := map[string]provider.InternalProviderClient{}
	chainNameToIndex := map[string]int{}
	asFound := []string{}
	for _, conditionInterface := range conditionsInterface {
		// get map from interface
		conditionMap, ok := conditionInterface.(map[interface{}]interface{})
		if !ok {
			return nil, nil, fmt.Errorf("conditions must be an object")
		}
		var from string
		var as string
		var ignorable bool
		var not bool
		fromRaw, ok := conditionMap["from"]
		if ok {
			delete(conditionMap, "from")
			from, ok = fromRaw.(string)
			if !ok {
				return nil, nil, fmt.Errorf("from must be a string literal, not %v", fromRaw)
			}
		}
		asRaw, ok := conditionMap["as"]
		if ok {
			delete(conditionMap, "as")
			as, ok = asRaw.(string)
			if !ok {
				return nil, nil, fmt.Errorf("as must be a string literal, not %v", asRaw)
			}
		}
		ignorableRaw, ok := conditionMap["ignore"]
		if ok {
			delete(conditionMap, "ignore")
			ignorable, ok = ignorableRaw.(bool)
			if !ok {
				return nil, nil, fmt.Errorf("ignore must be a boolean, not %v", ignorableRaw)
			}
		}
		notKeywordRaw, ok := conditionMap["not"]
		if ok {
			delete(conditionMap, "not")
			not, ok = notKeywordRaw.(bool)
			if !ok {
				return nil, nil, fmt.Errorf("not must be a boolean, not %v", notKeywordRaw)
			}
		}
		for k, v := range conditionMap {
			key, ok := k.(string)
			if !ok {
				return nil, nil, fmt.Errorf("condition key must be string")
			}
			var ce engine.ConditionEntry
			switch key {
			case "and":
				iConditions, ok := v.([]interface{})
				if !ok {
					return nil, nil, fmt.Errorf("inner condition for and is not array")
				}
				conds, provs, err := r.getConditions(iConditions)
				if err != nil {
					return nil, nil, err
				}
				// There was no error so the conditions have all been filtered
				// Return early to prevent constructing an empty rule
				if len(conds) == 0 && len(conds) != len(iConditions) {
					return []engine.ConditionEntry{}, nil, nil
				}
				ce = engine.ConditionEntry{
					From:      from,
					As:        as,
					Ignorable: ignorable,
					Not:       not,
					ProviderSpecificConfig: engine.AndCondition{
						Conditions: conds,
					},
				}
				for k, prov := range provs {
					providers[k] = prov
				}
			case "or":
				iConditions, ok := v.([]interface{})
				if !ok {
					return nil, nil, fmt.Errorf("inner condition for and is not array")
				}
				conds, provs, err := r.getConditions(iConditions)
				if err != nil {
					return nil, nil, err
				}
				// There was no error so the conditions have all been filtered
				// Return early to prevent constructing an empty rule
				if len(conds) == 0 && len(conds) != len(iConditions) {
					return []engine.ConditionEntry{}, nil, nil
				}
				ce = engine.ConditionEntry{
					From:      from,
					As:        as,
					Ignorable: ignorable,
					Not:       not,
					ProviderSpecificConfig: engine.OrCondition{
						Conditions: conds,
					},
				}
				for k, prov := range provs {
					providers[k] = prov
				}
			case "":
				return nil, nil, fmt.Errorf("must have at least one condition")
			default:
				// Need to get condition from provider
				// Handle provider
				s := strings.Split(key, ".")
				if len(s) != 2 {
					return nil, nil, fmt.Errorf("condition must be of the form {provider}.{capability}")
				}
				providerKey, capability := s[0], s[1]

				condition, provider, err := r.getConditionForProvider(providerKey, capability, v)
				if err != nil {
					return nil, nil, err
				}
				if condition == nil {
					continue
				}

				ce = engine.ConditionEntry{
					From:                   from,
					As:                     as,
					ProviderSpecificConfig: condition,
					Ignorable:              ignorable,
					Not:                    not,
				}
				providers[providerKey] = provider
			}
			if ce.From != "" && ce.As != "" && ce.From == ce.As {
				return nil, nil, fmt.Errorf("condition cannot have the same value for fields 'from' and 'as'")
			} else if ce.As != "" {
				for _, as := range asFound {
					if as == ce.As {
						return nil, nil, fmt.Errorf("condition cannot have multiple 'as' fields with the same name")
					}
				}
				asFound = append(asFound, ce.As)

				index, ok := chainNameToIndex[ce.As]
				if !ok {
					//prepend
					conditions = append([]engine.ConditionEntry{ce}, conditions...)
				} else {
					//insert
					conditions = append(conditions[:index+1], conditions[index:]...)
					conditions[index] = ce
				}
			} else if ce.From != "" && ce.As == "" {
				chainNameToIndex[ce.From] = len(conditions)
				conditions = append(conditions, ce)
			} else {
				conditions = append(conditions, ce)
			}
		}
	}

	return conditions, providers, nil
}

func (r *RuleParser) getConditionForProvider(langProvider, capability string, value interface{}) (engine.Conditional, provider.InternalProviderClient, error) {
	// Here there can only be a single provider.
	client, ok := r.ProviderNameToClient[langProvider]
	if !ok {
		return nil, nil, fmt.Errorf("unable to find provider for: %v", langProvider)
	}

	if !provider.HasCapability(client.Capabilities(), capability) {
		return nil, nil, fmt.Errorf("unable to find cap: %v from provider: %v", capability, langProvider)
	}

	ignorable := false
	if m, ok := value.(map[string]interface{}); ok {
		if v, ok := m["ignore"]; ok {
			if i, ok := v.(bool); ok {
				ignorable = i
			}
		}
	}

	if capability == "dependency" && !r.NoDependencyRules {
		depCondition := provider.DependencyCondition{
			Client: client,
		}

		fullCondition, ok := value.(map[interface{}]interface{})
		if !ok {
			return nil, nil, fmt.Errorf("Unable to parse dependency condition for %s", langProvider)
		}
		for k, v := range fullCondition {
			key, ok := k.(string)
			if !ok {
				return nil, nil, fmt.Errorf("Unable to parse dependency condition for %s", langProvider)
			}
			value, ok := v.(string)
			if !ok {
				return nil, nil, fmt.Errorf("Unable to parse dependency condition for %s", langProvider)
			}
			switch key {
			case "name":
				depCondition.Name = value
			case "upperbound":
				depCondition.Upperbound = value
			case "lowerbound":
				depCondition.Lowerbound = value
			case "nameregex":
				depCondition.NameRegex = value
			default:
				return nil, nil, fmt.Errorf("%s is not a valid argument for a dependency condition", key)
			}
		}
		if depCondition.NameRegex != "" {
			return &depCondition, client, nil

		}
		if depCondition.Name == "" {
			return nil, nil, fmt.Errorf("Unable to parse dependency condition for %s (name is required)", langProvider)
		}

		if depCondition.Upperbound == "" && depCondition.Lowerbound == "" {
			return nil, nil, fmt.Errorf("Unable to parse dependency condition for %s (one of upperbound or lowerbound is required)", langProvider)
		}

		return &depCondition, client, nil
	} else if capability == "dependency" && r.NoDependencyRules {
		r.Log.V(5).Info(fmt.Sprintf("not evaluating dependency condition - %s.%s for %#v", langProvider, capability, value))
		return nil, nil, nil
	}

	var selector *labels.LabelSelector[*provider.Dep]
	// Only set this, if the client has deps.
	if r.DepLabelSelector != nil && provider.HasCapability(client.Capabilities(), "dependency") {
		selector = r.DepLabelSelector
	}

	return provider.ProviderCondition{
		Client:           client,
		Capability:       capability,
		ConditionInfo:    value,
		Ignore:           ignorable,
		DepLabelSelector: selector,
	}, client, nil
}

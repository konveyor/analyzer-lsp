package parser

import (
	"fmt"
	"os"
	path "path/filepath"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/provider"
)

const (
	RULE_SET_GOLDEN_FILE_NAME = "ruleset.yaml"
)

var defaultRuleSet = &engine.RuleSet{
	Name: "konveyor-analysis",
}

type RuleParser struct {
	ProviderNameToClient map[string]provider.Client
	Log                  logr.Logger
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
func (r *RuleParser) LoadRules(filepath string) ([]engine.RuleSet, map[string]provider.Client, error) {
	// Load Rules from file containing rules.
	info, err := os.Stat(filepath)
	if err != nil {
		return nil, nil, err
	}

	// If a single file, then it must have the ruleset metadata.
	if info.Mode().IsRegular() {
		rules, m, err := r.LoadRule(filepath)
		if err != nil {
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
	clientMap := map[string]provider.Client{}
	// If this takes too long, we should consider moving this to async.
	files, err := os.ReadDir(filepath)
	if err != nil {
		return nil, nil, err
	}
	var ruleSet *engine.RuleSet
	rules := []engine.Rule{}
	foundTree := false
	for _, f := range files {
		info, err := os.Stat(path.Join(filepath, f.Name()))
		if err != nil {
			return nil, nil, err
		}
		if info.IsDir() {
			foundTree = true
			r, m, err := r.LoadRules(path.Join(filepath, f.Name()))
			if err != nil {
				return nil, nil, err
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
				return nil, nil, err
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
	return ruleSets, clientMap, err
}

func (r *RuleParser) LoadRule(filepath string) ([]engine.Rule, map[string]provider.Client, error) {
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
		return nil, nil, err
	}

	// rules that provide metadata
	infoRules := []engine.Rule{}
	// all rules
	rules := []engine.Rule{}
	ruleIDMap := map[string]*struct{}{}
	providers := map[string]provider.Client{}
	for _, ruleMap := range ruleMap {
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
					perform.Message = &message
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

		ruleID, ok := ruleMap["ruleID"].(string)
		if !ok {
			return nil, nil, fmt.Errorf("unable to find ruleID in rule")
		}

		if _, ok := ruleIDMap[ruleID]; ok {
			return nil, nil, fmt.Errorf("duplicated rule id: %v", ruleID)
		}

		rule := engine.Rule{
			Perform: perform,
			RuleID:  ruleID,
		}

		whenMap, ok := ruleMap["when"].(map[interface{}]interface{})
		if !ok {
			return nil, nil, fmt.Errorf("a Rule must have a single condition")
		}

		// IF there is a not, then we assume a single condition at this level and store it to be used in the default case.
		// There may be a better way of doing this.
		notKeywordRaw, ok := whenMap["not"]
		not := false
		if ok {
			// Delete from map after getting the value, so that when we range over the when map it does not have to be handeled again.
			delete(whenMap, "not")
			not, ok = notKeywordRaw.(bool)
			if !ok {
				return nil, nil, fmt.Errorf("not must be a boolean, not %v", notKeywordRaw)
			}
		}

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
				rule.When = engine.OrCondition{Conditions: conditions}
				for k, prov := range provs {
					providers[k] = prov
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
				rule.When = engine.AndCondition{Conditions: conditions}
				for k, prov := range provs {
					providers[k] = prov
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

				c := engine.ConditionEntry{
					Not:                    not,
					ProviderSpecificConfig: condition,
				}
				rule.When = c
				providers[providerKey] = provider
			}
		}

		ruleIDMap[rule.RuleID] = nil
		if rule.Perform.Tag != nil {
			infoRules = append(infoRules, rule)
		} else {
			rules = append(rules, rule)
		}
	}

	return append(infoRules, rules...), providers, nil

}

func (r *RuleParser) getConditions(conditionsInterface []interface{}) ([]engine.ConditionEntry, map[string]provider.Client, error) {

	conditions := []engine.ConditionEntry{}
	providers := map[string]provider.Client{}
	chainNameToIndex := map[string]int{}
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
				return nil, nil, fmt.Errorf("as must be a boolean, not %v", ignorableRaw)
			}
		}
		notKeywordRaw, ok := conditionMap["not"]
		if ok {
			delete(conditionMap, "not")
			not, ok = notKeywordRaw.(bool)
			if !ok {
				return nil, nil, fmt.Errorf("not must a boolean, not %v", notKeywordRaw)
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

				ce = engine.ConditionEntry{
					From:                   from,
					As:                     as,
					ProviderSpecificConfig: condition,
					Ignorable:              ignorable,
					Not:                    not,
				}
				providers[providerKey] = provider
			}
			if ce.As != "" {
				index, ok := chainNameToIndex[ce.As]
				if !ok {
					//prepend
					conditions = append([]engine.ConditionEntry{ce}, conditions...)
				}
				//insert
				conditions = append(conditions[:index+1], conditions[index:]...)
				conditions[index] = ce
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

func (r *RuleParser) getConditionForProvider(langProvider, capability string, value interface{}) (engine.Conditional, provider.Client, error) {
	// Here there can only be a single provider.
	client, ok := r.ProviderNameToClient[langProvider]
	if !ok {
		return nil, nil, fmt.Errorf("unable to find provider for :%v", langProvider)
	}

	if !client.HasCapability(capability) {
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

	if capability == "dependency" {
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
			default:
				return nil, nil, fmt.Errorf("%s is not a valid argument for a dependency condition", key)
			}
		}

		if depCondition.Name == "" {
			return nil, nil, fmt.Errorf("Unable to parse dependency condition for %s (name is required)", langProvider)
		}
		if depCondition.Upperbound == "" && depCondition.Lowerbound == "" {
			return nil, nil, fmt.Errorf("Unable to parse dependency condition for %s (one of upperbound or lowerbound is required)", langProvider)
		}

		return &depCondition, client, nil
	}

	return &provider.ProviderCondition{
		Client:        client,
		Capability:    capability,
		ConditionInfo: value,
		Ignore:        ignorable,
	}, client, nil
}

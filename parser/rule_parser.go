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

type RuleParser struct {
	ProviderNameToClient map[string]provider.Client
	Log                  logr.Logger
}

// This will load the rules from the filestytem, using the provided provider clients
func (r *RuleParser) LoadRules(filepath string) ([]engine.Rule, map[string]provider.Client, error) {
	// Load Rules from file containing rules.
	info, err := os.Stat(filepath)
	if err != nil {
		return nil, nil, err
	}

	if info.Mode().IsRegular() {
		return r.LoadRule(filepath)
	}

	var rules []engine.Rule
	clientMap := map[string]provider.Client{}
	// If this takes too long, we should consider moving this to async.
	files, err := os.ReadDir(filepath)
	if err != nil {
		return rules, nil, err
	}
	for _, f := range files {
		r, m, err := r.LoadRules(path.Join(filepath, f.Name()))
		if err != nil {
			return nil, nil, err
		}
		for k, v := range m {
			clientMap[k] = v
		}
		rules = append(rules, r...)
	}

	if err != nil {
		return nil, nil, err
	}

	return rules, clientMap, err
}

func (r *RuleParser) LoadRule(filepath string) ([]engine.Rule, map[string]provider.Client, error) {
	content, err := os.ReadFile(filepath)
	if err != nil {
		return nil, nil, err
	}

	ruleMap := []map[string]interface{}{}

	err = yaml.Unmarshal(content, &ruleMap)
	if err != nil {
		return nil, nil, err
	}

	rules := []engine.Rule{}
	providers := map[string]provider.Client{}
	for _, ruleMap := range ruleMap {
		// Rules right now only contain two top level things, message and when.
		// When is where we need to handle conditions
		message, ok := ruleMap["message"].(string)
		if !ok {
			return nil, nil, fmt.Errorf("unable to find message in rule")
		}
		ruleID, ok := ruleMap["ruleID"].(string)
		if !ok {
			return nil, nil, fmt.Errorf("unable to find ruleID in rule")
		}

		rule := engine.Rule{
			Perform: message,
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
				return nil, nil, fmt.Errorf("not must a boolean, not %v", notKeywordRaw)
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
			case "chain":
				m, ok := value.([]interface{})
				if !ok {
					return nil, nil, fmt.Errorf("invalid type for chain clause, must be an array")
				}
				conditions, provs, err := r.getConditions(m)
				if err != nil {
					return nil, nil, err
				}
				rule.When = engine.ChainCondition{Conditions: conditions}
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

		rules = append(rules, rule)
	}

	return rules, providers, nil

}

func (r *RuleParser) getConditions(conditionsInterface []interface{}) ([]engine.ConditionEntry, map[string]provider.Client, error) {

	conditions := []engine.ConditionEntry{}
	providers := map[string]provider.Client{}
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
				conditions = append(conditions, engine.ConditionEntry{
					From:      from,
					As:        as,
					Ignorable: ignorable,
					Not:       not,
					ProviderSpecificConfig: engine.AndCondition{
						Conditions: conds,
					},
				})
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
				conditions = append(conditions, engine.ConditionEntry{
					From:      from,
					As:        as,
					Ignorable: ignorable,
					Not:       not,
					ProviderSpecificConfig: engine.OrCondition{
						Conditions: conds,
					},
				})
				for k, prov := range provs {
					providers[k] = prov
				}
			case "chain":
				iConditions, ok := v.([]interface{})
				if !ok {
					return nil, nil, fmt.Errorf("inner condition for and is not array")
				}
				conds, provs, err := r.getConditions(iConditions)
				if err != nil {
					return nil, nil, err
				}
				conditions = append(conditions, engine.ConditionEntry{
					From:      from,
					As:        as,
					Ignorable: ignorable,
					Not:       not,
					ProviderSpecificConfig: engine.ChainCondition{
						Conditions: conds,
					},
				})
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
				conditions = append(conditions, engine.ConditionEntry{
					From:                   from,
					As:                     as,
					ProviderSpecificConfig: condition,
					Ignorable:              ignorable,
					Not:                    not,
				})
				providers[providerKey] = provider
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

	providerCaps, err := client.Capabilities()
	if err != nil {
		return nil, nil, err
	}
	found := false
	for _, c := range providerCaps {
		if c == capability {
			found = true
			break
		}
	}
	if !found {
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

	return &provider.ProviderCondition{
		Client:        client,
		Capability:    capability,
		ConditionInfo: value,
		Ignore:        ignorable,
	}, client, nil
}

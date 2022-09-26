package parser

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/shawn-hurley/jsonrpc-golang/engine"
	"github.com/shawn-hurley/jsonrpc-golang/provider"
)

type RuleParser struct {
	ProviderNameToClient map[string]provider.Client
}

// This will load the rules from the filestytem, using the provided provider clients
func (r *RuleParser) LoadRules(filepath string) ([]engine.Rule, []provider.Client, error) {
	// Load Rules from file containing rules.
	content, err := os.ReadFile(filepath)
	if err != nil {
		return nil, nil, err
	}

	ruleMap := []map[string]interface{}{}

	err = json.Unmarshal(content, &ruleMap)
	if err != nil {
		return nil, nil, err
	}

	rules := []engine.Rule{}
	providers := []provider.Client{}
	for _, ruleMap := range ruleMap {
		// Rules right now only contain two top level things, message and when.
		// When is where we need to handle conditions
		message, ok := ruleMap["message"].(string)
		if !ok {
			return nil, nil, fmt.Errorf("unable to find message in rule")
		}

		rule := engine.Rule{
			Perform: message,
		}

		whenMap, ok := ruleMap["when"].(map[string]interface{})
		if !ok {
			return nil, nil, fmt.Errorf("a Rule must have a single condition")
		}

		for key, value := range whenMap {
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
				providers = append(providers, provs...)
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
				providers = append(providers, provs...)
			default:
				// Handle provider
				if key == "" {
					return nil, nil, fmt.Errorf("must have at least one condition")
				}
				m, ok := value.(map[string]interface{})
				if !ok {
					return nil, nil, fmt.Errorf("single condition must be a object")
				}
				condition, provider, err := r.getConditionForProvider(key, m)
				if err != nil {
					return nil, nil, err
				}
				rule.When = condition
				providers = append(providers, provider)
			}
		}

		rules = append(rules, rule)
	}

	return rules, providers, nil

}

func (r *RuleParser) getConditions(conditionsInterface []interface{}) ([]engine.Conditional, []provider.Client, error) {

	conditions := []engine.Conditional{}
	providers := []provider.Client{}
	for _, conditionInterface := range conditionsInterface {
		// get map from interface
		conditionMap, ok := conditionInterface.(map[string]interface{})
		if !ok {
			return nil, nil, fmt.Errorf("conditions must be an object")
		}
		for key, v := range conditionMap {
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
				conditions = append(conditions, engine.AndCondition{
					Conditions: conds,
				})
				providers = append(providers, provs...)
			case "or":
				iConditions, ok := v.([]interface{})
				if !ok {
					return nil, nil, fmt.Errorf("inner condition for and is not array")
				}
				conds, provs, err := r.getConditions(iConditions)
				if err != nil {
					return nil, nil, err
				}
				conditions = append(conditions, engine.OrCondition{
					Conditions: conds,
				})
				providers = append(providers, provs...)
			default:
				// Need to get condition from provider
				// Handle provider
				if key == "" {
					return nil, nil, fmt.Errorf("must have at least one condition")
				}
				m, ok := v.(map[string]interface{})
				if !ok {
					return nil, nil, fmt.Errorf("single condition must be a object")
				}
				condition, prov, err := r.getConditionForProvider(key, m)
				if err != nil {
					return nil, nil, err
				}
				conditions = append(conditions, condition)
				providers = append(providers, prov)
			}
		}
	}

	return conditions, providers, nil

}

func (r *RuleParser) getConditionForProvider(langProvider string, capMap map[string]interface{}) (engine.Conditional, provider.Client, error) {
	// Here there can only be a single provider.
	client, ok := r.ProviderNameToClient[langProvider]
	if !ok {
		return nil, nil, fmt.Errorf("unable to find provider for :%v", langProvider)
	}

	providerCaps, err := client.Capabilities()
	if err != nil {
		return nil, nil, err
	}

	if len(capMap) != 1 {
		return nil, nil, fmt.Errorf("must have a single capability for a condition")
	}

	for capKey, value := range capMap {
		found := false
		for _, c := range providerCaps {
			if c == capKey {
				found = true
				break
			}
		}
		if !found {
			return nil, nil, fmt.Errorf("unable to find cap: %v from provider: %v", capKey, langProvider)
		}
		return &provider.ProviderCondition{
			Client:        client,
			Capability:    capKey,
			ConditionInfo: value,
		}, client, nil
	}
	return nil, nil, fmt.Errorf("unable to get condition")
}

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

// This will
func (r *RuleParser) LoadRules(filepath string) ([]engine.Rule, error) {
	// Load Rules from file containing rules.
	content, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	ruleMap := []map[string]interface{}{}

	err = json.Unmarshal(content, &ruleMap)
	if err != nil {
		return nil, err
	}

	rules := []engine.Rule{}
	for _, ruleMap := range ruleMap {
		// Rule's right now only contain two top level things, message and when.
		// When is where we need to handle conditions
		message, ok := ruleMap["message"].(string)
		if !ok {
			return nil, fmt.Errorf("unable to find message in rule")
		}

		rule := engine.Rule{
			Perform: message,
			When:    engine.Conditional{},
		}

		whenMap, ok := ruleMap["when"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("a Rule must have a single condition")
		}

		for key, value := range whenMap {
			switch key {
			case "or":
				//Handle when clause
				fmt.Printf("%#v", value)
				m, ok := value.([]interface{})
				if !ok {
					return nil, fmt.Errorf("invalid type for or clause, must be an array")
				}
				conditions, err := r.getConditions(m)
				if err != nil {
					return nil, err
				}
				rule.When.Or = conditions
			case "and":
				//Handle when clause
				m, ok := value.([]interface{})
				if !ok {
					return nil, fmt.Errorf("invalid type for and clause, must be an array")
				}
				conditions, err := r.getConditions(m)
				if err != nil {
					return nil, err
				}
				rule.When.And = conditions
			default:
				// Handle provider
				if key == "" {
					return nil, fmt.Errorf("must have at least one condition")
				}
				m, ok := value.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("single condition must be a object")
				}
				condition, err := r.getConditionForProvider(key, m)
				if err != nil {
					return nil, err
				}
				rule.When.InnerCondition = condition
			}
		}

		rules = append(rules, rule)
	}

	return rules, nil

}

func (r *RuleParser) getConditions(conditionsInterface []interface{}) ([]engine.Condition, error) {

	conditions := []engine.Condition{}
	for _, conditionInterface := range conditionsInterface {
		// get map from interface
		conditionMap, ok := conditionInterface.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("conditions must be an object")
		}
		for key, v := range conditionMap {
			switch key {
			case "and":
				fmt.Printf("here")
				iConditions, ok := v.([]interface{})
				if !ok {
					return nil, fmt.Errorf("inner condition for and is not array")
				}
				conds, err := r.getConditions(iConditions)
				if err != nil {
					return nil, err
				}
				conditions = append(conditions, engine.Condition{
					When: &engine.Conditional{
						And: conds,
					},
				})
			case "or":
				iConditions, ok := v.([]interface{})
				if !ok {
					return nil, fmt.Errorf("inner condition for and is not array")
				}
				conds, err := r.getConditions(iConditions)
				if err != nil {
					return nil, err
				}
				conditions = append(conditions, engine.Condition{
					When: &engine.Conditional{
						Or: conds,
					},
				})
			default:
				// Need to get condition from provider
				// Handle provider
				if key == "" {
					return nil, fmt.Errorf("must have at least one condition")
				}
				m, ok := v.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("single condition must be a object")
				}
				fmt.Printf("\n\n%v: %v\n\n", key, v)
				condition, err := r.getConditionForProvider(key, m)
				if err != nil {
					return nil, err
				}
				c := engine.Condition{
					InnerCondition: condition,
				}
				conditions = append(conditions, c)
			}

		}

	}

	return conditions, nil

}

func (r *RuleParser) getConditionForProvider(langProvider string, capMap map[string]interface{}) (engine.InnerCondition, error) {
	// Here there can only be a single provider.
	fmt.Printf("\n%v\n", capMap)
	client, ok := r.ProviderNameToClient[langProvider]
	if !ok {
		return engine.Condition{}, fmt.Errorf("unable to find provider for :%v", langProvider)
	}

	providerCaps, err := client.Capabilities()
	if err != nil {
		return engine.Condition{}, err
	}

	if len(capMap) != 1 {
		return engine.Condition{}, fmt.Errorf("must have a single capability for a condition")
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
			return engine.Condition{}, fmt.Errorf("unable to find cap: %v from provider: %v", capKey, langProvider)
		}
		return engine.Condition{
			InnerCondition: &provider.ProviderCondition{
				Client:        client,
				Capability:    capKey,
				ConditionInfo: value,
			},
		}, nil

	}
	return engine.Condition{}, fmt.Errorf("unable to get condition")
}

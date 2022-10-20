package parser

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/provider"
)

type RuleParser struct {
	ProviderNameToClient map[string]provider.Client
	log                  logr.Logger
}

// This will load the rules from the filestytem, using the provided provider clients
func (r *RuleParser) LoadRules(filepath string) ([]engine.Rule, []provider.Client, error) {
	// Load Rules from file containing rules.
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

		rule := engine.Rule{
			Perform: message,
		}

		whenMap, ok := ruleMap["when"].(map[interface{}]interface{})
		if !ok {
			return nil, nil, fmt.Errorf("a Rule must have a single condition")
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
				rule.When = condition
				providers[providerKey] = provider

			}
		}

		rules = append(rules, rule)
	}

	providerList := []provider.Client{}
	for _, p := range providers {
		providerList = append(providerList, p)
	}

	return rules, providerList, nil

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
				fmt.Println(from)
				fmt.Println(as)
				conditions = append(conditions, engine.ConditionEntry{From: from, As: as, ProviderSpecificConfig: engine.AndCondition{
					Conditions: conds,
				}})
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
				conditions = append(conditions, engine.ConditionEntry{From: from, As: as, ProviderSpecificConfig: engine.OrCondition{
					Conditions: conds,
				}})
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
				conditions = append(conditions, engine.ConditionEntry{From: from, As: as, ProviderSpecificConfig: engine.ChainCondition{
					Conditions: conds,
				}})
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
				conditions = append(conditions, engine.ConditionEntry{From: from, As: as, ProviderSpecificConfig: condition})
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
	return &provider.ProviderCondition{
		Client:        client,
		Capability:    capability,
		ConditionInfo: value,
	}, client, nil
}

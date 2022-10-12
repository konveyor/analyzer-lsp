package engine

import (
	"fmt"

	"github.com/go-logr/logr"
)

type ConditionResponse struct {
	Passed bool `yaml:"passed"`
	// For each time the condition is hit, add all of the context.
	// keys here, will be used in the message.
	ConditionHitContext []map[string]string `yaml:"conditionHitContext"`
}

type Conditional interface {
	Evaluate(log logr.Logger) (ConditionResponse, error)
}

type Rule struct {
	Perform string      `yaml:"perform,omitempty"`
	When    Conditional `yaml:"when,omitempty"`
}

type AndCondition struct {
	Conditions []Conditional
}

func (a AndCondition) Evaluate(log logr.Logger) (ConditionResponse, error) {

	if len(a.Conditions) == 0 {
		return ConditionResponse{}, fmt.Errorf("conditions must not be empty while evaluationg")
	}

	fullResponse := ConditionResponse{Passed: true}
	for _, c := range a.Conditions {
		response, err := c.Evaluate(log)

		// Short cirtcut loop if one and condition fails
		if !response.Passed {
			return response, err
		}
	}

	return fullResponse, nil
}

type OrCondition struct {
	Conditions []Conditional
}

func (o OrCondition) Evaluate(log logr.Logger) (ConditionResponse, error) {
	if len(o.Conditions) == 0 {
		return ConditionResponse{}, fmt.Errorf("conditions must not be empty while evaluationg")
	}

	for _, c := range o.Conditions {
		response, err := c.Evaluate(log)
		// Short cirtcut loop if one or condition passes we can move on
		// We may not want to do this in the future.
		if response.Passed {
			return response, err
		}
	}

	// if no coditions are true, then nothing returns early, and it means or is not true
	return ConditionResponse{}, nil
}

var _ Conditional = AndCondition{}

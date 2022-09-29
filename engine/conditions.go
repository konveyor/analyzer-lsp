package engine

import "fmt"

type CondtionResponse struct {
	Passed bool
	// For each time the condition is hit, add all of the context.
	// keys here, will be used in the message.
	ConditionHitContext []map[string]string
}

type Conditional interface {
	Evaluate() (CondtionResponse, error)
}

type Rule struct {
	Perform string      `json:"perform,omitempty"`
	When    Conditional `json:"when,omitempty"`
}

type AndCondition struct {
	Conditions []Conditional
}

func (a AndCondition) Evaluate() (CondtionResponse, error) {

	if len(a.Conditions) == 0 {
		return CondtionResponse{}, fmt.Errorf("condtions must not be empty while evaluationg")
	}

	fullResponse := CondtionResponse{Passed: true}
	for _, c := range a.Conditions {
		response, err := c.Evaluate()

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

func (o OrCondition) Evaluate() (CondtionResponse, error) {
	if len(o.Conditions) == 0 {
		return CondtionResponse{}, fmt.Errorf("conditions must not be empty while evaluationg")
	}

	for _, c := range o.Conditions {
		response, err := c.Evaluate()
		// Short cirtcut loop if one or condition passes we can move on
		// We may not want to do this in the future.
		if response.Passed {
			return response, err
		}
	}

	// if no coditions are true, then nothing returns early, and it means or is not true
	return CondtionResponse{}, nil
}

var _ Conditional = AndCondition{}

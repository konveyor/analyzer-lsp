package engine

import (
	"fmt"

	"github.com/go-logr/logr"
)

var _ Conditional = AndCondition{}
var _ Conditional = OrCondition{}
var _ Conditional = ChainCondition{}

type ConditionResponse struct {
	Passed bool `yaml:"passed"`
	// For each time the condition is hit, add all of the context.
	// keys here, will be used in the message.
	ConditionHitContext []map[string]string    `yaml:"conditionHitContext"`
	TemplateContext     map[string]interface{} `yaml:",inline"`
}

type ConditionEntry struct {
	From                   string
	As                     string
	ProviderSpecificConfig Conditional
}

// TODO maybe do this
// func validateConditionVariables(conds []ConditionEntry) bool {
// 	// Flatten all entries (ands/ors/nested chains etc if we want to support that
// // Alternatively we could just leave it to the user to enforce ordering
// 	// Run through condition entries to make sure we've got indices for all names
// 	for i, c := range conds {
// 	}
// 	g := graph{
// 		nodes: []*node{},
// 		edges: map[node][]*node{},
// 	}
// 	for i, c := range conds {
// 		if c.As != "" {
// 			g.AddNode(node{index: i, name: c.As})
// 		}
// 		if c.From != "" {
// 			// g.AddEdge()
// 		}
// 	}
// 	return false
// }

// type graph struct {
// 	nodes []*node
// 	edges map[node][]*node
// }

// type node struct {
// 	index int
// 	name  string
// }

// func (g *graph) AddNode(n node) {
// 	g.nodes = append(g.nodes, &n)
// }

// func (g *graph) AddEdge(node1, node2 node) {
// 	g.edges[node1] = append(g.edges[node1], &node2)
// 	g.edges[node2] = append(g.edges[node2], &node1)
// }

type Conditional interface {
	Evaluate(log logr.Logger, ctx map[string]interface{}) (ConditionResponse, error)
}

type Rule struct {
	Perform string      `yaml:"perform,omitempty"`
	When    Conditional `yaml:"when,omitempty"`
}

type AndCondition struct {
	Conditions []ConditionEntry `yaml:"and"`
}

func (a AndCondition) Evaluate(log logr.Logger, ctx map[string]interface{}) (ConditionResponse, error) {

	if len(a.Conditions) == 0 {
		return ConditionResponse{}, fmt.Errorf("conditions must not be empty while evaluating")
	}

	fullResponse := ConditionResponse{Passed: true}
	for _, c := range a.Conditions {
		response, err := c.ProviderSpecificConfig.Evaluate(log, ctx)

		// Short cirtcut loop if one and condition fails
		if !response.Passed {
			return response, err
		}
	}

	return fullResponse, nil
}

type OrCondition struct {
	Conditions []ConditionEntry `yaml:"or"`
}

func (o OrCondition) Evaluate(log logr.Logger, ctx map[string]interface{}) (ConditionResponse, error) {
	if len(o.Conditions) == 0 {
		return ConditionResponse{}, fmt.Errorf("conditions must not be empty while evaluationg")
	}

	for _, c := range o.Conditions {
		response, err := c.ProviderSpecificConfig.Evaluate(log, ctx)
		// Short cirtcut loop if one or condition passes we can move on
		// We may not want to do this in the future.
		if response.Passed {
			return response, err
		}
	}

	// if no coditions are true, then nothing returns early, and it means or is not true
	return ConditionResponse{}, nil
}

type ChainCondition struct {
	Conditions []ConditionEntry `yaml:"chain"`
}

func (ch ChainCondition) Evaluate(log logr.Logger, ctx map[string]interface{}) (ConditionResponse, error) {

	if len(ch.Conditions) == 0 {
		return ConditionResponse{}, fmt.Errorf("conditions must not be empty while evaluating")
	}

	fullResponse := ConditionResponse{Passed: true}
	var hitContext []map[string]string
	var passed bool
	for _, c := range ch.Conditions {
		response, err := c.ProviderSpecificConfig.Evaluate(log, ctx)
		if err != nil {
			return fullResponse, err
		}
		if c.As != "" {
			ctx[c.As] = response.TemplateContext
		}
		passed = response.Passed
		hitContext = response.ConditionHitContext
	}
	fullResponse.Passed = passed
	fullResponse.TemplateContext = ctx
	fullResponse.ConditionHitContext = hitContext

	return fullResponse, nil
}

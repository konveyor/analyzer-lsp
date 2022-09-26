package engine

type InnerConndtionResponse struct {
	Passed bool
	// For each time the condition is hit, add all of the context.
	// keys here, will be used in the message.
	ConditionHitContext []map[string]string
}

type InnerCondition interface {
	Evaluate() (InnerConndtionResponse, error)
}

type Condition struct {
	// Optional When clause
	When *Conditional `json:"when,omitempty"`

	InnerCondition `json:"innerCondition,omitempty"`
}

type Conditional struct {
	Or  []Condition `json:"or,omitempty"`
	And []Condition `json:"and,omitempty"`

	InnerCondition `json:"innerCondition,omitempty"`
}

type Rule struct {
	Perform string      `json:"perform,omitempty"`
	When    Conditional `json:"when,omitempty"`
}

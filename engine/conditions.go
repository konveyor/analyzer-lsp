package engine

type InnerCondition interface {
	Evaluate() (bool, error)
}
type Condition struct {
	// Optional When clause
	When *Conditional

	InnerCondition
}

type Conditional struct {
	Or  []Condition
	And []Condition

	InnerCondition
}

type Rule struct {
	Perform string
	When    Conditional
}

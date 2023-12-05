package internal

import "fmt"

type VariableLabelSelector map[string]interface{}

func (v VariableLabelSelector) GetLabels() []string {
	if len(v) == 0 {
		// adding a single empty string will allow for Not selectors to match
		// incidents that have no variables
		return []string{""}
	}
	s := []string{}
	for k, v := range v {
		s = append(s, fmt.Sprintf("%s=%s", k, v))
	}
	return s
}

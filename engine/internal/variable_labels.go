package internal

import (
	"fmt"
	"strings"
)

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

func MatchVariables(elem string, items []string) bool {
	// Adding the trailing . to make sure that com.example.apps matches but not com.example2.apps
	var containsSearch bool
	if strings.Contains(elem, ".") {
		elem = fmt.Sprintf("%v.", elem)
		containsSearch = true
	}
	for _, i := range items {
		if containsSearch {
			if strings.Contains(i, elem) {
				return true
			}
		} else {
			if i == elem {
				return true
			}
		}
	}
	return false

}

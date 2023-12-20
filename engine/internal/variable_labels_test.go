package internal

import (
	"testing"

	"github.com/konveyor/analyzer-lsp/engine/labels"
)

func Test_incidentSelector(t *testing.T) {
	tests := []struct {
		name             string
		incidentSelector string
		variables        map[string]interface{}
		want             bool
	}{
		{
			name:             "expression 001",
			incidentSelector: "(!package || package=io.konveyor.demo.ordermanagement)",
			variables: map[string]interface{}{
				"package": "io.konveyor.demo.ordermanagement.controller",
			},
			want: true,
		},
		{
			name:             "expression 002",
			incidentSelector: "(package && package=io.konveyor.demo.ordermanagement.controller)",
			variables: map[string]interface{}{
				"package": "io.konveyor.demo.ordermanagement.controller",
			},
			want: true,
		},
		{
			name:             "expression 003",
			incidentSelector: "package=io.konveyor.demo.ordermanagement.controller",
			variables: map[string]interface{}{
				"package": "io.konveyor.demo.ordermanagement",
			},
			want: false,
		},
		{
			name:             "expression 004",
			incidentSelector: "package=io.konveyor.demo.ordermanagement.controller",
			variables: map[string]interface{}{
				"package": "io.konveyor.demo.ordermanagement.controller",
			},
			want: true,
		},
	}
	for _, tt := range tests {
		var incidentSelector *labels.LabelSelector[VariableLabelSelector]
		var err error
		if tt.incidentSelector != "" {
			incidentSelector, err = labels.NewLabelSelector[VariableLabelSelector](tt.incidentSelector, MatchVariables)
			if err != nil {
				t.Errorf(err.Error())
			}
		}
		t.Run(tt.name, func(t *testing.T) {
			got, err := incidentSelector.Matches(VariableLabelSelector(tt.variables))
			if err != nil {
				// t.Errorf("Got %v, want %s", b, tt.want)
				t.Errorf(err.Error())
			}
			if got != tt.want {
				t.Errorf("Got %v, want %v", got, tt.want)
			}
		})
	}
}

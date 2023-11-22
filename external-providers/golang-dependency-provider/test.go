package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/konveyor/analyzer-lsp/provider"
)

func Test_parseGoDepLines(t *testing.T) {
	tests := []struct {
		name        string
		goDepOutput string
		want        []provider.DepDAGItem
		wantErr     bool
	}{
		{
			name:        "an empty go dep output shouldn't produce anything",
			goDepOutput: "",
			want:        []provider.DepDAGItem{},
			wantErr:     false,
		},
		{
			name: "an invalid go dep output should produce an error",
			goDepOutput: `invalid-dep invalid-dep
github.com/konveyor/analyzer-lsp github.com/antchfx/xmlquery@v1.3.12
github.com/konveyor/analyzer-lsp gopkg.in/yaml.v3@v3.0.1`,
			want:    []provider.DepDAGItem{},
			wantErr: true,
		},
		{
			name: "an invalid go dep output should produce an error",
			goDepOutput: `github.com/konveyor/analyzer-lsp github.com/antchfx/jsonquery@v1.3.0
github.com/konveyor/analyzer-lsp github.com/antchfx/xmlquery@v1.3.12
github.com/konveyor/analyzer-lsp gopkg.in/yaml.v3@v3.0.1
github.com/antchfx/jsonquery@v1.3.0 github.com/antchfx/xpath@v1.2.1
github.com/antchfx/jsonquery@v1.3.0 github.com/golang/groupcache@v0.0.0-20200121045136-8c9f03a8e57e
github.com/antchfx/xmlquery@v1.3.12 github.com/antchfx/xpath@v1.2.1
github.com/antchfx/xmlquery@v1.3.12 github.com/golang/groupcache@v0.0.0-20200121045136-8c9f03a8e57e`,
			want: []provider.DepDAGItem{
				{
					Dep: provider.Dep{
						Name:    "gopkg.in/yaml.v3",
						Version: "v3.0.1",
						Labels:  []string{fmt.Sprintf("%v=downloadable", provider.DepSourceLabel)},
					},
					AddedDeps: []provider.DepDAGItem{},
				},
				{
					Dep: provider.Dep{
						Name:    "github.com/antchfx/jsonquery",
						Version: "v1.3.0",
						Labels:  []string{fmt.Sprintf("%v=downloadable", provider.DepSourceLabel)},
					},
					AddedDeps: []provider.DepDAGItem{
						{
							Dep: provider.Dep{
								Name:    "github.com/antchfx/xpath",
								Version: "v1.2.1",
								Labels:  []string{fmt.Sprintf("%v=downloadable", provider.DepSourceLabel)},
							},
						},
						{
							Dep: provider.Dep{
								Name:    "github.com/golang/groupcache",
								Version: "v0.0.0-20200121045136-8c9f03a8e57e",
								Labels:  []string{fmt.Sprintf("%v=downloadable", provider.DepSourceLabel)},
							},
						},
					},
				},
				{
					Dep: provider.Dep{
						Name:    "github.com/antchfx/xmlquery",
						Version: "v1.3.12",
						Labels:  []string{fmt.Sprintf("%v=downloadable", provider.DepSourceLabel)},
					},
					AddedDeps: []provider.DepDAGItem{
						{
							Dep: provider.Dep{
								Name:    "github.com/antchfx/xpath",
								Version: "v1.2.1",
								Labels:  []string{fmt.Sprintf("%v=downloadable", provider.DepSourceLabel)},
							},
						},
						{
							Dep: provider.Dep{
								Name:    "github.com/golang/groupcache",
								Version: "v0.0.0-20200121045136-8c9f03a8e57e",
								Labels:  []string{fmt.Sprintf("%v=downloadable", provider.DepSourceLabel)},
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseGoDepLines(strings.Split(tt.goDepOutput, "\n"))
			if (err != nil) != tt.wantErr {
				t.Errorf("parseGoDepLines() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("expected deps of size: %v, got %v", len(tt.want), got)
			}
			for _, wantedDep := range tt.want {
				found := false
				for _, gotDep := range got {
					if reflect.DeepEqual(wantedDep, gotDep) {
						found = true
					}
				}
				if !found {
					t.Errorf("expected to get get dep: %v, did not find in: %v", wantedDep, got)
				}
			}
		})
	}
}

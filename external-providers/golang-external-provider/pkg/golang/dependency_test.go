package golang

import (
	"reflect"
	"strings"
	"testing"

	dependency "github.com/konveyor/analyzer-lsp/provider"
)

func Test_parseGoDepLines(t *testing.T) {
	tests := []struct {
		name        string
		goDepOutput string
		want        map[dependency.Dep][]dependency.Dep
		wantErr     bool
	}{
		{
			name:        "an empty go dep output shouldn't produce anything",
			goDepOutput: "",
			want:        map[dependency.Dep][]dependency.Dep{},
			wantErr:     false,
		},
		{
			name: "an invalid go dep output should produce an error",
			goDepOutput: `invalid-dep invalid-dep
github.com/konveyor/analyzer-lsp github.com/antchfx/xmlquery@v1.3.12
github.com/konveyor/analyzer-lsp gopkg.in/yaml.v3@v3.0.1`,
			want:    map[dependency.Dep][]dependency.Dep{},
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
			want: map[dependency.Dep][]dependency.Dep{
				{
					Name:    "gopkg.in/yaml.v3",
					Version: "v3.0.1"}: {},
				{
					Name:    "github.com/antchfx/jsonquery",
					Version: "v1.3.0",
				}: {
					{Name: "github.com/antchfx/xpath", Version: "v1.2.1"},
					{Name: "github.com/golang/groupcache", Version: "v0.0.0-20200121045136-8c9f03a8e57e"},
				},
				{
					Name:    "github.com/antchfx/xmlquery",
					Version: "v1.3.12",
				}: {
					{Name: "github.com/antchfx/xpath", Version: "v1.2.1"},
					{Name: "github.com/golang/groupcache", Version: "v0.0.0-20200121045136-8c9f03a8e57e"},
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
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseGoDepLines() = %v, want %v", got, tt.want)
			}
		})
	}
}

package provider

import (
	"reflect"
	"testing"
)

func TestWithProviderSpecificDefaults(t *testing.T) {
	defaults := func() map[string]interface{} {
		return map[string]interface{}{
			LspServerPathConfigKey: "/usr/local/bin/server",
			"lspServerName":        "lang",
			"lspServerArgs":        []interface{}{"--stdio"},
		}
	}
	tests := []struct {
		name string
		in   map[string]interface{}
		want map[string]interface{}
	}{
		{
			name: "nil config gets every default",
			in:   nil,
			want: defaults(),
		},
		{
			name: "empty config gets every default",
			in:   map[string]interface{}{},
			want: defaults(),
		},
		{
			name: "partial config keeps its values and gains the rest",
			in:   map[string]interface{}{"lspServerName": "custom"},
			want: map[string]interface{}{
				LspServerPathConfigKey: "/usr/local/bin/server",
				"lspServerName":        "custom",
				"lspServerArgs":        []interface{}{"--stdio"},
			},
		},
		{
			name: "full config is left alone",
			in: map[string]interface{}{
				LspServerPathConfigKey: "/opt/server",
				"lspServerName":        "custom",
				"lspServerArgs":        []interface{}{},
			},
			want: map[string]interface{}{
				LspServerPathConfigKey: "/opt/server",
				"lspServerName":        "custom",
				"lspServerArgs":        []interface{}{},
			},
		},
		{
			name: "empty string and nil count as unset",
			in: map[string]interface{}{
				LspServerPathConfigKey: "",
				"lspServerName":        nil,
			},
			want: defaults(),
		},
		{
			name: "keys without a default pass through",
			in:   map[string]interface{}{"includedPaths": []interface{}{"src"}},
			want: map[string]interface{}{
				LspServerPathConfigKey: "/usr/local/bin/server",
				"lspServerName":        "lang",
				"lspServerArgs":        []interface{}{"--stdio"},
				"includedPaths":        []interface{}{"src"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WithProviderSpecificDefaults(InitConfig{ProviderSpecificConfig: tt.in}, defaults())
			if !reflect.DeepEqual(got.ProviderSpecificConfig, tt.want) {
				t.Errorf("got %v, want %v", got.ProviderSpecificConfig, tt.want)
			}
		})
	}
}

func TestWithProviderSpecificDefaultsDoesNotMutateInput(t *testing.T) {
	in := map[string]interface{}{"lspServerName": "custom"}
	c := InitConfig{Location: "/src", ProviderSpecificConfig: in}
	got := WithProviderSpecificDefaults(c, map[string]interface{}{LspServerPathConfigKey: "/x"})

	if _, ok := in[LspServerPathConfigKey]; ok {
		t.Errorf("caller map was modified: %v", in)
	}
	if got.Location != "/src" {
		t.Errorf("other fields must be preserved, got location %q", got.Location)
	}
}

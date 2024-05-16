package builtin

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/konveyor/analyzer-lsp/provider"
)

func newLocationForLine(line float64) provider.Location {
	return provider.Location{
		StartPosition: provider.Position{
			Line: line,
		},
		EndPosition: provider.Position{
			Line: line,
		},
	}
}

func Test_builtinServiceClient_getLocation(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		content string
		want    provider.Location
		wantErr bool
	}{
		{
			name:    "search for innerText with leading and trailing newlines",
			path:    "./testdata/pom.xml",
			content: "\n\t\t\tch.qos.logback\n\t\t\tlogback-classic\n\t\t\t1.1.7\n\t\t",
			want:    newLocationForLine(117),
		},
		{
			name:    "search for innerText with spaces in between content",
			path:    "./testdata/pom.xml",
			content: "\n\t<version>0.0.1-SNAPSHOT</version>\n\n\n\t\t<name>Order Management</name>\n\t\t<packaging>war</packaging>\n",
			want:    newLocationForLine(6),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &builtinServiceClient{
				log:           testr.New(t),
				cacheMutex:    sync.RWMutex{},
				locationCache: make(map[string]float64),
			}
			got, err := b.getLocation(context.TODO(), tt.path, tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("builtinServiceClient.getLocation() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("builtinServiceClient.getLocation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_builtinServiceClient_filterByIncludedPaths(t *testing.T) {
	tests := []struct {
		name          string
		inputPaths    []string
		includedPaths []string
		want          []string
	}{
		{
			name:          "no included paths given, match all",
			inputPaths:    []string{"/test/a/b/", "/test/a/c/"},
			includedPaths: []string{},
			want:          []string{"/test/a/b/", "/test/a/c/"},
		},
		{
			name:          "included file path doesn't match",
			inputPaths:    []string{"/test/a/b/file.py"},
			includedPaths: []string{"/test/a/c/file.py"},
			want:          []string{},
		},
		{
			name:          "included file path matches",
			inputPaths:    []string{"/test/a/b/file.py"},
			includedPaths: []string{"/test/a/b/file.py"},
			want:          []string{"/test/a/b/file.py"},
		},
		{
			name:          "input dir path is equal to included path",
			inputPaths:    []string{"/test/a/b/"},
			includedPaths: []string{"////test/a/b//"},
			want:          []string{"/test/a/b"},
		},
		{
			name:          "input dir path is a sub-tree of included path",
			inputPaths:    []string{"/test/a/b/c/d/", "///test/a/b/c/e/file.java"},
			includedPaths: []string{"////test/a/b//"},
			want:          []string{"/test/a/b/c/d", "/test/a/b/c/e/file.java"},
		},
		{
			name:          "input dir path is not equal to included path and is not a sub-tree",
			inputPaths:    []string{"/test/a/b/c/d/", "///test/a/b/c/e/file.java", "/test/a/d/e/f/"},
			includedPaths: []string{"////test/a/d//"},
			want:          []string{"/test/a/d/e/f"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &builtinServiceClient{
				config: provider.InitConfig{
					ProviderSpecificConfig: map[string]interface{}{
						"includedPaths": tt.includedPaths,
					},
				},
				includedPaths: tt.includedPaths,
				log:           testr.New(t),
			}
			if got := b.filterByIncludedPaths(tt.inputPaths); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("builtinServiceClient.filterByIncludedPaths() = %v, want %v", got, tt.want)
			}
		})
	}
}

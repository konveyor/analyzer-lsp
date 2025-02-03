package builtin

import (
	"context"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/konveyor/analyzer-lsp/engine"
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
		inputPath     string
		includedPaths []string
		want          bool
	}{
		{
			name:          "no included paths given, match all",
			inputPath:     "/test/a/b",
			includedPaths: []string{},
			want:          true,
		},
		{
			name:          "included file path doesn't match",
			inputPath:     "/test/a/b/file.py",
			includedPaths: []string{"/test/a/c/file.py"},
			want:          false,
		},
		{
			name:          "included file path matches",
			inputPath:     "/test/a/b/file.py",
			includedPaths: []string{"/test/a/b/file.py"},
			want:          true,
		},
		{
			name:          "input dir path is equivalent to included paths",
			inputPath:     "/test/a/b/",
			includedPaths: []string{"////test/a/b//"},
			want:          true,
		},
		{
			name:          "input dir path is a sub-tree of included path",
			inputPath:     "///test/a/b/c/e/",
			includedPaths: []string{"////test/a/b//"},
			want:          true,
		},
		{
			name:          "input dir path is not equal to included path and is not a sub-tree",
			inputPath:     "///test/a/b/c/e/file.java",
			includedPaths: []string{"////test/a/d//"},
			want:          false,
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
			if got := b.isFileIncluded(tt.inputPath); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("builtinServiceClient.filterByIncludedPaths() = %v, want %v", got, tt.want)
			}
		})
	}
}


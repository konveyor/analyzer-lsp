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

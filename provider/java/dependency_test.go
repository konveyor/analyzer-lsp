package java

import (
	"reflect"
	"strings"
	"testing"

	"github.com/konveyor/analyzer-lsp/dependency/dependency"
)

func Test_parseMavenDepLines(t *testing.T) {
	tests := []struct {
		name        string
		mavenOutput string
		wantDeps    map[dependency.Dep][]dependency.Dep
		wantErr     bool
	}{
		{
			name:        "an empty maven output should not return any dependencies",
			mavenOutput: "",
			wantDeps:    map[dependency.Dep][]dependency.Dep{},
			wantErr:     false,
		},
		{
			name: "an invalid maven output should return an error",
			mavenOutput: `com.example.apps:java:jar:1.0-SNAPSHOT
+- invalid maven output
|  \- invalid dep
+- invalid dep
|  +- invalid dep`,
			wantDeps: map[dependency.Dep][]dependency.Dep{},
			wantErr:  true,
		},
		{
			name: "a valid maven dependency graph must be parsed without errors",
			mavenOutput: `com.example.apps:java:jar:1.0-SNAPSHOT
+- junit:junit:jar:4.11:test
|  \- org.hamcrest:hamcrest-core:jar:1.3:test
+- io.fabric8:kubernetes-client:jar:6.0.0:compile
|  +- io.fabric8:kubernetes-httpclient-okhttp:jar:6.0.0:runtime
|  |  +- com.squareup.okhttp3:okhttp:jar:3.12.12:runtime
|  |  |  \- com.squareup.okio:okio:jar:1.15.0:runtime
|  |  \- com.squareup.okhttp3:logging-interceptor:jar:3.12.12:runtime
|  \- io.fabric8:zjsonpatch:jar:0.3.0:compile`,
			wantDeps: map[dependency.Dep][]dependency.Dep{
				{
					Name:     "junit.junit",
					Version:  "4.11",
					Location: "test",
					Indirect: false,
				}: {
					{
						Name:     "org.hamcrest.hamcrest-core",
						Version:  "1.3",
						Location: "test",
						Indirect: true,
					},
				},
				{
					Name:     "io.fabric8.kubernetes-client",
					Version:  "6.0.0",
					Location: "compile",
					Indirect: false,
				}: {
					{
						Name:     "io.fabric8.kubernetes-httpclient-okhttp",
						Version:  "6.0.0",
						Location: "runtime",
						Indirect: true,
					},
					{
						Name:     "com.squareup.okhttp3.okhttp",
						Version:  "3.12.12",
						Location: "runtime",
						Indirect: true,
					},
					{
						Name:     "com.squareup.okio.okio",
						Version:  "1.15.0",
						Location: "runtime",
						Indirect: true,
					},
					{
						Name:     "com.squareup.okhttp3.logging-interceptor",
						Version:  "3.12.12",
						Location: "runtime",
						Indirect: true,
					},
					{
						Name:     "io.fabric8.zjsonpatch",
						Version:  "0.3.0",
						Location: "compile",
						Indirect: true,
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := strings.Split(tt.mavenOutput, "\n")
			deps := map[dependency.Dep][]dependency.Dep{}
			if err := parseMavenDepLines(lines[1:], deps); (err != nil) != tt.wantErr {
				t.Errorf("parseMavenDepLines() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(tt.wantDeps, deps) {
				t.Errorf("parseMavenDepLines() want %v got %v", tt.wantDeps, deps)
			}
		})
	}
}

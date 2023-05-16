package java

import (
	"reflect"
	"strings"
	"testing"

	"github.com/konveyor/analyzer-lsp/provider"
)

func Test_parseMavenDepLines(t *testing.T) {
	tests := []struct {
		name        string
		mavenOutput string
		wantDeps    map[provider.Dep][]provider.Dep
		wantErr     bool
	}{
		{
			name:        "an empty maven output should not return any dependencies",
			mavenOutput: "",
			wantDeps:    map[provider.Dep][]provider.Dep{},
			wantErr:     false,
		},
		{
			name: "an invalid maven output should return an error",
			mavenOutput: `com.example.apps:java:jar:1.0-SNAPSHOT
+- invalid maven output
|  \- invalid dep
+- invalid dep
|  +- invalid dep`,
			wantDeps: map[provider.Dep][]provider.Dep{},
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
			wantDeps: map[provider.Dep][]provider.Dep{
				{
					Name:     "junit.junit",
					Version:  "4.11",
					Type:     "test",
					Indirect: false,
					SHA:      "4e031bb61df09069aeb2bffb4019e7a5034a4ee0",
				}: {
					{
						Name:     "org.hamcrest.hamcrest-core",
						Version:  "1.3",
						Type:     "test",
						Indirect: true,
						SHA:      "42a25dc3219429f0e5d060061f71acb49bf010a0",
					},
				},
				{
					Name:     "io.fabric8.kubernetes-client",
					Version:  "6.0.0",
					Type:     "compile",
					Indirect: false,
					SHA:      "d0831d44e12313df8989fc1d4a9c90452f08858e",
				}: {
					{
						Name:     "io.fabric8.kubernetes-httpclient-okhttp",
						Version:  "6.0.0",
						Type:     "runtime",
						Indirect: true,
						SHA:      "70690b98acb07a809c55d15d7cf45f53ec1026e1",
					},
					{
						Name:     "com.squareup.okhttp3.okhttp",
						Version:  "3.12.12",
						Type:     "runtime",
						Indirect: true,
						SHA:      "d3e1ce1d2b3119adf270b2d00d947beb03fe3321",
					},
					{
						Name:     "com.squareup.okio.okio",
						Version:  "1.15.0",
						Type:     "runtime",
						Indirect: true,
						SHA:      "bc28b5a964c8f5721eb58ee3f3c47a9bcbf4f4d8",
					},
					{
						Name:     "com.squareup.okhttp3.logging-interceptor",
						Version:  "3.12.12",
						Type:     "runtime",
						Indirect: true,
						SHA:      "d952189f6abb148ff72aab246aa8c28cf99b469f",
					},
					{
						Name:     "io.fabric8.zjsonpatch",
						Version:  "0.3.0",
						Type:     "compile",
						Indirect: true,
						SHA:      "d3ebf0f291297649b4c8dc3ecc81d2eddedc100d",
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := strings.Split(tt.mavenOutput, "\n")
			deps := map[provider.Dep][]provider.Dep{}
			if err := parseMavenDepLines(lines[1:], deps, "testdata"); (err != nil) != tt.wantErr {
				t.Errorf("parseMavenDepLines() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(tt.wantDeps, deps) {
				t.Errorf("parseMavenDepLines() want %v got %v", tt.wantDeps, deps)
			}
		})
	}
}

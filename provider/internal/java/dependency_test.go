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
		wantDeps    []provider.DepDAGItem
		wantErr     bool
	}{
		{
			name:        "an empty maven output should not return any dependencies",
			mavenOutput: "",
			wantDeps:    []provider.DepDAGItem{},
			wantErr:     false,
		},
		{
			name: "an invalid maven output should return an error",
			mavenOutput: `com.example.apps:java:jar:1.0-SNAPSHOT
+- invalid maven output
|  \- invalid dep
+- invalid dep
|  +- invalid dep`,
			wantDeps: nil,
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
			wantDeps: []provider.DepDAGItem{
				{
					Dep: provider.Dep{
						Name:               "junit.junit",
						Version:            "4.11",
						Type:               "test",
						Indirect:           false,
						ResolvedIdentifier: "4e031bb61df09069aeb2bffb4019e7a5034a4ee0",
					},
					AddedDeps: []provider.DepDAGItem{
						{
							Dep: provider.Dep{
								Name:               "org.hamcrest.hamcrest-core",
								Version:            "1.3",
								Type:               "test",
								Indirect:           true,
								ResolvedIdentifier: "42a25dc3219429f0e5d060061f71acb49bf010a0",
							},
						},
					},
				},
				{
					Dep: provider.Dep{
						Name:               "io.fabric8.kubernetes-client",
						Version:            "6.0.0",
						Type:               "compile",
						Indirect:           false,
						ResolvedIdentifier: "d0831d44e12313df8989fc1d4a9c90452f08858e",
					},
					AddedDeps: []provider.DepDAGItem{
						{
							Dep: provider.Dep{
								Name:               "io.fabric8.kubernetes-httpclient-okhttp",
								Version:            "6.0.0",
								Type:               "runtime",
								Indirect:           true,
								ResolvedIdentifier: "70690b98acb07a809c55d15d7cf45f53ec1026e1",
							},
						},
						{
							Dep: provider.Dep{
								Name:               "com.squareup.okhttp3.okhttp",
								Version:            "3.12.12",
								Type:               "runtime",
								Indirect:           true,
								ResolvedIdentifier: "d3e1ce1d2b3119adf270b2d00d947beb03fe3321",
							},
						},
						{
							Dep: provider.Dep{
								Name:               "com.squareup.okio.okio",
								Version:            "1.15.0",
								Type:               "runtime",
								Indirect:           true,
								ResolvedIdentifier: "bc28b5a964c8f5721eb58ee3f3c47a9bcbf4f4d8",
							},
						},
						{
							Dep: provider.Dep{
								Name:               "com.squareup.okhttp3.logging-interceptor",
								Version:            "3.12.12",
								Type:               "runtime",
								Indirect:           true,
								ResolvedIdentifier: "d952189f6abb148ff72aab246aa8c28cf99b469f",
							},
						},
						{
							Dep: provider.Dep{
								Name:               "io.fabric8.zjsonpatch",
								Version:            "0.3.0",
								Type:               "compile",
								Indirect:           true,
								ResolvedIdentifier: "d3ebf0f291297649b4c8dc3ecc81d2eddedc100d",
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
			lines := strings.Split(tt.mavenOutput, "\n")
			deps := []provider.DepDAGItem{}
			var err error
			if deps, err = parseMavenDepLines(lines[1:], "testdata"); (err != nil) != tt.wantErr {
				t.Errorf("parseMavenDepLines() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(tt.wantDeps) != len(deps) {
				t.Errorf("expected wanted deps of size: %v, got: %v", len(tt.wantDeps), deps)
			}
			for _, wantedDep := range tt.wantDeps {
				found := false
				for _, gotDep := range deps {
					if reflect.DeepEqual(wantedDep, gotDep) {
						found = true
					}
				}
				if !found {
					t.Errorf("Unable to find wanted dep: %v", wantedDep)
				}
			}
		})
	}
}

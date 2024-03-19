package java

import (
	"reflect"
	"strings"
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/konveyor/analyzer-lsp/engine/labels"
	"github.com/konveyor/analyzer-lsp/provider"
)

func Test_parseMavenDepLines(t *testing.T) {
	tests := []struct {
		name                string
		mavenOutput         string
		wantDeps            []provider.DepDAGItem
		excludedPackages    []string
		openSourceLabelPath string
		wantErr             bool
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
|  +- io.netty:netty-transport-native-epoll:jar:linux-aarch_64:4.1.76.Final:runtime
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
						Labels: []string{
							labels.AsString(provider.DepSourceLabel, "internal"),
							labels.AsString(provider.DepLanguageLabel, "java"),
						},
						Extras: map[string]interface{}{
							groupIdKey:    "junit",
							artifactIdKey: "junit",
							pomPathKey:    "pom.xml",
						},
						FileURIPrefix: "file://testdata/junit/junit/4.11",
					},
					AddedDeps: []provider.DepDAGItem{
						{
							Dep: provider.Dep{
								Name:               "org.hamcrest.hamcrest-core",
								Version:            "1.3",
								Type:               "test",
								Indirect:           true,
								ResolvedIdentifier: "42a25dc3219429f0e5d060061f71acb49bf010a0",
								Labels: []string{
									labels.AsString(provider.DepSourceLabel, "internal"),
									labels.AsString(provider.DepLanguageLabel, "java"),
								},
								Extras: map[string]interface{}{
									groupIdKey:    "org.hamcrest",
									artifactIdKey: "hamcrest-core",
									pomPathKey:    "pom.xml",
									baseDepKey:    provider.Dep{
										Name:               "junit.junit",
										Version:            "4.11",
										Type:               "test",
										Indirect:           false,
										ResolvedIdentifier: "4e031bb61df09069aeb2bffb4019e7a5034a4ee0",
										Labels: []string{
											labels.AsString(provider.DepSourceLabel, "internal"),
											labels.AsString(provider.DepLanguageLabel, "java"),
										},
										Extras: map[string]interface{}{
											groupIdKey:    "junit",
											artifactIdKey: "junit",
											pomPathKey:    "pom.xml",
										},
										FileURIPrefix: "file://testdata/junit/junit/4.11",
									},
								},
								FileURIPrefix: "file://testdata/org/hamcrest/hamcrest-core/1.3",
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
						Labels: []string{
							labels.AsString(provider.DepSourceLabel, "internal"),
							labels.AsString(provider.DepLanguageLabel, "java"),
						},
						Extras: map[string]interface{}{
							groupIdKey:    "io.fabric8",
							artifactIdKey: "kubernetes-client",
							pomPathKey:    "pom.xml",
						},
						FileURIPrefix: "file://testdata/io/fabric8/kubernetes-client/6.0.0",
					},
					AddedDeps: []provider.DepDAGItem{
						{
							Dep: provider.Dep{
								Name:               "io.netty.netty-transport-native-epoll",
								Version:            "4.1.76.Final",
								Type:               "runtime",
								Classifier:         "linux-aarch_64",
								Indirect:           true,
								ResolvedIdentifier: "e1ee2a9c5f63b1b71260caf127a1e50667d62854",
								Labels: []string{
									labels.AsString(provider.DepSourceLabel, "internal"),
									labels.AsString(provider.DepLanguageLabel, "java"),
								},
								Extras: map[string]interface{}{
									groupIdKey:    "io.netty",
									artifactIdKey: "netty-transport-native-epoll",
									pomPathKey:    "pom.xml",
									baseDepKey:    provider.Dep{
										Name:               "io.fabric8.kubernetes-client",
										Version:            "6.0.0",
										Type:               "compile",
										Indirect:           false,
										ResolvedIdentifier: "d0831d44e12313df8989fc1d4a9c90452f08858e",
										Labels: []string{
											labels.AsString(provider.DepSourceLabel, "internal"),
											labels.AsString(provider.DepLanguageLabel, "java"),
										},
										Extras: map[string]interface{}{
											groupIdKey:    "io.fabric8",
											artifactIdKey: "kubernetes-client",
											pomPathKey:    "pom.xml",
										},
										FileURIPrefix: "file://testdata/io/fabric8/kubernetes-client/6.0.0",
									},
								},
								FileURIPrefix: "file://testdata/io/netty/netty-transport-native-epoll/4.1.76.Final",
							},
						},
						{
							Dep: provider.Dep{
								Name:               "io.fabric8.kubernetes-httpclient-okhttp",
								Version:            "6.0.0",
								Type:               "runtime",
								Indirect:           true,
								ResolvedIdentifier: "70690b98acb07a809c55d15d7cf45f53ec1026e1",
								Labels: []string{
									labels.AsString(provider.DepSourceLabel, "internal"),
									labels.AsString(provider.DepLanguageLabel, "java"),
								},
								Extras: map[string]interface{}{
									groupIdKey:    "io.fabric8",
									artifactIdKey: "kubernetes-httpclient-okhttp",
									pomPathKey:    "pom.xml",
									baseDepKey:    provider.Dep{
										Name:               "io.fabric8.kubernetes-client",
										Version:            "6.0.0",
										Type:               "compile",
										Indirect:           false,
										ResolvedIdentifier: "d0831d44e12313df8989fc1d4a9c90452f08858e",
										Labels: []string{
											labels.AsString(provider.DepSourceLabel, "internal"),
											labels.AsString(provider.DepLanguageLabel, "java"),
										},
										Extras: map[string]interface{}{
											groupIdKey:    "io.fabric8",
											artifactIdKey: "kubernetes-client",
											pomPathKey:    "pom.xml",
										},
										FileURIPrefix: "file://testdata/io/fabric8/kubernetes-client/6.0.0",
									},
								},
								FileURIPrefix: "file://testdata/io/fabric8/kubernetes-httpclient-okhttp/6.0.0",
							},
						},
						{
							Dep: provider.Dep{
								Name:               "com.squareup.okhttp3.okhttp",
								Version:            "3.12.12",
								Type:               "runtime",
								Indirect:           true,
								ResolvedIdentifier: "d3e1ce1d2b3119adf270b2d00d947beb03fe3321",
								Labels: []string{
									labels.AsString(provider.DepSourceLabel, "internal"),
									labels.AsString(provider.DepLanguageLabel, "java"),
								},
								Extras: map[string]interface{}{
									groupIdKey:    "com.squareup.okhttp3",
									artifactIdKey: "okhttp",
									pomPathKey:    "pom.xml",
									baseDepKey:    provider.Dep{
										Name:               "io.fabric8.kubernetes-client",
										Version:            "6.0.0",
										Type:               "compile",
										Indirect:           false,
										ResolvedIdentifier: "d0831d44e12313df8989fc1d4a9c90452f08858e",
										Labels: []string{
											labels.AsString(provider.DepSourceLabel, "internal"),
											labels.AsString(provider.DepLanguageLabel, "java"),
										},
										Extras: map[string]interface{}{
											groupIdKey:    "io.fabric8",
											artifactIdKey: "kubernetes-client",
											pomPathKey:    "pom.xml",
										},
										FileURIPrefix: "file://testdata/io/fabric8/kubernetes-client/6.0.0",
									},
								},
								FileURIPrefix: "file://testdata/com/squareup/okhttp3/okhttp/3.12.12",
							},
						},
						{
							Dep: provider.Dep{
								Name:               "com.squareup.okio.okio",
								Version:            "1.15.0",
								Type:               "runtime",
								Indirect:           true,
								ResolvedIdentifier: "bc28b5a964c8f5721eb58ee3f3c47a9bcbf4f4d8",
								Labels: []string{
									labels.AsString(provider.DepSourceLabel, "internal"),
									labels.AsString(provider.DepLanguageLabel, "java"),
								},
								Extras: map[string]interface{}{
									groupIdKey:    "com.squareup.okio",
									artifactIdKey: "okio",
									pomPathKey:    "pom.xml",
									baseDepKey:    provider.Dep{
										Name:               "io.fabric8.kubernetes-client",
										Version:            "6.0.0",
										Type:               "compile",
										Indirect:           false,
										ResolvedIdentifier: "d0831d44e12313df8989fc1d4a9c90452f08858e",
										Labels: []string{
											labels.AsString(provider.DepSourceLabel, "internal"),
											labels.AsString(provider.DepLanguageLabel, "java"),
										},
										Extras: map[string]interface{}{
											groupIdKey:    "io.fabric8",
											artifactIdKey: "kubernetes-client",
											pomPathKey:    "pom.xml",
										},
										FileURIPrefix: "file://testdata/io/fabric8/kubernetes-client/6.0.0",
									},
								},
								FileURIPrefix: "file://testdata/com/squareup/okio/okio/1.15.0",
							},
						},
						{
							Dep: provider.Dep{
								Name:               "com.squareup.okhttp3.logging-interceptor",
								Version:            "3.12.12",
								Type:               "runtime",
								Indirect:           true,
								ResolvedIdentifier: "d952189f6abb148ff72aab246aa8c28cf99b469f",
								Labels: []string{
									labels.AsString(provider.DepSourceLabel, "internal"),
									labels.AsString(provider.DepLanguageLabel, "java"),
								},
								Extras: map[string]interface{}{
									groupIdKey:    "com.squareup.okhttp3",
									artifactIdKey: "logging-interceptor",
									pomPathKey:    "pom.xml",
									baseDepKey:    provider.Dep{
										Name:               "io.fabric8.kubernetes-client",
										Version:            "6.0.0",
										Type:               "compile",
										Indirect:           false,
										ResolvedIdentifier: "d0831d44e12313df8989fc1d4a9c90452f08858e",
										Labels: []string{
											labels.AsString(provider.DepSourceLabel, "internal"),
											labels.AsString(provider.DepLanguageLabel, "java"),
										},
										Extras: map[string]interface{}{
											groupIdKey:    "io.fabric8",
											artifactIdKey: "kubernetes-client",
											pomPathKey:    "pom.xml",
										},
										FileURIPrefix: "file://testdata/io/fabric8/kubernetes-client/6.0.0",
									},
								},
								FileURIPrefix: "file://testdata/com/squareup/okhttp3/logging-interceptor/3.12.12",
							},
						},
						{
							Dep: provider.Dep{
								Name:               "io.fabric8.zjsonpatch",
								Version:            "0.3.0",
								Type:               "compile",
								Indirect:           true,
								ResolvedIdentifier: "d3ebf0f291297649b4c8dc3ecc81d2eddedc100d",
								Labels: []string{
									labels.AsString(provider.DepSourceLabel, "internal"),
									labels.AsString(provider.DepLanguageLabel, "java"),
								},
								Extras: map[string]interface{}{
									groupIdKey:    "io.fabric8",
									artifactIdKey: "zjsonpatch",
									pomPathKey:    "pom.xml",
									baseDepKey:    provider.Dep{
										Name:               "io.fabric8.kubernetes-client",
										Version:            "6.0.0",
										Type:               "compile",
										Indirect:           false,
										ResolvedIdentifier: "d0831d44e12313df8989fc1d4a9c90452f08858e",
										Labels: []string{
											labels.AsString(provider.DepSourceLabel, "internal"),
											labels.AsString(provider.DepLanguageLabel, "java"),
										},
										Extras: map[string]interface{}{
											groupIdKey:    "io.fabric8",
											artifactIdKey: "kubernetes-client",
											pomPathKey:    "pom.xml",
										},
										FileURIPrefix: "file://testdata/io/fabric8/kubernetes-client/6.0.0",
									},
								},
								FileURIPrefix: "file://testdata/io/fabric8/zjsonpatch/0.3.0",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "test opensource and exclude labels",
			mavenOutput: `com.example.apps:java:jar:1.0-SNAPSHOT
+- junit:junit:jar:4.11:test
|  \- org.hamcrest:hamcrest-core:jar:1.3:test
+- io.fabric8:kubernetes-client:jar:6.0.0:compile
|  +- io.fabric8:kubernetes-httpclient-okhttp:jar:6.0.0:runtime
|  |  +- com.squareup.okhttp3:okhttp:jar:3.12.12:runtime
|  |  |  \- com.squareup.okio:okio:jar:1.15.0:runtime
|  |  \- com.squareup.okhttp3:logging-interceptor:jar:3.12.12:runtime
|  \- io.fabric8:zjsonpatch:jar:0.3.0:compile`,
			openSourceLabelPath: "./testdata/open_source_packages",
			excludedPackages: []string{
				"org.hamcrest.*",
			},
			wantDeps: []provider.DepDAGItem{
				{
					Dep: provider.Dep{
						Name:               "junit.junit",
						Version:            "4.11",
						Type:               "test",
						Indirect:           false,
						ResolvedIdentifier: "4e031bb61df09069aeb2bffb4019e7a5034a4ee0",
						Labels: []string{
							labels.AsString(provider.DepSourceLabel, "open-source"),
							labels.AsString(provider.DepLanguageLabel, "java"),
						},
						Extras: map[string]interface{}{
							groupIdKey:    "junit",
							artifactIdKey: "junit",
							pomPathKey:    "pom.xml",
						},
						FileURIPrefix: "file://testdata/junit/junit/4.11",
					},
					AddedDeps: []provider.DepDAGItem{
						{
							Dep: provider.Dep{
								Name:               "org.hamcrest.hamcrest-core",
								Version:            "1.3",
								Type:               "test",
								Indirect:           true,
								ResolvedIdentifier: "42a25dc3219429f0e5d060061f71acb49bf010a0",
								Labels: []string{
									labels.AsString(provider.DepExcludeLabel, ""),
									labels.AsString(provider.DepSourceLabel, "internal"),
									labels.AsString(provider.DepLanguageLabel, "java"),
								},
								Extras: map[string]interface{}{
									groupIdKey:    "org.hamcrest",
									artifactIdKey: "hamcrest-core",
									pomPathKey:    "pom.xml",
									baseDepKey:    provider.Dep{
										Name:               "junit.junit",
										Version:            "4.11",
										Type:               "test",
										Indirect:           false,
										ResolvedIdentifier: "4e031bb61df09069aeb2bffb4019e7a5034a4ee0",
										Labels: []string{
											labels.AsString(provider.DepSourceLabel, "open-source"),
											labels.AsString(provider.DepLanguageLabel, "java"),
										},
										Extras: map[string]interface{}{
											groupIdKey:    "junit",
											artifactIdKey: "junit",
											pomPathKey:    "pom.xml",
										},
										FileURIPrefix: "file://testdata/junit/junit/4.11",
									},
								},
								FileURIPrefix: "file://testdata/org/hamcrest/hamcrest-core/1.3",
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
						Labels: []string{
							labels.AsString(provider.DepSourceLabel, "internal"),
							labels.AsString(provider.DepLanguageLabel, "java"),
						},
						Extras: map[string]interface{}{
							groupIdKey:    "io.fabric8",
							artifactIdKey: "kubernetes-client",
							pomPathKey:    "pom.xml",
						},
						FileURIPrefix: "file://testdata/io/fabric8/kubernetes-client/6.0.0",
					},
					AddedDeps: []provider.DepDAGItem{
						{
							Dep: provider.Dep{
								Name:               "io.fabric8.kubernetes-httpclient-okhttp",
								Version:            "6.0.0",
								Type:               "runtime",
								Indirect:           true,
								ResolvedIdentifier: "70690b98acb07a809c55d15d7cf45f53ec1026e1",
								Labels: []string{
									labels.AsString(provider.DepSourceLabel, "internal"),
									labels.AsString(provider.DepLanguageLabel, "java"),
								},
								Extras: map[string]interface{}{
									groupIdKey:    "io.fabric8",
									artifactIdKey: "kubernetes-httpclient-okhttp",
									pomPathKey:    "pom.xml",
									baseDepKey:    provider.Dep{
										Name:               "io.fabric8.kubernetes-client",
										Version:            "6.0.0",
										Type:               "compile",
										Indirect:           false,
										ResolvedIdentifier: "d0831d44e12313df8989fc1d4a9c90452f08858e",
										Labels: []string{
											labels.AsString(provider.DepSourceLabel, "internal"),
											labels.AsString(provider.DepLanguageLabel, "java"),
										},
										Extras: map[string]interface{}{
											groupIdKey:    "io.fabric8",
											artifactIdKey: "kubernetes-client",
											pomPathKey:    "pom.xml",
										},
										FileURIPrefix: "file://testdata/io/fabric8/kubernetes-client/6.0.0",
									},
								},
								FileURIPrefix: "file://testdata/io/fabric8/kubernetes-httpclient-okhttp/6.0.0",
							},
						},
						{
							Dep: provider.Dep{
								Name:               "com.squareup.okhttp3.okhttp",
								Version:            "3.12.12",
								Type:               "runtime",
								Indirect:           true,
								ResolvedIdentifier: "d3e1ce1d2b3119adf270b2d00d947beb03fe3321",
								Labels: []string{
									labels.AsString(provider.DepSourceLabel, "internal"),
									labels.AsString(provider.DepLanguageLabel, "java"),
								},
								Extras: map[string]interface{}{
									groupIdKey:    "com.squareup.okhttp3",
									artifactIdKey: "okhttp",
									pomPathKey:    "pom.xml",
									baseDepKey:    provider.Dep{
										Name:               "io.fabric8.kubernetes-client",
										Version:            "6.0.0",
										Type:               "compile",
										Indirect:           false,
										ResolvedIdentifier: "d0831d44e12313df8989fc1d4a9c90452f08858e",
										Labels: []string{
											labels.AsString(provider.DepSourceLabel, "internal"),
											labels.AsString(provider.DepLanguageLabel, "java"),
										},
										Extras: map[string]interface{}{
											groupIdKey:    "io.fabric8",
											artifactIdKey: "kubernetes-client",
											pomPathKey:    "pom.xml",
										},
										FileURIPrefix: "file://testdata/io/fabric8/kubernetes-client/6.0.0",
									},
								},
								FileURIPrefix: "file://testdata/com/squareup/okhttp3/okhttp/3.12.12",
							},
						},
						{
							Dep: provider.Dep{
								Name:               "com.squareup.okio.okio",
								Version:            "1.15.0",
								Type:               "runtime",
								Indirect:           true,
								ResolvedIdentifier: "bc28b5a964c8f5721eb58ee3f3c47a9bcbf4f4d8",
								Labels: []string{
									labels.AsString(provider.DepSourceLabel, "internal"),
									labels.AsString(provider.DepLanguageLabel, "java"),
								},
								Extras: map[string]interface{}{
									groupIdKey:    "com.squareup.okio",
									artifactIdKey: "okio",
									pomPathKey:    "pom.xml",
									baseDepKey:    provider.Dep{
										Name:               "io.fabric8.kubernetes-client",
										Version:            "6.0.0",
										Type:               "compile",
										Indirect:           false,
										ResolvedIdentifier: "d0831d44e12313df8989fc1d4a9c90452f08858e",
										Labels: []string{
											labels.AsString(provider.DepSourceLabel, "internal"),
											labels.AsString(provider.DepLanguageLabel, "java"),
										},
										Extras: map[string]interface{}{
											groupIdKey:    "io.fabric8",
											artifactIdKey: "kubernetes-client",
											pomPathKey:    "pom.xml",
										},
										FileURIPrefix: "file://testdata/io/fabric8/kubernetes-client/6.0.0",
									},
								},
								FileURIPrefix: "file://testdata/com/squareup/okio/okio/1.15.0",
							},
						},
						{
							Dep: provider.Dep{
								Name:               "com.squareup.okhttp3.logging-interceptor",
								Version:            "3.12.12",
								Type:               "runtime",
								Indirect:           true,
								ResolvedIdentifier: "d952189f6abb148ff72aab246aa8c28cf99b469f",
								Labels: []string{
									labels.AsString(provider.DepSourceLabel, "internal"),
									labels.AsString(provider.DepLanguageLabel, "java"),
								},
								Extras: map[string]interface{}{
									groupIdKey:    "com.squareup.okhttp3",
									artifactIdKey: "logging-interceptor",
									pomPathKey:    "pom.xml",
									baseDepKey:    provider.Dep{
										Name:               "io.fabric8.kubernetes-client",
										Version:            "6.0.0",
										Type:               "compile",
										Indirect:           false,
										ResolvedIdentifier: "d0831d44e12313df8989fc1d4a9c90452f08858e",
										Labels: []string{
											labels.AsString(provider.DepSourceLabel, "internal"),
											labels.AsString(provider.DepLanguageLabel, "java"),
										},
										Extras: map[string]interface{}{
											groupIdKey:    "io.fabric8",
											artifactIdKey: "kubernetes-client",
											pomPathKey:    "pom.xml",
										},
										FileURIPrefix: "file://testdata/io/fabric8/kubernetes-client/6.0.0",
									},
								},
								FileURIPrefix: "file://testdata/com/squareup/okhttp3/logging-interceptor/3.12.12",
							},
						},
						{
							Dep: provider.Dep{
								Name:               "io.fabric8.zjsonpatch",
								Version:            "0.3.0",
								Type:               "compile",
								Indirect:           true,
								ResolvedIdentifier: "d3ebf0f291297649b4c8dc3ecc81d2eddedc100d",
								Labels: []string{
									labels.AsString(provider.DepSourceLabel, "internal"),
									labels.AsString(provider.DepLanguageLabel, "java"),
								},
								Extras: map[string]interface{}{
									groupIdKey:    "io.fabric8",
									artifactIdKey: "zjsonpatch",
									pomPathKey:    "pom.xml",
									baseDepKey:    provider.Dep{
										Name:               "io.fabric8.kubernetes-client",
										Version:            "6.0.0",
										Type:               "compile",
										Indirect:           false,
										ResolvedIdentifier: "d0831d44e12313df8989fc1d4a9c90452f08858e",
										Labels: []string{
											labels.AsString(provider.DepSourceLabel, "internal"),
											labels.AsString(provider.DepLanguageLabel, "java"),
										},
										Extras: map[string]interface{}{
											groupIdKey:    "io.fabric8",
											artifactIdKey: "kubernetes-client",
											pomPathKey:    "pom.xml",
										},
										FileURIPrefix: "file://testdata/io/fabric8/kubernetes-client/6.0.0",
									},
								},
								FileURIPrefix: "file://testdata/io/fabric8/zjsonpatch/0.3.0",
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
			p := javaServiceClient{
				log:         testr.New(t),
				depToLabels: map[string]*depLabelItem{},
				config: provider.InitConfig{
					ProviderSpecificConfig: map[string]interface{}{
						"excludePackages": tt.excludedPackages,
					},
				},
			}
			if tt.openSourceLabelPath != "" {
				p.config.ProviderSpecificConfig["depOpenSourceLabelsFile"] = tt.openSourceLabelPath
			}
			// we are not testing dep init here, so ignore error
			p.depInit()
			if deps, err = p.parseMavenDepLines(lines[1:], "testdata", "pom.xml"); (err != nil) != tt.wantErr {
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

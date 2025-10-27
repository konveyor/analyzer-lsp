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
									baseDepKey: map[string]interface{}{
										"name":    "junit.junit",
										"version": "4.11",
										"extras": map[string]interface{}{
											groupIdKey:    "junit",
											artifactIdKey: "junit",
											pomPathKey:    "pom.xml",
										},
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
									baseDepKey: map[string]interface{}{
										"name":    "io.fabric8.kubernetes-client",
										"version": "6.0.0",
										"extras": map[string]interface{}{
											groupIdKey:    "io.fabric8",
											artifactIdKey: "kubernetes-client",
											pomPathKey:    "pom.xml",
										},
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
									baseDepKey: map[string]interface{}{
										"name":    "io.fabric8.kubernetes-client",
										"version": "6.0.0",
										"extras": map[string]interface{}{
											groupIdKey:    "io.fabric8",
											artifactIdKey: "kubernetes-client",
											pomPathKey:    "pom.xml",
										},
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
									baseDepKey: map[string]interface{}{
										"name":    "io.fabric8.kubernetes-client",
										"version": "6.0.0",
										"extras": map[string]interface{}{
											groupIdKey:    "io.fabric8",
											artifactIdKey: "kubernetes-client",
											pomPathKey:    "pom.xml",
										},
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
									baseDepKey: map[string]interface{}{
										"name":    "io.fabric8.kubernetes-client",
										"version": "6.0.0",
										"extras": map[string]interface{}{
											groupIdKey:    "io.fabric8",
											artifactIdKey: "kubernetes-client",
											pomPathKey:    "pom.xml",
										},
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
									baseDepKey: map[string]interface{}{
										"name":    "io.fabric8.kubernetes-client",
										"version": "6.0.0",
										"extras": map[string]interface{}{
											groupIdKey:    "io.fabric8",
											artifactIdKey: "kubernetes-client",
											pomPathKey:    "pom.xml",
										},
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
									baseDepKey: map[string]interface{}{
										"name":    "io.fabric8.kubernetes-client",
										"version": "6.0.0",
										"extras": map[string]interface{}{
											groupIdKey:    "io.fabric8",
											artifactIdKey: "kubernetes-client",
											pomPathKey:    "pom.xml",
										},
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
									baseDepKey: map[string]interface{}{
										"name":    "junit.junit",
										"version": "4.11",
										"extras": map[string]interface{}{
											groupIdKey:    "junit",
											artifactIdKey: "junit",
											pomPathKey:    "pom.xml",
										},
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
									baseDepKey: map[string]interface{}{
										"name":    "io.fabric8.kubernetes-client",
										"version": "6.0.0",
										"extras": map[string]interface{}{
											groupIdKey:    "io.fabric8",
											artifactIdKey: "kubernetes-client",
											pomPathKey:    "pom.xml",
										},
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
									baseDepKey: map[string]interface{}{
										"name":    "io.fabric8.kubernetes-client",
										"version": "6.0.0",
										"extras": map[string]interface{}{
											groupIdKey:    "io.fabric8",
											artifactIdKey: "kubernetes-client",
											pomPathKey:    "pom.xml",
										},
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
									baseDepKey: map[string]interface{}{
										"name":    "io.fabric8.kubernetes-client",
										"version": "6.0.0",
										"extras": map[string]interface{}{
											groupIdKey:    "io.fabric8",
											artifactIdKey: "kubernetes-client",
											pomPathKey:    "pom.xml",
										},
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
									baseDepKey: map[string]interface{}{
										"name":    "io.fabric8.kubernetes-client",
										"version": "6.0.0",
										"extras": map[string]interface{}{
											groupIdKey:    "io.fabric8",
											artifactIdKey: "kubernetes-client",
											pomPathKey:    "pom.xml",
										},
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
									baseDepKey: map[string]interface{}{
										"name":    "io.fabric8.kubernetes-client",
										"version": "6.0.0",
										"extras": map[string]interface{}{
											groupIdKey:    "io.fabric8",
											artifactIdKey: "kubernetes-client",
											pomPathKey:    "pom.xml",
										},
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
			//p.depInit()
			var deps []provider.DepDAGItem
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

func Test_parseGradleDependencyOutput(t *testing.T) {
	gradleOutput := `
Starting a Gradle Daemon, 1 incompatible Daemon could not be reused, use --status for details

> Task :dependencies

------------------------------------------------------------
Root project
------------------------------------------------------------

annotationProcessor - Annotation processors and their dependencies for source set 'main'.
No dependencies

api - API dependencies for source set 'main'. (n)
No dependencies

apiElements - API elements for main. (n)
No dependencies

archives - Configuration for archive artifacts. (n)
No dependencies

compileClasspath - Compile classpath for source set 'main'.
+--- org.codehaus.groovy:groovy:3.+ -> 3.0.21
+--- org.codehaus.groovy:groovy-json:3.+ -> 3.0.21
|    \--- org.codehaus.groovy:groovy:3.0.21
+--- com.codevineyard:hello-world:{strictly 1.0.1} -> 1.0.1
\--- :simple-jar

testRuntimeOnly - Runtime only dependencies for source set 'test'. (n)
No dependencies

(*) - dependencies omitted (listed previously)

(n) - Not resolved (configuration is not meant to be resolved)

A web-based, searchable dependency report is available by adding the --scan option.

BUILD SUCCESSFUL in 4s
1 actionable task: 1 executed
`

	lines := strings.Split(gradleOutput, "\n")

	p := javaServiceClient{
		log:         testr.New(t),
		depToLabels: map[string]*depLabelItem{},
		config: provider.InitConfig{
			ProviderSpecificConfig: map[string]interface{}{
				"excludePackages": []string{},
			},
		},
	}

	wantedDeps := []provider.DepDAGItem{
		{
			Dep: provider.Dep{
				Name:     "org.codehaus.groovy.groovy",
				Version:  "3.0.21",
				Indirect: false,
			},
		},
		{
			Dep: provider.Dep{
				Name:     "org.codehaus.groovy.groovy-json",
				Version:  "3.0.21",
				Indirect: false,
			},
			AddedDeps: []provider.DepDAGItem{
				{
					Dep: provider.Dep{
						Name:     "org.codehaus.groovy.groovy",
						Version:  "3.0.21",
						Indirect: true,
					},
				},
			},
		},
		{
			Dep: provider.Dep{
				Name:     "com.codevineyard.hello-world",
				Version:  "1.0.1",
				Indirect: false,
			},
		},
		{
			Dep: provider.Dep{
				Name:     "simple-jar",
				Indirect: false,
			},
		},
	}

	deps := p.parseGradleDependencyOutput(lines)

	if len(deps) != len(wantedDeps) {
		t.Errorf("different number of dependencies found")
	}

	for i := 0; i < len(deps); i++ {
		dep := deps[i]
		wantedDep := wantedDeps[i]
		if dep.Dep.Name != wantedDep.Dep.Name {
			t.Errorf("wanted name: %s, found name: %s", wantedDep.Dep.Name, dep.Dep.Name)
		}
		if dep.Dep.Version != wantedDep.Dep.Version {
			t.Errorf("wanted version: %s, found version: %s", wantedDep.Dep.Version, dep.Dep.Version)
		}
		if len(dep.AddedDeps) != len(wantedDep.AddedDeps) {
			t.Errorf("wanted %d child deps, found %d for dep %s", len(wantedDep.AddedDeps), len(dep.AddedDeps), dep.Dep.Name)
		}

	}

}

func Test_parseGradleDependencyOutput_withTwoLevelsOfNesting(t *testing.T) {
	gradleOutput := `
Starting a Gradle Daemon, 1 incompatible Daemon could not be reused, use --status for details

> Task :dependencies

------------------------------------------------------------
Root project
------------------------------------------------------------

annotationProcessor - Annotation processors and their dependencies for source set 'main'.
No dependencies

api - API dependencies for source set 'main'. (n)
No dependencies

apiElements - API elements for main. (n)
No dependencies

archives - Configuration for archive artifacts. (n)
No dependencies

compileClasspath - Compile classpath for source set 'main'.
+--- net.sourceforge.pmd:pmd-java:5.6.1
     +--- net.sourceforge.pmd:pmd-core:5.6.1
     |    \--- com.google.code.gson:gson:2.5
     \--- net.sourceforge.saxon:saxon:9.1.0.8
+--- org.apache.logging.log4j:log4j-api:2.9.1

testRuntimeOnly - Runtime only dependencies for source set 'test'. (n)
No dependencies

(*) - dependencies omitted (listed previously)

(n) - Not resolved (configuration is not meant to be resolved)

A web-based, searchable dependency report is available by adding the --scan option.

BUILD SUCCESSFUL in 4s
1 actionable task: 1 executed
`

	lines := strings.Split(gradleOutput, "\n")

	p := javaServiceClient{
		log:         testr.New(t),
		depToLabels: map[string]*depLabelItem{},
		config: provider.InitConfig{
			ProviderSpecificConfig: map[string]interface{}{
				"excludePackages": []string{},
			},
		},
	}

	wantedDeps := []provider.DepDAGItem{
		{
			Dep: provider.Dep{
				Name:     "net.sourceforge.pmd.pmd-java",
				Version:  "5.6.1",
				Indirect: false,
			},
			AddedDeps: []provider.DepDAGItem{
				{
					Dep: provider.Dep{
						Name:     "net.sourceforge.pmd.pmd-core",
						Version:  "5.6.1",
						Indirect: true,
					},
					AddedDeps: []provider.DepDAGItem{
						{
							Dep: provider.Dep{
								Name:     "com.google.code.gson.gson",
								Version:  "2.5",
								Indirect: true,
							},
						},
					},
				},
				{
					Dep: provider.Dep{
						Name:     "net.sourceforge.saxon.saxon",
						Version:  "9.1.0.8",
						Indirect: true,
					},
				},
			},
		},
		{
			Dep: provider.Dep{
				Name:     "org.apache.logging.log4j.log4j-api",
				Version:  "2.9.1",
				Indirect: false,
			},
		},
	}

	deps := p.parseGradleDependencyOutput(lines)

	if len(deps) != len(wantedDeps) {
		t.Errorf("different number of dependencies found")
	}

	for i := 0; i < len(deps); i++ {
		dep := deps[i]
		wantedDep := wantedDeps[i]
		if dep.Dep.Name != wantedDep.Dep.Name {
			t.Errorf("wanted name: %s, found name: %s", wantedDep.Dep.Name, dep.Dep.Name)
		}
		if dep.Dep.Version != wantedDep.Dep.Version {
			t.Errorf("wanted version: %s, found version: %s", wantedDep.Dep.Version, dep.Dep.Version)
		}
		if len(dep.AddedDeps) != len(wantedDep.AddedDeps) {
			t.Errorf("wanted %d child deps, found %d for dep %s", len(wantedDep.AddedDeps), len(dep.AddedDeps), dep.Dep.Name)
		}

	}

}

package builtin

import (
	"context"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/provider"
	"gopkg.in/yaml.v2"
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

func BenchmarkRunOSSpecificGrepCommand(b *testing.B) {
	for i := 0; i < b.N; i++ {
		path, err := filepath.Abs("../../../external-providers/java-external-provider/examples/customers-tomcat-legacy/")
		if err != nil {
			return
		}
		runOSSpecificGrepCommand("Apache License 1.1",
			path,
			provider.ProviderContext{Template: map[string]engine.ChainTemplate{}}, logr.Discard())
	}
}

func Test_builtinServiceClient_Evaluate_InclusionExclusion(t *testing.T) {
	baseLocation := filepath.Join(".", "testdata", "search_scopes")
	baseLocation, err := filepath.Abs(baseLocation)
	if err != nil {
		t.Errorf("builtinServiceClient.Evaluate() unable to run tests, cannot get absolute file path for base location err = %v", err)
		return
	}

	tests := []struct {
		name       string
		capability string
		// this is what we already had before introducing scopes
		includedPathsFromConfig []string
		condition               builtinCondition
		chainTemplate           engine.ChainTemplate
		wantFilePaths           []string
		wantErr                 bool
	}{
		{
			name:                    "(Filecontent) No include, no exclude",
			capability:              "filecontent",
			includedPathsFromConfig: []string{},
			condition: builtinCondition{
				Filecontent: fileContentCondition{
					Pattern: "(fox|app.config.property = .*)",
				},
			},
			chainTemplate: engine.ChainTemplate{},
			wantFilePaths: []string{
				filepath.Join("dir_a", "a.txt"),
				filepath.Join("dir_a", "a.properties"),
				filepath.Join("dir_a", "dir_b", "ab.properties"),
				filepath.Join("dir_a", "dir_b", "ab.txt"),
				filepath.Join("dir_b", "dir_a", "ba.properties"),
				filepath.Join("dir_b", "b.properties"),
				filepath.Join("dir_b", "b.txt"),
			},
		},
		{
			name:       "(Filecontent) Include using the config (legacy inclusion), no exclude",
			capability: "filecontent",
			includedPathsFromConfig: []string{
				"dir_a/",
			},
			condition: builtinCondition{
				Filecontent: fileContentCondition{
					Pattern: "(fox|app.config.property = .*)",
				},
			},
			chainTemplate: engine.ChainTemplate{},
			wantFilePaths: []string{
				filepath.Join("dir_a", "a.txt"),
				filepath.Join("dir_a", "a.properties"),
				filepath.Join("dir_a", "dir_b", "ab.properties"),
				filepath.Join("dir_a", "dir_b", "ab.txt"),
			},
		},
		{
			name:       "(File) Include using the config (legacy inclusion), no exclude",
			capability: "file",
			includedPathsFromConfig: []string{
				"dir_a",
			},
			condition: builtinCondition{
				File: fileCondition{
					Pattern: ".*.properties",
				},
			},
			chainTemplate: engine.ChainTemplate{},
			wantFilePaths: []string{
				filepath.Join("dir_a", "a.properties"),
				filepath.Join("dir_a", "dir_b", "ab.properties"),
			},
		},
		{
			name:       "(XML) Include using the config (legacy inclusion), no exclude",
			capability: "xml",
			includedPathsFromConfig: []string{
				"dir_b",
			},
			condition: builtinCondition{
				XML: xmlCondition{
					XPath: "//name[text()='Test name']",
				},
			},
			chainTemplate: engine.ChainTemplate{},
			wantFilePaths: []string{
				filepath.Join("dir_b", "dir_a", "ba.xml"),
				filepath.Join("dir_b", "b.xml"),
			},
		},
		{
			name:       "(JSON) Include using the config (legacy inclusion), no exclude",
			capability: "json",
			includedPathsFromConfig: []string{
				"dir_b",
			},
			condition: builtinCondition{
				JSON: jsonCondition{
					XPath: "//name[text()='Test name']",
				},
			},
			chainTemplate: engine.ChainTemplate{},
			wantFilePaths: []string{
				filepath.Join("dir_b", "dir_a", "ba.json"),
				filepath.Join("dir_b", "b.json"),
			},
		},
		{
			name:       "(Filecontent) Include using the config (legacy inclusion), with exclude",
			capability: "filecontent",
			includedPathsFromConfig: []string{
				"dir_a/",
				"dir_b/",
			},
			condition: builtinCondition{
				Filecontent: fileContentCondition{
					Pattern: "(fox|app.config.property = .*)",
				},
			},
			chainTemplate: engine.ChainTemplate{
				ExcludedPaths: []string{
					filepath.Join(baseLocation, "dir_a"),
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_b", "dir_a", "ba.properties"),
				filepath.Join("dir_b", "b.properties"),
				filepath.Join("dir_b", "b.txt"),
			},
		},
		{
			name:       "(File) Include using the config (legacy inclusion), with exclude",
			capability: "file",
			includedPathsFromConfig: []string{
				"dir_a",
				"dir_b",
			},
			condition: builtinCondition{
				File: fileCondition{
					Pattern: ".*.properties",
				},
			},
			chainTemplate: engine.ChainTemplate{
				ExcludedPaths: []string{
					filepath.Join(baseLocation, "dir_a"),
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_b", "b.properties"),
				filepath.Join("dir_b", "dir_a", "ba.properties"),
			},
		},
		{
			name:       "(XML) Include using the config (legacy inclusion), with exclude",
			capability: "xml",
			includedPathsFromConfig: []string{
				"dir_a",
				"dir_b",
			},
			condition: builtinCondition{
				XML: xmlCondition{
					XPath: "//name[text()='Test name']",
				},
			},
			chainTemplate: engine.ChainTemplate{
				ExcludedPaths: []string{
					filepath.Join(baseLocation, "dir_b"),
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_a", "dir_b", "ab.xml"),
				filepath.Join("dir_a", "a.xml"),
			},
		},
		{
			name:       "(JSON) Include using the config (legacy inclusion), with exclude",
			capability: "json",
			includedPathsFromConfig: []string{
				"dir_a",
				"dir_b",
			},
			condition: builtinCondition{
				JSON: jsonCondition{
					XPath: "//name[text()='Test name']",
				},
			},
			chainTemplate: engine.ChainTemplate{
				ExcludedPaths: []string{
					filepath.Join(baseLocation, "dir_a"),
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_b", "dir_a", "ba.json"),
				filepath.Join("dir_b", "b.json"),
			},
		},
		{
			name:                    "(Filecontent) Include using the scopes, no exclude",
			capability:              "filecontent",
			includedPathsFromConfig: []string{},
			condition: builtinCondition{
				Filecontent: fileContentCondition{
					Pattern: "(fox|app.config.property = .*)",
				},
			},
			chainTemplate: engine.ChainTemplate{
				Filepaths: []string{
					filepath.Join("dir_a", "a.txt"),
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_a", "a.txt"),
			},
		},
		{
			name:                    "(File) Include using the scopes, no exclude",
			capability:              "file",
			includedPathsFromConfig: []string{},
			condition: builtinCondition{
				File: fileCondition{
					Pattern: ".*.properties",
				},
			},
			chainTemplate: engine.ChainTemplate{
				Filepaths: []string{
					filepath.Join("dir_b", "b.properties"),
					filepath.Join("dir_b", "dir_a", "ba.properties"),
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_b", "b.properties"),
				filepath.Join("dir_b", "dir_a", "ba.properties"),
			},
		},
		{
			name:                    "(XML) Include using the scopes, no exclude",
			capability:              "xml",
			includedPathsFromConfig: []string{},
			condition: builtinCondition{
				XML: xmlCondition{
					XPath: "//name[text()='Test name']",
				},
			},
			chainTemplate: engine.ChainTemplate{
				Filepaths: []string{
					filepath.Join("dir_a", "a.xml"),
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_a", "a.xml"),
			},
		},
		{
			name:                    "(JSON) Include using the scopes, no exclude",
			capability:              "json",
			includedPathsFromConfig: []string{},
			condition: builtinCondition{
				JSON: jsonCondition{
					XPath: "//name[text()='Test name']",
				},
			},
			chainTemplate: engine.ChainTemplate{
				Filepaths: []string{
					filepath.Join("dir_b", "dir_a", "ba.json"),
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_b", "dir_a", "ba.json"),
			},
		},

		{
			name:                    "(Filecontent) Exclude dir, no include",
			capability:              "filecontent",
			includedPathsFromConfig: []string{},
			condition: builtinCondition{
				Filecontent: fileContentCondition{
					Pattern: "(fox|app.config.property = .*)",
				},
			},
			chainTemplate: engine.ChainTemplate{
				ExcludedPaths: []string{
					filepath.Join(baseLocation, "dir_a"),
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_b", "dir_a", "ba.properties"),
				filepath.Join("dir_b", "b.properties"),
				filepath.Join("dir_b", "b.txt"),
			},
		},
		{
			name:                    "(File) Exclude dir, no include",
			capability:              "file",
			includedPathsFromConfig: []string{},
			condition: builtinCondition{
				File: fileCondition{
					Pattern: "(.*.txt|.*.properties)",
				},
			},
			chainTemplate: engine.ChainTemplate{
				ExcludedPaths: []string{
					filepath.Join(baseLocation, "dir_a", "dir_b"),
					filepath.Join(baseLocation, "dir_b", "dir_a"),
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_a", "a.txt"),
				filepath.Join("dir_a", "a.properties"),
				filepath.Join("dir_b", "b.properties"),
				filepath.Join("dir_b", "b.txt"),
			},
		},
		{
			name:                    "(XML) Exclude dir, no include",
			capability:              "xml",
			includedPathsFromConfig: []string{},
			condition: builtinCondition{
				XML: xmlCondition{
					XPath: "//name[text()='Test name']",
				},
			},
			chainTemplate: engine.ChainTemplate{
				ExcludedPaths: []string{
					filepath.Join(baseLocation, "dir_a"),
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_b", "b.xml"),
				filepath.Join("dir_b", "dir_a", "ba.xml"),
			},
		},
		{
			name:                    "(JSON) Exclude dir, no include",
			capability:              "json",
			includedPathsFromConfig: []string{},
			condition: builtinCondition{
				JSON: jsonCondition{
					XPath: "//name[text()='Test name']",
				},
			},
			chainTemplate: engine.ChainTemplate{
				ExcludedPaths: []string{
					filepath.Join(baseLocation, "dir_b", "dir_a"),
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_a", "a.json"),
				filepath.Join("dir_a", "dir_b", "ab.json"),
				filepath.Join("dir_b", "b.json"),
			},
		},
		{
			name:                    "(Filecontent) Exclude using pattern",
			capability:              "filecontent",
			includedPathsFromConfig: []string{},
			condition: builtinCondition{
				Filecontent: fileContentCondition{
					Pattern: "(fox|app.config.property = .*)",
				},
			},
			chainTemplate: engine.ChainTemplate{
				ExcludedPaths: []string{
					".*ba.*",
					".*.txt",
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_a", "a.properties"),
				filepath.Join("dir_a", "dir_b", "ab.properties"),
				filepath.Join("dir_b", "b.properties"),
			},
		},
	}

	getAbsolutePaths := func(baseLocation string, relativePaths []string) []string {
		absPaths := []string{}
		for _, relPath := range relativePaths {
			absPaths = append(absPaths, filepath.Join(baseLocation, relPath))
		}
		return absPaths
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &builtinServiceClient{
				config: provider.InitConfig{
					Location: baseLocation,
				},
				log:           testr.New(t),
				includedPaths: tt.includedPathsFromConfig,
				locationCache: map[string]float64{},
				cacheMutex:    sync.RWMutex{},
			}
			chainTemplate := engine.ChainTemplate{
				Filepaths:     getAbsolutePaths(p.config.Location, tt.chainTemplate.Filepaths),
				ExcludedPaths: tt.chainTemplate.ExcludedPaths,
			}
			tt.condition.ProviderContext = provider.ProviderContext{
				Template: map[string]engine.ChainTemplate{
					engine.TemplateContextPathScopeKey: chainTemplate,
				},
			}
			conditionInfo, err := yaml.Marshal(&tt.condition)
			if err != nil {
				t.Errorf("builtinServiceClient.Evaluate() invalid test case, please check if condition is correct")
				return
			}
			got, err := p.Evaluate(context.TODO(), tt.capability, conditionInfo)
			if (err != nil) != tt.wantErr {
				t.Errorf("builtinServiceClient.Evaluate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			gotFilepaths := []string{}
			for _, fp := range got.Incidents {
				gotFilepaths = append(gotFilepaths, fp.FileURI.Filename())
			}
			wantFilepaths := getAbsolutePaths(p.config.Location, tt.wantFilePaths)
			sort.Strings(gotFilepaths)
			sort.Strings(wantFilepaths)
			if !reflect.DeepEqual(gotFilepaths, wantFilepaths) {
				t.Errorf("builtinServiceClient.Evaluate() = %v, want %v", gotFilepaths, wantFilepaths)
			}
		})
	}
}

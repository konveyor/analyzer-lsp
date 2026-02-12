package builtin

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
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

func BenchmarkFileSearch(b *testing.B) {
	baseLocation, err := filepath.Abs("../../../external-providers/java-external-provider/examples/customers-tomcat-legacy/")
	if err != nil {
		b.Fatalf("error getting base location for benchmark test")
	}
	sc := &builtinServiceClient{
		config: provider.InitConfig{
			Location: baseLocation,
		},
		log:            logr.Discard(),
		locationCache:  map[string]float64{},
		cacheMutex:     sync.RWMutex{},
		workingCopyMgr: NewTempFileWorkingCopyManger(logr.Discard()),
	}
	fileSearcher := provider.FileSearcher{
		BasePath: baseLocation,
		FailFast: true,
		Log:      logr.Discard(),
	}
	for i := 0; i < b.N; i++ {
		filePaths, err := fileSearcher.Search(provider.SearchCriteria{
			Patterns: []string{},
		})
		if err != nil {
			b.Fatalf("error running file search for benchmark test")
		}
		sc.performFileContentSearch("Apache License 1.1", filePaths)
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
		excludedPathsFromConfig []string
		condition               builtinCondition
		chainTemplate           engine.ChainTemplate
		notifiedFileChanges     []provider.FileChange
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
		{
			name:                    "(Filecontent) Exclude using pattern & dir",
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
					filepath.Join(baseLocation, "dir_a", "dir_b"),
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_a", "a.properties"),
				filepath.Join("dir_b", "b.properties"),
			},
		},
		{
			name:       "(Filecontent) Exclude using pattern, relative path & dir, with legacy inclusion",
			capability: "filecontent",
			includedPathsFromConfig: []string{
				filepath.Join(baseLocation, "dir_a"),
			},
			condition: builtinCondition{
				Filecontent: fileContentCondition{
					Pattern: "(fox|<data>)",
				},
			},
			chainTemplate: engine.ChainTemplate{
				ExcludedPaths: []string{
					"a.json",
					filepath.Join("dir_a", "a.properties"),
					filepath.Join(baseLocation, "dir_a", "dir_b"),
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_a", "a.txt"),
				filepath.Join("dir_a", "a.xml"),
			},
		},
		{
			name:       "(Filecontent) Exclude using pattern, relative path & dir at rule scope, with legacy inclusion & filepath inclusion",
			capability: "filecontent",
			includedPathsFromConfig: []string{
				filepath.Join(baseLocation, "dir_b", "dir_a"),
			},
			excludedPathsFromConfig: []string{
				filepath.Join("dir_b", "dir_a", "ba.properties"),
				"b.json",
				"dir_a",
			},
			condition: builtinCondition{
				Filecontent: fileContentCondition{
					Pattern: "(fox|<data>|\"description\"|app.config.property = .*)",
				},
			},
			chainTemplate: engine.ChainTemplate{
				Filepaths: []string{
					// in this test, we check if rule scope overrides the
					// global scope
					"dir_b",
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_b", "b.properties"),
				filepath.Join("dir_b", "b.txt"),
				filepath.Join("dir_b", "b.xml"),
				filepath.Join("dir_b", "dir_a", "ba.json"),
				filepath.Join("dir_b", "dir_a", "ba.xml"),
			},
		},
		{
			name:       "(Filecontent) Exclude using pattern, relative path & dir at provider scope, with legacy inclusion & filepath inclusion",
			capability: "filecontent",
			includedPathsFromConfig: []string{
				filepath.Join(baseLocation, "dir_b", "dir_a"),
			},
			condition: builtinCondition{
				Filecontent: fileContentCondition{
					Pattern: "(fox|<data>|\"description\"|app.config.property = .*)",
				},
			},
			chainTemplate: engine.ChainTemplate{
				Filepaths: []string{
					// in this test, we check if rule scope overrides the
					// global scope
					"dir_b",
				},
				ExcludedPaths: []string{
					filepath.Join("dir_b", "dir_a", "ba.properties"),
					"b.json",
					"dir_a",
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_b", "b.properties"),
				filepath.Join("dir_b", "b.txt"),
				filepath.Join("dir_b", "b.xml"),
				filepath.Join("dir_b", "dir_a", "ba.json"),
				filepath.Join("dir_b", "dir_a", "ba.xml"),
			},
		},
		{
			name:       "(XML) Include files from cond.Filepaths (single rendered path), no include / exclude",
			capability: "xml",
			condition: builtinCondition{
				XML: xmlCondition{
					XPath: "//name[text()='Test name']",
					Filepaths: []string{fmt.Sprintf("%s %s",
						filepath.Join(baseLocation, "dir_b", "dir_a", "ba.xml"), filepath.Join(baseLocation, "dir_b", "b.xml")),
					},
				},
			},
			chainTemplate: engine.ChainTemplate{},
			wantFilePaths: []string{
				filepath.Join("dir_b", "dir_a", "ba.xml"),
				filepath.Join("dir_b", "b.xml"),
			},
		},
		{
			name:       "(XML) Include files from cond.Filepaths (single rendered path), with include",
			capability: "xml",
			condition: builtinCondition{
				XML: xmlCondition{
					XPath: "//name[text()='Test name']",
					Filepaths: []string{fmt.Sprintf("%s %s",
						filepath.Join(baseLocation, "dir_b", "dir_a", "ba.xml"), filepath.Join(baseLocation, "dir_b", "b.xml")),
					},
				},
			},
			chainTemplate: engine.ChainTemplate{
				Filepaths: []string{
					// here we test whether intersection works as expected
					filepath.Join("dir_b", "b.xml"),
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_b", "b.xml"),
			},
		},
		{
			name:       "(XML) Include files from cond.Filepaths, with include & exclude rule scope",
			capability: "xml",
			condition: builtinCondition{
				XML: xmlCondition{
					XPath: "//name[text()='Test name']",
					Filepaths: []string{
						filepath.Join("dir_b", "dir_a", "ba.xml"), filepath.Join("dir_b", "b.xml"),
						filepath.Join("dir_a", "dir_b", "ab.xml"), filepath.Join("dir_a", "a.xml"),
					},
				},
			},
			chainTemplate: engine.ChainTemplate{
				Filepaths: []string{
					// here we test whether intersection works as expected
					filepath.Join("dir_b", "b.xml"),
					filepath.Join("dir_a", "dir_b"),
				},
				ExcludedPaths: []string{
					filepath.Join("dir_b", "dir_a"),
					filepath.Join("dir_a", "a.xml"),
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_b", "b.xml"),
				filepath.Join("dir_a", "dir_b", "ab.xml"),
			},
		},
		{
			name:       "(XML) Include files from cond.Filepaths, with include & with notifyFileChanges()",
			capability: "xml",
			condition: builtinCondition{
				XML: xmlCondition{
					XPath: "//name[text()='Test name']",
					Filepaths: []string{
						"b.xml",
					},
				},
			},
			chainTemplate: engine.ChainTemplate{
				Filepaths: []string{
					filepath.Join("dir_b", "dir_a", "ba.xml"),
				},
			},
			notifiedFileChanges: []provider.FileChange{
				{
					Path:    filepath.Join(baseLocation, "dir_b", "dir_a", "ba.xml"),
					Content: "<data>\n\t<name>Test name</name>\n\t<description>Test description</description>\n</data>",
				},
			},
			wantFilePaths: []string{},
		},
		{
			name:       "(XML) Include files from cond.Filepaths, with include & with notifyFileChanges()",
			capability: "xml",
			condition: builtinCondition{
				XML: xmlCondition{
					XPath: "//name[text()='Test name']",
					Filepaths: []string{
						"b.xml",
					},
				},
			},
			notifiedFileChanges: []provider.FileChange{
				{
					Path:    filepath.Join(baseLocation, "dir_b", "b.xml"),
					Content: "<data>\n\t<name>Test name</name>\n\t<description>Test description</description>\n</data>",
				},
			},
			wantFilePaths: []string{filepath.Join("dir_b", "b.xml")},
		},
		{
			name:       "(Filecontent) Include files from cond.Filepaths (single rendered path), no include / exclude",
			capability: "filecontent",
			condition: builtinCondition{
				Filecontent: fileContentCondition{
					Pattern: "(fox|app.config.property = .*)",
					Filepaths: []string{fmt.Sprintf("%s %s",
						filepath.Join(baseLocation, "dir_a", "a.txt"), filepath.Join(baseLocation, "dir_b", "b.txt")),
					},
				},
			},
			chainTemplate: engine.ChainTemplate{},
			wantFilePaths: []string{
				filepath.Join("dir_a", "a.txt"),
				filepath.Join("dir_b", "b.txt"),
			},
		},
		{
			name:       "(Filecontent) Include files from cond.Filepaths (single rendered path), with include",
			capability: "filecontent",
			condition: builtinCondition{
				Filecontent: fileContentCondition{
					Pattern: "(fox|app.config.property = .*)",
					Filepaths: []string{fmt.Sprintf("%s %s",
						filepath.Join(baseLocation, "dir_a", "a.txt"), filepath.Join(baseLocation, "dir_b", "b.txt")),
					},
				},
			},
			chainTemplate: engine.ChainTemplate{
				Filepaths: []string{
					// here we test whether intersection works as expected
					filepath.Join("dir_b", "b.txt"),
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_b", "b.txt"),
			},
		},
		{
			name:       "(Filecontent) Include files from cond.Filepaths, with include & exclude rule scope",
			capability: "filecontent",
			condition: builtinCondition{
				Filecontent: fileContentCondition{
					Pattern: "(fox|app.config.property = .*)",
					Filepaths: []string{
						filepath.Join("dir_a", "a.txt"), filepath.Join("dir_b", "b.txt"),
						filepath.Join("dir_a", "a.properties"), filepath.Join("dir_b", "b.properties"),
					},
				},
			},
			chainTemplate: engine.ChainTemplate{
				Filepaths: []string{
					// here we test whether intersection works as expected
					filepath.Join("dir_b", "b.txt"),
					filepath.Join("dir_a", "a.properties"),
				},
				ExcludedPaths: []string{
					filepath.Join("dir_b", "b.properties"),
					filepath.Join("dir_a", "a.txt"),
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_b", "b.txt"),
				filepath.Join("dir_a", "a.properties"),
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
				log: testr.NewWithOptions(t, testr.Options{
					Verbosity: 10,
				}),
				includedPaths:  tt.includedPathsFromConfig,
				excludedDirs:   tt.excludedPathsFromConfig,
				locationCache:  map[string]float64{},
				cacheMutex:     sync.RWMutex{},
				workingCopyMgr: NewTempFileWorkingCopyManger(testr.New(t)),
			}
			p.workingCopyMgr.init()
			defer p.workingCopyMgr.stop()
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
			for _, change := range tt.notifiedFileChanges {
				p.NotifyFileChanges(context.TODO(), change)
			}
			// working copy manager needs to reconcile the changes
			time.Sleep(time.Second * 1)
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

func Test_builtinServiceClient_Evaluate_ExcludeDirs(t *testing.T) {
	baseLocation := filepath.Join(".", "testdata", "search_scopes")
	baseLocation, err := filepath.Abs(baseLocation)
	if err != nil {
		t.Errorf("builtinServiceClient.Evaluate() unable to run tests, cannot get absolute file path for base location err = %v", err)
		return
	}
	tests := []struct {
		name                   string
		capability             string
		condition              builtinCondition
		excludedDirsFromConfig []string
		wantFilePaths          []string
		wantErr                bool
	}{
		{
			name:                   "(filecontent) no exclude, match all",
			capability:             "filecontent",
			excludedDirsFromConfig: []string{},
			condition: builtinCondition{
				Filecontent: fileContentCondition{
					Pattern: "(fox|app.config.property = .*)",
				},
			},
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
			name:       "(filecontent) excluded all, match none",
			capability: "filecontent",
			condition: builtinCondition{
				Filecontent: fileContentCondition{
					Pattern: "(fox|app.config.property = .*)",
				},
			},
			excludedDirsFromConfig: []string{
				filepath.Join(baseLocation, "dir_a"),
				filepath.Join(baseLocation, "dir_b"),
			},
			wantFilePaths: []string{},
		},
		{
			name:       "(filecontent) dir_a excluded path given, match only inside dir_b",
			capability: "filecontent",
			condition: builtinCondition{
				Filecontent: fileContentCondition{
					Pattern: "(fox|app.config.property = .*)",
				},
			},
			excludedDirsFromConfig: []string{filepath.Join(baseLocation, "dir_a")},
			wantFilePaths: []string{
				filepath.Join("dir_b", "dir_a", "ba.properties"),
				filepath.Join("dir_b", "b.properties"),
				filepath.Join("dir_b", "b.txt"),
			},
		},
		{
			name:                   "(file) no exclude, match all",
			capability:             "file",
			excludedDirsFromConfig: []string{},
			condition: builtinCondition{
				File: fileCondition{
					Pattern: ".*.properties",
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_a", "a.properties"),
				filepath.Join("dir_a", "dir_b", "ab.properties"),
				filepath.Join("dir_b", "b.properties"),
				filepath.Join("dir_b", "dir_a", "ba.properties"),
			},
		},
		{
			name:                   "(file) exclude dir_b, match only dir_a",
			capability:             "file",
			excludedDirsFromConfig: []string{filepath.Join(baseLocation, "dir_b")},
			condition: builtinCondition{
				File: fileCondition{
					Pattern: ".*.properties",
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_a", "a.properties"),
				filepath.Join("dir_a", "dir_b", "ab.properties"),
			},
		},
		{
			name:                   "(XML) exclude dir_a",
			capability:             "xml",
			excludedDirsFromConfig: []string{filepath.Join(baseLocation, "dir_a")},
			condition: builtinCondition{
				XML: xmlCondition{
					XPath: "//name[text()='Test name']",
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_b", "dir_a", "ba.xml"),
				filepath.Join("dir_b", "b.xml"),
			},
		},
		{
			name:                   "(JSON) exclude dir_b",
			capability:             "json",
			excludedDirsFromConfig: []string{filepath.Join(baseLocation, "dir_b")},
			condition: builtinCondition{
				JSON: jsonCondition{
					XPath: "//name[text()='Test name']",
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_a", "dir_b", "ab.json"),
				filepath.Join("dir_a", "a.json"),
			},
		},
		{
			name:       "(XML) Exclude a non existent dir using a relative path",
			capability: "xml",
			// the name of the dir ab intentionally matches file ab.xml
			// this addresses an edge case introduced in PR #1042
			excludedDirsFromConfig: []string{"ab"},
			condition: builtinCondition{
				XML: xmlCondition{
					XPath: "//name[text()='Test name']",
				},
			},
			wantFilePaths: []string{
				filepath.Join("dir_a", "dir_b", "ab.xml"),
				filepath.Join("dir_a", "a.xml"),
				filepath.Join("dir_b", "b.xml"),
				filepath.Join("dir_b", "dir_a", "ba.xml"),
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

			sc := &builtinServiceClient{
				config: provider.InitConfig{
					Location: baseLocation,
					ProviderSpecificConfig: map[string]interface{}{
						"excludedDirs": tt.excludedDirsFromConfig,
					},
				},
				excludedDirs:   tt.excludedDirsFromConfig,
				log:            testr.New(t),
				locationCache:  map[string]float64{},
				cacheMutex:     sync.RWMutex{},
				workingCopyMgr: NewTempFileWorkingCopyManger(testr.New(t)),
			}
			conditionInfo, err := yaml.Marshal(&tt.condition)
			if err != nil {
				t.Errorf("builtinServiceClient.Evaluate() invalid test case, please check if condition is correct")
				return
			}
			got, err := sc.Evaluate(context.TODO(), tt.capability, conditionInfo)
			if (err != nil) != tt.wantErr {
				t.Errorf("builtinServiceClient.Evaluate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			gotFilepaths := []string{}
			for _, fp := range got.Incidents {
				gotFilepaths = append(gotFilepaths, fp.FileURI.Filename())
			}

			wantFilepaths := getAbsolutePaths(sc.config.Location, tt.wantFilePaths)
			sort.Strings(gotFilepaths)
			sort.Strings(wantFilepaths)
			if !reflect.DeepEqual(gotFilepaths, wantFilepaths) {
				t.Errorf("builtinServiceClient.Evaluate() = %v, want %v", gotFilepaths, wantFilepaths)
			}
		})
	}
}

func Test_builtinServiceClient_performFileContentSearch(t *testing.T) {
	baseLocation := filepath.Join(".", "testdata", "filecontent")
	baseLocation, err := filepath.Abs(baseLocation)
	if err != nil {
		t.Fatalf("unable to get absolute path for testdata: %v", err)
	}

	tests := []struct {
		name        string
		pattern     string
		files       []string
		wantMatches int
		wantFiles   []string // expected files that should have matches
		wantTexts   []string // expected matched text snippets
		wantErr     bool
		description string
	}{
		{
			name:        "literal string match - should use fast bytes.Index",
			pattern:     "fox",
			files:       []string{filepath.Join(baseLocation, "simple.txt")},
			wantMatches: 2,
			wantFiles:   []string{"simple.txt"},
			wantTexts:   []string{"fox", "fox"},
			description: "Literal string without regex metacharacters uses optimized byte search",
		},
		{
			name:        "simple regex pattern - single line",
			pattern:     "Line [0-9]+:",
			files:       []string{filepath.Join(baseLocation, "simple.txt")},
			wantMatches: 7,
			wantFiles:   []string{"simple.txt"},
			description: "Simple regex matches multiple lines",
		},
		{
			name:        "regex with word boundary",
			pattern:     `\bquick\b`,
			files:       []string{filepath.Join(baseLocation, "simple.txt")},
			wantMatches: 2,
			wantFiles:   []string{"simple.txt"},
			wantTexts:   []string{"quick", "quick"},
			description: "Word boundary anchors work correctly",
		},
		{
			name:        "pattern with alternation",
			pattern:     "(fox|dog)",
			files:       []string{filepath.Join(baseLocation, "simple.txt")},
			wantMatches: 3,
			wantFiles:   []string{"simple.txt"},
			description: "Alternation pattern matches all occurrences",
		},
		{
			name:        "case sensitive match",
			pattern:     "Line",
			files:       []string{filepath.Join(baseLocation, "simple.txt")},
			wantMatches: 7,
			wantFiles:   []string{"simple.txt"},
			description: "Case-sensitive matching by default",
		},
		{
			name:        "no matches found",
			pattern:     "nonexistent",
			files:       []string{filepath.Join(baseLocation, "simple.txt")},
			wantMatches: 0,
			wantFiles:   []string{},
			description: "Returns empty result when no matches",
		},
		{
			name:        "pattern with quotes",
			pattern:     `\"fox\"`,
			files:       []string{filepath.Join(baseLocation, "simple.txt")},
			wantMatches: 1,
			wantFiles:   []string{"simple.txt"},
			description: "Quotes around pattern are trimmed for backwards compatibility",
		},
		{
			name:        "multiline pattern with newline - XML dependency",
			pattern:     `<dependency>\s+<groupId>`,
			files:       []string{filepath.Join(baseLocation, "multiline.xml")},
			wantMatches: 1,
			wantFiles:   []string{"multiline.xml"},
			description: "Pattern with \\s matches across newlines",
		},
		{
			name:        "multiline pattern with negated char class",
			pattern:     `<property[^>]*>`,
			files:       []string{filepath.Join(baseLocation, "multiline.xml")},
			wantMatches: 1,
			wantFiles:   []string{"multiline.xml"},
			description: "Negated character class [^>] triggers multiline search",
		},
		{
			name:        "pattern matching special characters - dots",
			pattern:     `dots\.\.\.`,
			files:       []string{filepath.Join(baseLocation, "special_chars.txt")},
			wantMatches: 1,
			wantFiles:   []string{"special_chars.txt"},
			description: "Escaped dots match literal dots",
		},
		{
			name:        "pattern matching brackets",
			pattern:     `\[abc\]`,
			files:       []string{filepath.Join(baseLocation, "special_chars.txt")},
			wantMatches: 1,
			wantFiles:   []string{"special_chars.txt"},
			description: "Escaped brackets match literal brackets",
		},
		{
			name:        "pattern matching braces",
			pattern:     `\{key: value\}`,
			files:       []string{filepath.Join(baseLocation, "special_chars.txt")},
			wantMatches: 1,
			wantFiles:   []string{"special_chars.txt"},
			description: "Escaped braces match literal braces",
		},
		{
			name:        "pattern matching pipe character",
			pattern:     `\|\|\|`,
			files:       []string{filepath.Join(baseLocation, "special_chars.txt")},
			wantMatches: 1,
			wantFiles:   []string{"special_chars.txt"},
			description: "Escaped pipes match literal pipes",
		},
		{
			name:        "pattern matching backslashes",
			pattern:     `\\path\\to\\file`,
			files:       []string{filepath.Join(baseLocation, "special_chars.txt")},
			wantMatches: 1,
			wantFiles:   []string{"special_chars.txt"},
			description: "Double backslashes match literal backslashes",
		},
		{
			name:        "pattern matching slashes",
			pattern:     `/path/to/file`,
			files:       []string{filepath.Join(baseLocation, "special_chars.txt")},
			wantMatches: 1,
			wantFiles:   []string{"special_chars.txt"},
			description: "slashes match path like things",
		},
		{
			name:        "pattern with \\s+ matches whitespace",
			pattern:     `with\s+tabs`,
			files:       []string{filepath.Join(baseLocation, "whitespace.txt")},
			wantMatches: 1,
			wantFiles:   []string{"whitespace.txt"},
			description: "Whitespace pattern \\s triggers multiline search",
		},
		{
			name:    "multiple files - search all",
			pattern: "test",
			files: []string{
				filepath.Join(baseLocation, "simple.txt"),
				filepath.Join(baseLocation, "special_chars.txt"),
				filepath.Join(baseLocation, "whitespace.txt"),
			},
			wantMatches: 0, // "test" doesn't appear in simple.txt
			description: "Search across multiple files",
		},
		{
			name:        "empty file list",
			pattern:     "anything",
			files:       []string{},
			wantMatches: 0,
			wantFiles:   []string{},
			description: "Empty file list returns no matches",
		},
		{
			name:        "multiline XML attribute pattern",
			pattern:     `name="config"[^>]*value="test-value"`,
			files:       []string{filepath.Join(baseLocation, "multiline.xml")},
			wantMatches: 1,
			wantFiles:   []string{"multiline.xml"},
			description: "Pattern spanning multiple lines in XML attributes",
		},
		{
			name:        "literal match of common word",
			pattern:     "Test",
			files:       []string{filepath.Join(baseLocation, "special_chars.txt")},
			wantMatches: 11,
			wantFiles:   []string{"special_chars.txt"},
			description: "Literal string appears multiple times",
		},
		{
			name:        "pattern at start of line",
			pattern:     `^Line`,
			files:       []string{filepath.Join(baseLocation, "simple.txt")},
			wantMatches: 7,
			wantFiles:   []string{"simple.txt"},
			description: "Anchor ^ matches start of line",
		},
		{
			name:        "pattern at end of line",
			pattern:     `dog$`,
			files:       []string{filepath.Join(baseLocation, "simple.txt")},
			wantMatches: 1,
			wantFiles:   []string{"simple.txt"},
			description: "Anchor $ matches end of line",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := &builtinServiceClient{
				config: provider.InitConfig{
					Location: baseLocation,
				},
				log: testr.NewWithOptions(t, testr.Options{
					Verbosity: 20,
				}),
				locationCache:  map[string]float64{},
				cacheMutex:     sync.RWMutex{},
				workingCopyMgr: NewTempFileWorkingCopyManger(testr.New(t)),
			}

			results, err := sc.performFileContentSearch(tt.pattern, tt.files)
			if (err != nil) != tt.wantErr {
				t.Errorf("performFileContentSearch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(results) != tt.wantMatches {
				t.Errorf("%s: got %d matches, want %d matches", tt.description, len(results), tt.wantMatches)
				for i, result := range results {
					fileName := filepath.Base(uri.URI(result.positionParams.TextDocument.URI).Filename())
					t.Logf("  Match %d: line %d, file %s, text: %q",
						i+1, result.positionParams.Position.Line, fileName, result.match)
				}
				return
			}

			// Verify expected files contain matches
			if len(tt.wantFiles) > 0 {
				gotFiles := make(map[string]bool)
				for _, result := range results {
					fileName := filepath.Base(uri.URI(result.positionParams.TextDocument.URI).Filename())
					gotFiles[fileName] = true
				}
				for _, wantFile := range tt.wantFiles {
					if !gotFiles[wantFile] {
						t.Errorf("%s: expected file %s to have matches, but it didn't", tt.description, wantFile)
					}
				}
			}

			// Verify matched texts if specified
			if len(tt.wantTexts) > 0 {
				gotTexts := []string{}
				for _, result := range results {
					gotTexts = append(gotTexts, result.match)
				}
				if !reflect.DeepEqual(gotTexts, tt.wantTexts) {
					t.Errorf("%s: got matched texts %v, want %v", tt.description, gotTexts, tt.wantTexts)
				}
			}

			// Verify all results have valid line numbers
			for i, result := range results {
				if result.positionParams.Position.Line == 0 {
					t.Errorf("%s: result %d has invalid line number 0", tt.description, i)
				}
			}
		})
	}
}

func Test_builtinServiceClient_performFileContentSearch_Multiline(t *testing.T) {
	baseLocation := filepath.Join(".", "testdata")
	baseLocation, err := filepath.Abs(baseLocation)
	if err != nil {
		t.Fatalf("unable to get absolute path for testdata: %v", err)
	}

	tests := []struct {
		name         string
		pattern      string
		filePattern  string
		wantMatches  int
		wantLineNums []int // expected line numbers for matches
		description  string
	}{
		{
			name:         "multiline pattern matches across newlines",
			pattern:      `<Masthead[^>]*>[^<]*<MastheadToggle`,
			filePattern:  `\.(j|t)sx?$`,
			wantMatches:  2,
			wantLineNums: []int{8, 14}, // lines 8 and 14 (1-based)
			description:  "Should match both multiline (line 8) and single-line (line 14) JSX",
		},
		{
			name:         "multiline pattern with alternative matches MastheadBrand",
			pattern:      `<Masthead[^>]*>[^<]*<MastheadBrand`,
			filePattern:  `\.(j|t)sx?$`,
			wantMatches:  1,
			wantLineNums: []int{17}, // line 17 only (1-based) - line 8 has MastheadToggle in between
			description:  "Should match multiline patterns with lots of whitespace",
		},
		{
			name:         "combined pattern with OR",
			pattern:      `<Masthead[^>]*>[^<]*<MastheadToggle|<Masthead[^>]*>[^<]*<MastheadBrand`,
			filePattern:  `\.(j|t)sx?$`,
			wantMatches:  3,
			wantLineNums: []int{8, 14, 17}, // 8=multiline MastheadToggle, 14=single-line MastheadToggle, 17=multiline MastheadBrand (all 1-based)
			description:  "Should match all multiline and single-line occurrences",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := &builtinServiceClient{
				config: provider.InitConfig{
					Location: baseLocation,
				},
				log:            testr.New(t),
				locationCache:  map[string]float64{},
				cacheMutex:     sync.RWMutex{},
				workingCopyMgr: NewTempFileWorkingCopyManger(testr.New(t)),
			}

			condition := builtinCondition{
				Filecontent: fileContentCondition{
					Pattern:     tt.pattern,
					FilePattern: tt.filePattern,
				},
			}

			conditionInfo, err := yaml.Marshal(&condition)
			if err != nil {
				t.Fatalf("failed to marshal condition: %v", err)
			}

			got, err := sc.Evaluate(context.TODO(), "filecontent", conditionInfo)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}

			if len(got.Incidents) != tt.wantMatches {
				t.Errorf("%s: got %d matches, want %d matches", tt.description, len(got.Incidents), tt.wantMatches)
				for i, incident := range got.Incidents {
					t.Logf("  Match %d: line %d, file %s", i+1, incident.LineNumber, incident.FileURI.Filename())
				}
			}

			// Verify line numbers
			gotLineNums := []int{}
			for _, incident := range got.Incidents {
				if filepath.Base(incident.FileURI.Filename()) == "multiline-test.tsx" {
					gotLineNums = append(gotLineNums, *incident.LineNumber)
				}
			}
			sort.Ints(gotLineNums)

			if !reflect.DeepEqual(gotLineNums, tt.wantLineNums) {
				t.Errorf("%s: got line numbers %v, want %v", tt.description, gotLineNums, tt.wantLineNums)
			}
		})
	}
}

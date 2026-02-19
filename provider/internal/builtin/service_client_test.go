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
			name:       "(XML) Include using the config (legacy inclusion), no exclude",
			capability: "xml",
			includedPathsFromConfig: []string{
				"dir_b",
			},
			condition: builtinCondition{
				XML: xmlCondition{
					XPath: "//*/b:simple[text()=matches(self::node(), '*property*')]",
				},
			},
			chainTemplate: engine.ChainTemplate{},
			wantFilePaths: []string{},
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
			chainTemplate: engine.ChainTemplate{
				Filepaths: []string{
					filepath.Join("dir_b", "dir_a", "ba.xml"),
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
				log:            testr.New(t),
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

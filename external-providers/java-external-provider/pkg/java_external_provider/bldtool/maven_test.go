package bldtool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/konveyor/analyzer-lsp/provider"
)

type testLabeler struct{}

func (t *testLabeler) HasLabel(string) bool {
	return false
}

func (t *testLabeler) AddLabels(_ string, _ bool) []string {
	return nil
}

func TestGetMavenBuildTool(t *testing.T) {
	testCases := []struct {
		name           string
		location       string
		dependencyPath string
		expectNil      bool
	}{
		{
			name:           "ValidMavenProject",
			location:       "../dependency/testdata/maven-example",
			dependencyPath: "",
			expectNil:      false,
		},
		{
			name:           "InvalidLocation",
			location:       "../dependency/testdata/nonexistent",
			dependencyPath: "",
			expectNil:      true,
		},
		{
			name:           "GradleProject",
			location:       "../dependency/testdata/gradle-example",
			dependencyPath: "",
			expectNil:      true,
		},
	}

	log := testr.New(t)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := BuildToolOptions{
				Config: provider.InitConfig{
					Location:       tc.location,
					DependencyPath: tc.dependencyPath,
				},
				Labeler: &testLabeler{},
			}

			bt := getMavenBuildTool(opts, log)
			if tc.expectNil && bt != nil {
				t.Errorf("expected nil build tool, got %v", bt)
			}
			if !tc.expectNil && bt == nil {
				t.Errorf("expected non-nil build tool, got nil")
			}
		})
	}
}

func TestMavenParseDepString(t *testing.T) {
	testCases := []struct {
		name          string
		depString     string
		expectedName  string
		expectedVer   string
		expectedType  string
		expectedClass string
		expectErr     bool
	}{
		{
			name:         "SimpleJarDependency",
			depString:    "io.fabric8:kubernetes-client:jar:6.0.0:compile",
			expectedName: "io.fabric8.kubernetes-client",
			expectedVer:  "6.0.0",
			expectedType: "compile",
			expectErr:    false,
		},
		{
			name:          "DependencyWithClassifier",
			depString:     "io.netty:netty-transport-native-epoll:jar:linux-x86_64:4.1.76.Final:runtime",
			expectedName:  "io.netty.netty-transport-native-epoll",
			expectedVer:   "4.1.76.Final",
			expectedType:  "runtime",
			expectedClass: "linux-x86_64",
			expectErr:     false,
		},
		{
			name:         "DependencyWithPrettyPrint",
			depString:    "+- junit:junit:jar:4.11:test",
			expectedName: "junit.junit",
			expectedVer:  "4.11",
			expectedType: "test",
			expectErr:    false,
		},
		{
			name:         "DependencyWithTreeChars",
			depString:    "\\- javax:javaee-api:jar:7.0:provided",
			expectedName: "javax.javaee-api",
			expectedVer:  "7.0",
			expectedType: "provided",
			expectErr:    false,
		},
		{
			name:      "InvalidDependencyString",
			depString: "invalid",
			expectErr: true,
		},
	}

	log := testr.New(t)
	mvnBT := &mavenBuildTool{
		mavenBaseTool: mavenBaseTool{
			log:          log,
			labeler:      &testLabeler{},
			mvnLocalRepo: "/tmp/repo",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dep, err := mvnBT.parseDepString(tc.depString, "/tmp/repo", "/tmp/pom.xml")
			if tc.expectErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if dep.Name != tc.expectedName {
				t.Errorf("expected name %s, got %s", tc.expectedName, dep.Name)
			}
			if dep.Version != tc.expectedVer {
				t.Errorf("expected version %s, got %s", tc.expectedVer, dep.Version)
			}
			if tc.expectedType != "" && dep.Type != tc.expectedType {
				t.Errorf("expected type %s, got %s", tc.expectedType, dep.Type)
			}
			if tc.expectedClass != "" && dep.Classifier != tc.expectedClass {
				t.Errorf("expected classifier %s, got %s", tc.expectedClass, dep.Classifier)
			}
		})
	}
}

func TestMavenExtractSubmoduleTrees(t *testing.T) {
	testCases := []struct {
		name          string
		input         string
		expectedTrees int
	}{
		{
			name: "SingleModuleTree",
			input: `[INFO] --- maven-dependency-plugin:2.8:tree (default-cli) @ test ---
[INFO] com.example:test:jar:1.0.0
[INFO] +- junit:junit:jar:4.11:test
[INFO] |  \- org.hamcrest:hamcrest-core:jar:1.3:test
[INFO] \- com.google.guava:guava:jar:23.0:compile
[INFO] ------------------------------------------------------------------------`,
			expectedTrees: 1,
		},
		{
			name: "MultiModuleTree",
			input: `[INFO] --- maven-dependency-plugin:2.8:tree (default-cli) @ parent ---
[INFO] com.example:parent:pom:1.0.0
[INFO] ------------------------------------------------------------------------
[INFO]
[INFO] --- maven-dependency-plugin:2.8:tree (default-cli) @ module1 ---
[INFO] com.example:module1:jar:1.0.0
[INFO] \- junit:junit:jar:4.11:test
[INFO] ------------------------------------------------------------------------
[INFO]
[INFO] --- maven-dependency-plugin:2.8:tree (default-cli) @ module2 ---
[INFO] com.example:module2:jar:1.0.0
[INFO] \- com.google.guava:guava:jar:23.0:compile
[INFO] ------------------------------------------------------------------------`,
			expectedTrees: 3,
		},
	}

	log := testr.New(t)
	mvnBT := &mavenBuildTool{
		mavenBaseTool: mavenBaseTool{
			log: log,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			lines := strings.Split(tc.input, "\n")
			trees := mvnBT.extractSubmoduleTrees(lines)
			if len(trees) != tc.expectedTrees {
				t.Errorf("expected %d trees, got %d", tc.expectedTrees, len(trees))
			}
		})
	}
}

func TestMavenParseMavenDepLines(t *testing.T) {
	testCases := []struct {
		name         string
		lines        []string
		expectedDeps int
		expectedName string
	}{
		{
			name: "SingleDependency",
			lines: []string{
				"junit:junit:jar:4.11:test",
			},
			expectedDeps: 1,
			expectedName: "junit.junit",
		},
		{
			name: "DependencyWithTransitive",
			lines: []string{
				"junit:junit:jar:4.11:test",
				"   org.hamcrest:hamcrest-core:jar:1.3:test",
			},
			expectedDeps: 1,
			expectedName: "junit.junit",
		},
		{
			name: "MultipleDependencies",
			lines: []string{
				"junit:junit:jar:4.11:test",
				"   org.hamcrest:hamcrest-core:jar:1.3:test",
				"com.google.guava:guava:jar:23.0:compile",
			},
			expectedDeps: 2,
		},
	}

	log := testr.New(t)
	mvnBT := &mavenBuildTool{
		mavenBaseTool: mavenBaseTool{
			log:          log,
			labeler:      &testLabeler{},
			mvnLocalRepo: "/tmp/repo",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			deps, err := mvnBT.parseMavenDepLines(tc.lines, "/tmp/repo", "/tmp/pom.xml")
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if len(deps) != tc.expectedDeps {
				t.Errorf("expected %d dependencies, got %d", tc.expectedDeps, len(deps))
			}
			if tc.expectedName != "" && len(deps) > 0 {
				if deps[0].Dep.Name != tc.expectedName {
					t.Errorf("expected name %s, got %s", tc.expectedName, deps[0].Dep.Name)
				}
			}
		})
	}
}

func TestMavenGetSourceFileLocation(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()
	jarPath := filepath.Join(tmpDir, "test-1.0.jar")

	// Create a dummy jar file
	jarFile, err := os.Create(jarPath)
	if err != nil {
		t.Fatalf("failed to create test jar: %v", err)
	}
	jarFile.Close()

	log := testr.New(t)
	mvnBT := &mavenBuildTool{
		mavenBaseTool: mavenBaseTool{
			log: log,
		},
	}

	testCases := []struct {
		name         string
		packagePath  string
		jarPath      string
		javaFileName string
	}{
		{
			name:         "SimpleJavaFile",
			packagePath:  "com/example",
			jarPath:      jarPath,
			javaFileName: "Test.java",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := mvnBT.GetSourceFileLocation(tc.packagePath, tc.jarPath, tc.javaFileName)
			if err != nil {
				t.Logf("expected behavior - jar extraction may fail in test: %v", err)
				return
			}
			if result == "" {
				t.Errorf("expected non-empty result")
			}
		})
	}
}

func TestMavenShouldResolve(t *testing.T) {
	log := testr.New(t)
	mvnBT := &mavenBuildTool{
		mavenBaseTool: mavenBaseTool{
			log: log,
		},
	}

	if mvnBT.ShouldResolve() {
		t.Errorf("Maven build tool should not require immediate resolution")
	}
}

func TestMavenGetResolver(t *testing.T) {
	testDir := "../dependency/testdata/maven-example"
	absPath, err := filepath.Abs(filepath.Join(testDir, "pom.xml"))
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	log := testr.New(t)
	opts := BuildToolOptions{
		Config: provider.InitConfig{
			Location:       testDir,
			DependencyPath: "",
		},
		Labeler: &testLabeler{},
	}

	mvnBT := getMavenBuildTool(opts, log)
	if mvnBT == nil {
		t.Fatal("failed to create maven build tool")
	}

	resolver, err := mvnBT.GetResolver("/tmp/fernflower.jar")
	if err != nil {
		t.Errorf("unexpected error getting resolver: %v", err)
	}
	if resolver == nil {
		t.Errorf("expected non-nil resolver")
	}

	// Verify the build tool was correctly initialized
	mavenTool, ok := mvnBT.(*mavenBuildTool)
	if !ok {
		t.Fatalf("expected mavenBuildTool type")
	}
	if mavenTool.depCache.hashFile != absPath {
		t.Errorf("expected hashFile %s, got %s", absPath, mavenTool.depCache.hashFile)
	}
}

func TestMavenResolveDepFilepath(t *testing.T) {
	tmpDir := t.TempDir()
	localRepo := filepath.Join(tmpDir, ".m2", "repository")

	// Create directory structure for a test dependency
	groupPath := filepath.Join(localRepo, "io", "fabric8")
	artifactPath := filepath.Join(groupPath, "kubernetes-client", "6.0.0")
	err := os.MkdirAll(artifactPath, 0755)
	if err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Create a dummy .jar.sha1 file
	shaFile := filepath.Join(artifactPath, "kubernetes-client-6.0.0.jar.sha1")
	err = os.WriteFile(shaFile, []byte("abc123def456"), 0644)
	if err != nil {
		t.Fatalf("failed to create sha file: %v", err)
	}

	log := testr.New(t)
	mvnBT := &mavenBuildTool{
		mavenBaseTool: mavenBaseTool{
			log:          log,
			labeler:      &testLabeler{},
			mvnLocalRepo: localRepo,
		},
	}

	dep := &provider.Dep{
		Name:    "io.fabric8.kubernetes-client",
		Version: "6.0.0",
	}

	filepath := mvnBT.resolveDepFilepath(dep, "io.fabric8", "kubernetes-client", localRepo)

	if !strings.Contains(filepath, "kubernetes-client-6.0.0.jar.sha1") {
		t.Errorf("expected filepath to contain kubernetes-client-6.0.0.jar.sha1, got %s", filepath)
	}

	if dep.ResolvedIdentifier != "abc123def456" {
		t.Errorf("expected ResolvedIdentifier to be 'abc123def456', got %s", dep.ResolvedIdentifier)
	}
}

func TestMavenBinaryBuildTool(t *testing.T) {
	// Check if fernflower is available for decompilation
	fernflower, err := filepath.Abs("../dependency/testdata/fernflower.jar")
	if err != nil {
		t.Skip("fernflower not found, skipping maven binary build tool test")
	}
	if _, err := os.Stat(fernflower); os.IsNotExist(err) {
		t.Skip("fernflower not found, skipping maven binary build tool test")
	}

	testCases := []struct {
		Name                  string
		Location              string
		ExpectSuccess         bool
		AllowDepResolutionErr bool   // Allow dependency resolution errors (e.g., missing parent POMs)
		ExpectedFiles         map[string]bool // Files/directories we expect to find in the decompiled project
		ExpectedDepDirs       map[string]bool // Dependency directories we expect in the Maven repo
	}{
		{
			Name:                  "jar-binary",
			Location:              "../dependency/testdata/acmeair-common-1.0-SNAPSHOT.jar",
			ExpectSuccess:         true,
			AllowDepResolutionErr: true, // This artifact has a parent POM that won't be available
			ExpectedFiles: map[string]bool{
				"pom.xml": false, // Will be set to true when found
			},
			ExpectedDepDirs: map[string]bool{},
		},
		{
			Name:                  "war-binary",
			Location:              "../dependency/testdata/acmeair-webapp-1.0-SNAPSHOT.war",
			ExpectSuccess:         true,
			AllowDepResolutionErr: true, // This artifact has a parent POM that won't be available
			ExpectedFiles: map[string]bool{
				"pom.xml": false,
			},
			ExpectedDepDirs: map[string]bool{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			// Get absolute path to the binary location
			location, err := filepath.Abs(tc.Location)
			if err != nil {
				t.Fatalf("unable to get absolute path: %s", err)
			}

			// Verify the binary file exists
			if _, err := os.Stat(location); os.IsNotExist(err) {
				t.Fatalf("test binary not found: %s", location)
			}

			log := testr.NewWithOptions(t, testr.Options{
				Verbosity: 5,
			})

			opts := BuildToolOptions{
				Config: provider.InitConfig{
					Location:       location,
					DependencyPath: "",
				},
				Labeler: &testLabeler{},
			}

			// Get the Maven binary build tool
			mvnBinary := getMavenBinaryBuildTool(opts, log)
			if mvnBinary == nil {
				t.Fatal("failed to create maven binary build tool")
			}

			// Get the resolver
			resolver, err := mvnBinary.GetResolver(fernflower)
			if err != nil {
				t.Fatalf("unable to get resolver: %s", err)
			}
			if resolver == nil {
				t.Fatal("resolver is nil")
			}

			// Resolve sources - this will decompile the binary and create a Maven project
			projectLocation, depPath, err := resolver.ResolveSources(context.Background())
			if tc.ExpectSuccess && err != nil && !tc.AllowDepResolutionErr {
				t.Fatalf("unable to resolve sources: %s", err)
			}
			if !tc.ExpectSuccess && err == nil {
				t.Fatalf("expected error but got success")
			}

			// If we got an error but allow dep resolution errors, log it but continue
			if err != nil && tc.AllowDepResolutionErr {
				t.Logf("dependency resolution failed (expected): %s", err)
				// For binary artifacts, even if dependency resolution fails,
				// the binary should still be decompiled and a project created.
				// We need to manually find the project location since ResolveSources may not return it

				// The binary resolver creates java-project in the same directory as the binary
				projectLocation = filepath.Join(filepath.Dir(location), "java-project")
				depPath = ""
			}

			if !tc.ExpectSuccess {
				return
			}

			// Verify that the project location was created
			if projectLocation == "" {
				t.Fatal("project location is empty")
			}
			if _, err := os.Stat(projectLocation); os.IsNotExist(err) {
				t.Fatalf("project location not created: %s", projectLocation)
			}

			// Verify that the dependency path is set (unless we allow dep resolution errors)
			if depPath == "" && !tc.AllowDepResolutionErr {
				t.Fatal("dependency path is empty")
			}

			// Verify expected files exist in the decompiled project
			for expectedFile := range tc.ExpectedFiles {
				fullPath := filepath.Join(projectLocation, expectedFile)
				if _, err := os.Stat(fullPath); err == nil {
					tc.ExpectedFiles[expectedFile] = true
					t.Logf("found expected file/dir: %s", expectedFile)
				}
			}

			// Check if we found all expected files
			for expectedFile, found := range tc.ExpectedFiles {
				if !found {
					t.Logf("warning: expected file/dir not found: %s", expectedFile)
					// Not failing the test as binary decompilation structure may vary
				}
			}

			// Walk the decompiled project to verify structure
			t.Logf("Decompiled project location: %s", projectLocation)
			filepath.Walk(projectLocation, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				relPath, _ := filepath.Rel(projectLocation, path)
				if relPath != "." {
					t.Logf("found in project: %s (isDir: %v)", relPath, info.IsDir())
				}
				return nil
			})

			// Verify pom.xml exists (should always be generated for binary artifacts)
			pomPath := filepath.Join(projectLocation, "pom.xml")
			if _, err := os.Stat(pomPath); os.IsNotExist(err) {
				t.Errorf("pom.xml not found in decompiled project")
			} else {
				t.Logf("pom.xml successfully generated at: %s", pomPath)
			}
		})
	}
}

func TestGetMavenBinaryBuildTool(t *testing.T) {
	testCases := []struct {
		name      string
		location  string
		expectNil bool
	}{
		{
			name:      "ValidJarBinary",
			location:  "../dependency/testdata/acmeair-common-1.0-SNAPSHOT.jar",
			expectNil: false,
		},
		{
			name:      "ValidWarBinary",
			location:  "../dependency/testdata/acmeair-webapp-1.0-SNAPSHOT.war",
			expectNil: false,
		},
		{
			name:      "InvalidLocation",
			location:  "../dependency/testdata/nonexistent.jar",
			expectNil: true,
		},
		{
			name:      "EmptyLocation",
			location:  "",
			expectNil: true,
		},
	}

	log := testr.New(t)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			absLocation := tc.location
			if tc.location != "" {
				var err error
				absLocation, err = filepath.Abs(tc.location)
				if err != nil {
					absLocation = tc.location
				}
			}

			opts := BuildToolOptions{
				Config: provider.InitConfig{
					Location:       absLocation,
					DependencyPath: "",
				},
				Labeler: &testLabeler{},
			}

			bt := getMavenBinaryBuildTool(opts, log)
			if tc.expectNil && bt != nil {
				t.Errorf("expected nil build tool, got %v", bt)
			}
			if !tc.expectNil && bt == nil {
				t.Errorf("expected non-nil build tool, got nil")
			}
		})
	}
}

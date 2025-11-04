package dependency

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/go-logr/logr/testr"
)

func TestBinaryResolver(t *testing.T) {

	warProjectOutputWithPomXML := map[string]any{
		"pom.xml": nil,
	}
	for key := range warProjectOutput {
		warProjectOutputWithPomXML[key] = nil
	}
	testCases := []struct {
		Name        string
		Location    string
		testProject testProject
		mavenDir    testMavenDir
	}{
		{
			Name:        "jar-binary",
			Location:    "testdata/acmeair-common-1.0-SNAPSHOT.jar",
			testProject: testProject{output: jarProjectOutput},
			mavenDir:    testMavenDir{output: jarProjectMavenDir},
		},
		{
			Name:        "war-binary",
			Location:    "testdata/acmeair-webapp-1.0-SNAPSHOT.war",
			testProject: testProject{output: warProjectOutputWithPomXML},
			mavenDir:    testMavenDir{output: warProjectMavenDir},
		},
		/*{
			Name:        "ear-binary",
			Location:    "testdata/jee-example-app-1.0.0.ear",
			testProject: testProject{output: earProjectOutput},
			mavenDir:    testMavenDir{output: earProjectMavenDir},
		},*/
	}

	for _, test := range testCases {
		fernflower, err := filepath.Abs("testdata/fernflower.jar")
		if err != nil {
			t.Fatalf("can't find fernflower in testdata")
		}
		t.Run(test.Name, func(t *testing.T) {
			mavenDir := t.TempDir()

			projectTmpDir := t.TempDir()
			projectDir := filepath.Join(projectTmpDir, "java-project")

			fileName := filepath.Base(test.Location)
			newLocation := filepath.Join(projectTmpDir, fileName)
			err := CopyFile(test.Location, newLocation)
			if err != nil {
				t.Fail()
			}

			resolver := GetBinaryResolver(ResolverOptions{
				Log: testr.NewWithOptions(t, testr.Options{
					Verbosity: 2,
				}),
				Location:       filepath.Clean(newLocation),
				DecompileTool:  fernflower,
				Labeler:        &testLabeler{},
				LocalRepo:      filepath.Clean(mavenDir),
				Insecure:       false,
				MavenIndexPath: "test",
			})
			if err != nil {
				t.Logf("unable to get resolver: %s", err)
				t.Fail()
			}

			location, depPath, err := resolver.ResolveSources(context.Background())
			if err != nil {
				t.Logf("unable to resolve source: %s", err)
				t.Fail()
			}

			if location != projectDir {
				t.Logf("unable to get ExpectedLocation\nexpected: %s\nactual: %s", projectDir, location)
				t.Fail()
			}

			if depPath != mavenDir {
				t.Logf("unable to get ExpectedLocalRepo\nexpected: %s\nactual: %s", mavenDir, depPath)
				t.Fail()
			}
			test.testProject.matchProject(projectDir, t)
			missed := test.testProject.foundAllFiles()
			if len(missed) > 0 {
				t.Logf("missed: %#v", missed)
				t.Fail()
			}
			test.mavenDir.matchMavenDir(mavenDir, t)
			missed = test.mavenDir.foundAllFiles()
			if len(missed) > 0 {
				t.Logf("missed: %#v", missed)
				t.Fail()
			}
		})
	}
}

func TestMavenResolver(t *testing.T) {
	// Skip if maven is not installed
	if _, err := exec.LookPath("mvn"); err != nil {
		t.Skip("maven not found, skipping maven resolver test")
	}

	testCases := []struct {
		Name     string
		Location string
		// A non exhaustive list, but make sure that these sources exist
		expectedSources map[string]any
	}{
		{
			Name:     "maven-multi-module",
			Location: "testdata/maven-example",

			expectedSources: map[string]any{
				"io/fabric8/kubernetes-client/6.0.0/kubernetes-client-6.0.0-sources.jar": nil,
				"io/fabric8/kubernetes-client/6.0.0/kubernetes-client-6.0.0.jar":         nil,
			},
		},
		{
			Name:     "maven-unavailable-dependency",
			Location: "testdata/maven-unavailable-dep",

			expectedSources: map[string]any{
				"junit/junit/4.13.2/junit-4.13.2-sources.jar": nil,
				"junit/junit/4.13.2/junit-4.13.2.jar":         nil,
			},
		},
	}

	for _, test := range testCases {
		fernflower, err := filepath.Abs("testdata/fernflower.jar")
		if err != nil {
			t.Fatalf("can't find fernflower in testdata")
		}

		t.Run(test.Name, func(t *testing.T) {
			mavenDir := t.TempDir()

			t.Setenv("MAVEN_OPTS", fmt.Sprintf("-Dmaven.repo.local=%v", mavenDir))

			location, err := filepath.Abs(test.Location)
			if err != nil {
				t.Fatalf("unable to get absolute path: %s", err)
			}

			resolver := GetMavenResolver(ResolverOptions{
				Log: testr.NewWithOptions(t, testr.Options{
					Verbosity: 20,
				}),
				Location:       filepath.Clean(location),
				DecompileTool:  fernflower,
				Labeler:        &testLabeler{},
				LocalRepo:      filepath.Clean(mavenDir),
				Insecure:       false,
				MavenIndexPath: "testdata",
			})

			projectLocation, depPath, err := resolver.ResolveSources(context.Background())
			if err != nil {
				t.Logf("unable to resolve sources: %s", err)
				t.Fail()
			}

			// Verify that the project location is the original location
			if projectLocation != location {
				t.Logf("unexpected project location\nexpected: %s\nactual: %s", location, projectLocation)
				t.Fail()
			}

			// Verify that the dependency path is the maven local repo
			if depPath != mavenDir {
				t.Logf("unexpected dependency path\nexpected: %s\nactual: %s", mavenDir, depPath)
				t.Fail()
			}

			// Verify that dependencies were downloaded to the local repo
			if _, err := os.Stat(mavenDir); os.IsNotExist(err) {
				t.Logf("maven local repo not created")
				t.Fail()
			}
			t.Logf("looping maven dir")
			// I want to verify that the sources are put in place correctly as well.
			filepath.Walk(mavenDir, func(path string, info fs.FileInfo, err error) error {
				relPath, err := filepath.Rel(mavenDir, path)
				if err != nil {
					t.Fatalf("unable to get relative path")
				}
				found := false
				for k := range test.expectedSources {
					if k == relPath {
						t.Logf("path: %v", relPath)
						found = true
						break
					}

					if strings.Contains(k, relPath) {
						return nil
					}
				}
				if found {
					test.expectedSources[relPath] = "a"
					return nil
				}
				return nil
			})
			for k, v := range test.expectedSources {
				if v == nil {
					t.Logf("unable to find: %s", k)
					t.Fail()
				}
			}
		})
	}
}

func TestGradleResolver(t *testing.T) {
	// Skip if gradle wrapper is not available
	gradleWrapper := "testdata/gradle-example/gradlew"
	if _, err := os.Stat(gradleWrapper); os.IsNotExist(err) {
		t.Skip("gradle wrapper not found, skipping gradle resolver test")
	}

	testCases := []struct {
		Name               string
		Location           string
		expectedSourcesJar map[string]any
	}{
		{
			Name:               "gradle-multi-project",
			Location:           "testdata/gradle-example",
			expectedSourcesJar: map[string]any{"error_prone_annotations-2.0.18-sources.jar": nil, "j2objc-annotations-1.1-sources.jar": nil},
		},
		{
			Name:               "gradle-multi-project",
			Location:           "testdata/gradle-example-v9",
			expectedSourcesJar: map[string]any{"error_prone_annotations-2.0.18-sources.jar": nil, "j2objc-annotations-1.1-sources.jar": nil},
		},
	}

	for _, test := range testCases {
		fernflower, err := filepath.Abs("testdata/fernflower.jar")
		if err != nil {
			t.Fatalf("can't find fernflower in testdata")
		}

		t.Run(test.Name, func(t *testing.T) {
			gradleHome := t.TempDir()
			gradleDepCache := filepath.Join(gradleHome, "caches", "modules-2")

			t.Setenv("GRADLE_USER_HOME", gradleHome)
			location, err := filepath.Abs(test.Location)
			if err != nil {
				t.Fatalf("unable to get absolute path: %s", err)
			}

			buildFile := filepath.Join(location, "build.gradle")
			wrapper, err := filepath.Abs(filepath.Join(location, "gradlew"))
			if err != nil {
				t.Fatalf("unable to get gradle wrapper path: %s", err)
			}

			// Get JAVA_HOME from environment or use a default
			javaHome := os.Getenv("JAVA_HOME")
			if javaHome == "" {
				t.Skip("JAVA_HOME not set, skipping gradle resolver test")
			}

			taskFile, err := filepath.Abs("../../../gradle/build.gradle")
			if err != nil {
				t.Fatalf("unable to get task file path: %s", err)
			}

			resolver := GetGradleResolver(ResolverOptions{
				Log: testr.NewWithOptions(t, testr.Options{
					Verbosity: 5,
				}),
				Location:       filepath.Clean(location),
				BuildFile:      buildFile,
				Wrapper:        wrapper,
				JavaHome:       javaHome,
				DecompileTool:  fernflower,
				Labeler:        &testLabeler{},
				GradleTaskFile: taskFile,
			})

			projectLocation, gradleCache, err := resolver.ResolveSources(context.Background())
			if err != nil {
				// Check if this is a Java version compatibility issue with the old Gradle wrapper
				if contains := regexp.MustCompile("Could not determine java version").MatchString(err.Error()); contains {
					t.Skip("Gradle wrapper version incompatible with current Java version")
				}
				t.Logf("unable to resolve sources: %s", err)
				t.Fail()
			}

			// Verify that the project location is the original location
			if projectLocation != location {
				t.Logf("unexpected project location\nexpected: %s\nactual: %s", location, projectLocation)
				t.Fail()
			}

			if gradleCache != gradleDepCache {
				t.Logf("unexpected gradle cache \nexpected: %s\nactual: %s", gradleDepCache, gradleCache)
				t.Fail()
			}

			filepath.Walk(gradleCache, func(path string, info fs.FileInfo, err error) error {
				found := false
				for k := range test.expectedSourcesJar {
					if filepath.Base(path) == k {
						found = true
						break
					}
				}
				if found {
					test.expectedSourcesJar[filepath.Base(path)] = "a"
				}
				return nil
			})
		})
	}
}

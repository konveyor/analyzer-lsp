package dependency

import (
	"context"
	"path/filepath"
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

//func TestMavenResolver(t *testing.T)  {}
//func TestGradleResolver(t *testing.T) {}

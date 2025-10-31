package dependency

import (
	"reflect"
	"testing"

	"github.com/go-logr/logr/testr"
)

// BenchmarkConstructArtifactFromSHA benchmarks the constructArtifactFromSHA function
// with different scenarios to measure performance characteristics.
func TestConstructArtifactFromSHA(t *testing.T) {

	testCases := []struct {
		name           string
		jarFile        string
		mavenIndexPath string
		shouldFind     bool
		value          JavaArtifact
	}{
		{
			name:           "InIndex",
			jarFile:        "testdata/should_find_in_index.jar",
			mavenIndexPath: "testdata",
			shouldFind:     true,
			value: JavaArtifact{
				FoundOnline: true,
				GroupId:     "org.springframework",
				ArtifactId:  "spring-core",
				Version:     "3.1.2.RELEASE",
				Sha1:        "dd4295f0567deb2cc629dd647d2f055268c2fd3e",
			},
		},
		{
			name:           "NotInIndex",
			jarFile:        "testdata/will_not_find.jar.jar",
			mavenIndexPath: "testdata",
			shouldFind:     false,
		},
	}

	log := testr.New(t)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			val, err := constructArtifactFromSHA(log, tc.jarFile, tc.mavenIndexPath)
			if err != nil && !tc.shouldFind {
				return
			}
			if err != nil {
				t.Fail()
			}
			if !tc.shouldFind {
				t.Fail()
			}
			if !reflect.DeepEqual(val, tc.value) {
				t.Fail()
			}
		})
	}
}

package dependency

import (
	"testing"

	"github.com/go-logr/logr"
)

// BenchmarkConstructArtifactFromSHA benchmarks the constructArtifactFromSHA function
// with different scenarios to measure performance characteristics.
func BenchmarkConstructArtifactFromSHA(b *testing.B) {
	log := logr.Discard()

	benchmarks := []struct {
		name           string
		jarFile        string
		mavenIndexPath string
	}{
		{
			name:           "InIndex",
			jarFile:        "testdata/should_find_in_index.jar",
			mavenIndexPath: "testdata",
		},
		{
			name:           "NotInIndex",
			jarFile:        "testdata/will_not_find.jar.jar",
			mavenIndexPath: "testdata",
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			// Reset timer to exclude setup time
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, _ = constructArtifactFromSHA(log, bm.jarFile, bm.mavenIndexPath)
			}
		})
	}
}

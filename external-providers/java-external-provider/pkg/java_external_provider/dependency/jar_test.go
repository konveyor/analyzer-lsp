package dependency

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/go-logr/logr/testr"
)

type noopDecompiler struct{}

func (n *noopDecompiler) internalDecompileIntoProject(_ context.Context, _, _ string, _ chan DecomplierResponse, _ *sync.WaitGroup) error {
	return nil
}

func (n *noopDecompiler) internalDecompile(_ context.Context, _ string, _ chan DecomplierResponse, _ *sync.WaitGroup) error {
	return nil
}

func (n *noopDecompiler) internalDecompileClasses(_ context.Context, _, _ string, _ chan DecomplierResponse, _ *sync.WaitGroup) error {
	return nil
}

func TestJarArtifactRun_InvalidDepRecovery_ErrCleared(t *testing.T) {
	log := testr.New(t)
	ctx := context.Background()
	tmpDir := t.TempDir()

	artifactPath := filepath.Join(tmpDir, "nonexistent.jar")
	mavenIndexPath := filepath.Join(tmpDir, "no-index")

	responseCh := make(chan DecomplierResponse, 1)
	j := &jarArtifact{
		baseArtifact: baseArtifact{
			artifactPath:        artifactPath,
			m2Repo:              tmpDir,
			decompileTool:       filepath.Join(tmpDir, "fernflower.jar"),
			javaPath:            "java",
			labeler:             &testLabeler{},
			mavenIndexPath:      mavenIndexPath,
			decompiler:          &noopDecompiler{},
			decompilerResponses: responseCh,
			decompilerWG:        &sync.WaitGroup{},
		},
	}

	go func() {
		_ = j.Run(ctx, log)
	}()

	resp := <-responseCh

	if resp.err != nil {
		t.Errorf("response err must be nil after err != nil recovery block (jar.go); got: %v", resp.err)
	}
	if len(resp.Artifacts) != 1 {
		t.Fatalf("expected exactly one artifact, got %d", len(resp.Artifacts))
	}
	art := resp.Artifacts[0]
	if art.GroupId != EMBEDDED_KONVEYOR_GROUP {
		t.Errorf("expected GroupId %q, got %q", EMBEDDED_KONVEYOR_GROUP, art.GroupId)
	}
	if art.ArtifactId != "nonexistent" {
		t.Errorf("expected ArtifactId from jar filename %q, got %q", "nonexistent", art.ArtifactId)
	}
	if art.Version != "0.0.0-SNAPSHOT" {
		t.Errorf("expected Version %q, got %q", "0.0.0-SNAPSHOT", art.Version)
	}
	if art.FoundOnline {
		t.Error("expected FoundOnline false for dummy artifact")
	}
}

func TestJarArtifact_getJarDestPath(t *testing.T) {
	m2Repo := "/tmp/m2"
	j := &jarArtifact{
		baseArtifact: baseArtifact{m2Repo: m2Repo},
	}

	tests := []struct {
		name string
		dep  JavaArtifact
		want string
	}{
		{
			name: "standard coordinates",
			dep:  JavaArtifact{GroupId: "org.springframework", ArtifactId: "spring-core", Version: "5.3.1"},
			want: filepath.Join(m2Repo, "org", "springframework", "spring-core", "5.3.1", "spring-core-5.3.1.jar"),
		},
		{
			name: "embedded dummy coordinates",
			dep:  JavaArtifact{GroupId: EMBEDDED_KONVEYOR_GROUP, ArtifactId: "my-lib", Version: "0.0.0-SNAPSHOT"},
			want: filepath.Join(m2Repo, "io", "konveyor", "embededdep", "my-lib", "0.0.0-SNAPSHOT", "my-lib-0.0.0-SNAPSHOT.jar"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := j.getJarDestPath(tt.dep)
			if got != tt.want {
				t.Errorf("getJarDestPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestJarArtifact_getSourcesJarDestPath(t *testing.T) {
	m2Repo := "/tmp/m2"
	j := &jarArtifact{
		baseArtifact: baseArtifact{m2Repo: m2Repo},
	}

	tests := []struct {
		name string
		dep  JavaArtifact
		want string
	}{
		{
			name: "standard coordinates",
			dep:  JavaArtifact{GroupId: "org.springframework", ArtifactId: "spring-core", Version: "5.3.1"},
			want: filepath.Join(m2Repo, "org", "springframework", "spring-core", "5.3.1", "spring-core-5.3.1-sources.jar"),
		},
		{
			name: "embedded dummy coordinates",
			dep:  JavaArtifact{GroupId: EMBEDDED_KONVEYOR_GROUP, ArtifactId: "my-lib", Version: "0.0.0-SNAPSHOT"},
			want: filepath.Join(m2Repo, "io", "konveyor", "embededdep", "my-lib", "0.0.0-SNAPSHOT", "my-lib-0.0.0-SNAPSHOT-sources.jar"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := j.getSourcesJarDestPath(tt.dep)
			if got != tt.want {
				t.Errorf("getSourcesJarDestPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

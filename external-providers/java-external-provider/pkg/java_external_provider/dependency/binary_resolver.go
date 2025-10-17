package dependency

import (
	"context"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency/labels"
)

type binaryDependencyResolver struct {
	decompileTool      string
	labeler            labels.Labeler
	localRepo          string
	log                logr.Logger
	settingsFile       string
	insecure           bool
	location           string
	cleanBin           bool
	disableMavenSearch bool
}

func GetBinaryResolver(options ResolverOptions) Resolver {
	log := options.Log.WithName("binary-resolver")
	return &binaryDependencyResolver{
		localRepo:          options.LocalRepo,
		settingsFile:       options.BuildFile,
		insecure:           options.Insecure,
		location:           options.Location,
		log:                log,
		decompileTool:      options.DecompileTool,
		labeler:            options.Labeler,
		disableMavenSearch: options.DisableMavenSearch,
	}
}

func (m *binaryDependencyResolver) ResolveSources(ctx context.Context) (string, string, error) {
	projectPath := filepath.Join(filepath.Dir(m.location), "java-project")
	// And whatever else we need
	decompiler, err := getDecompiler(DecompilerOpts{
		DecompileTool:      m.decompileTool,
		log:                m.log,
		workers:            DefaultWorkerPoolSize,
		labler:             m.labeler,
		disableMavenSearch: m.disableMavenSearch,
		m2Repo:             m.localRepo,
	})
	if err != nil {
		return "", "", err
	}

	dependencies, err := decompiler.DecompileIntoProject(ctx, m.location, projectPath)
	if err != nil {
		return "", "", err
	}

	//removeIncompleteDependencies(deduplicateJavaArtifacts(deps))
	err = createJavaProject(ctx, projectPath, dependencies)
	if err != nil {
		m.log.Error(err, "failed to create java project", "path", projectPath)
		return "", "", err
	}
	m.log.V(5).Info("created java project", "path", projectPath)
	return projectPath, m.localRepo, nil
}

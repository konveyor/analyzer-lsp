package dependency

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/tracing"
)

type jarArtifact struct {
	baseArtifact
}

func (j *jarArtifact) Run(ctx context.Context, log logr.Logger) error {
	log = log.WithName("jar").WithValues("artifact", filepath.Base(j.artifactPath))
	defer j.decompilerWG.Done()
	jobCtx, span := tracing.StartNewSpan(ctx, "java-artifact-job")
	dep, err := ToDependency(ctx, log, j.labeler, j.artifactPath, j.disableMavenSearch)
	if err != nil {
		log.Error(err, "failed to add dep", "file", j.artifactPath)
		return err
	}
	// If Dep is not valid, then we need to make dummy values.
	if !dep.IsValid() {
		log.Info("failed to create maven coordinates -- using file to create dummy values", "file", j.artifactPath)
		name := j.getFileName()
		newDep := JavaArtifact{
			FoundOnline: false,
			Packaging:   "",
			GroupId:     EMBEDDED_KONVEYOR_GROUP,
			ArtifactId:  name,
			Version:     "0.0.0-SNAPSHOT",
			Sha1:        "",
		}
		dep = newDep
	}
	artifacts := []JavaArtifact{dep}
	if !dep.FoundOnline {
		sourceDestPath := j.getSourcesJarDestPath(dep)
		log.Info("getting sources", "souce-dst", sourceDestPath)
		if _, err := os.Stat(sourceDestPath); err == nil {
			log.Info("getting sources - allready found", "souce-dst", sourceDestPath)
			// already decompiled, duplicate...
			j.decompilerResponses <- DecomplierResponse{
				Artifacts:         []JavaArtifact{},
				ouputLocationBase: filepath.Base(sourceDestPath),
				err:               nil,
			}
			return nil
		}

		// This will tell fernflower to decompile the jar
		// into a new jar at the m2Repo for the dependency
		destinationPath := j.getM2Path(dep)
		log.Info("getting sources - allready found", "souce-dst", sourceDestPath, "destPath", destinationPath)
		if err = os.MkdirAll(destinationPath, DirPermRWXGrp); err != nil {
			log.Info("getting sources - can not create dir", "souce-dst", sourceDestPath, "destPath", destinationPath)
			return err
		}

		cmd := j.getDecompileCommand(jobCtx, j.artifactPath, destinationPath)
		log.Info("getting sources - allready found", "souce-dst", sourceDestPath, "destPath", destinationPath, "cmd", cmd)
		err := cmd.Run()
		if err != nil {
			log.Error(err, "failed to decompile file", "file", j.artifactPath)
			return err
		}
		err = j.renameSourcesJar(destinationPath, sourceDestPath)
		if err != nil {
			log.Info("getting sources rename failure", "souce-dst", sourceDestPath, "destPath", destinationPath, "cmd", cmd, "err", err)
			return err
		}
		log.Info("decompiled sources jar", "artifact", j.artifactPath, "sources", sourceDestPath)
	}
	// When we find a jar, and have a dep, we should pre-copy it to m2repo to reduce the network traffic.
	destPath := j.getJarDestPath(dep)
	if err := CopyFile(j.artifactPath, destPath); err != nil {
		log.Error(err, fmt.Sprintf("failed copying jar to %s", destPath))
		return err
	}
	log.Info("copied jar file", "src", j.artifactPath, "dest", destPath)

	j.decompilerResponses <- DecomplierResponse{
		Artifacts:         artifacts,
		ouputLocationBase: filepath.Base(destPath),
		err:               nil,
	}
	span.End()
	jobCtx.Done()
	return nil
}

func (j *jarArtifact) getJarDestPath(dep JavaArtifact) string {
	// Destination for this file during copy always goes to the m2Repo.
	return filepath.Join(j.getM2Path(dep), fmt.Sprintf("%s-%s.jar", dep.ArtifactId, dep.Version))
}

func (j *jarArtifact) getSourcesJarDestPath(dep JavaArtifact) string {
	return filepath.Join(j.getM2Path(dep), fmt.Sprintf("%s-%s-sources.jar", dep.ArtifactId, dep.Version))
}

func (j *jarArtifact) renameSourcesJar(destinationPath, sourcesDestPath string) error {
	// Fernflower keeps the jar name, whatever it is.
	jarName := filepath.Base(j.artifactPath)
	// the Director for the output is used as destination for fernflower.
	sourcesFile := filepath.Join(destinationPath, jarName)
	return moveFile(sourcesFile, sourcesDestPath)
}

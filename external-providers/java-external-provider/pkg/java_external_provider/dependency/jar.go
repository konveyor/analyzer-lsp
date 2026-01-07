package dependency

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/tracing"
)

type jarArtifact struct {
	baseArtifact
}

func (j *jarArtifact) Run(ctx context.Context, log logr.Logger) error {
	log = log.WithName("jar").WithValues("artifact", filepath.Base(j.artifactPath))
	jobCtx, span := tracing.StartNewSpan(ctx, "java-artifact-job")
	var err error
	var artifacts []JavaArtifact
	var outputLocationBase string
	defer func() {
		log.V(9).Info("Returning", "artifact", j.artifactPath)
		j.decompilerResponses <- DecomplierResponse{
			Artifacts:         artifacts,
			ouputLocationBase: outputLocationBase,
			err:               err,
		}
	}()

	dep, err := ToDependency(ctx, log, j.labeler, j.artifactPath, j.mavenIndexPath)
	if err != nil {
		log.Error(err, "failed to get dependnecy information", "file", j.artifactPath)
	}
	// If Dep is not valid, then we need to make dummy values.
	if !dep.IsValid() {
		log.Info("failed to create maven coordinates -- using file to create dummy values", "file", j.artifactPath, "dep", fmt.Sprintf("%#v", dep))
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
	artifacts = []JavaArtifact{dep}
	if !dep.FoundOnline {
		sourceDestPath := j.getSourcesJarDestPath(dep)
		outputLocationBase = filepath.Base(sourceDestPath)
		log.Info("getting sources", "souce-dst", sourceDestPath)
		if _, err := os.Stat(sourceDestPath); err == nil {
			log.Info("getting sources - allready found", "souce-dst", sourceDestPath)
			// already decompiled, duplicate...
			return nil
		}

		// This will tell fernflower to decompile the jar
		// into a new jar at the m2Repo/decompile for the dependency
		// fernflower keeps the same name, so you have to change it here.
		destinationPath := filepath.Join(j.getM2Path(dep), "decompile")
		log.Info("decompiling jar to source", "destPath", destinationPath)
		if err = os.MkdirAll(destinationPath, DirPermRWXGrp); err != nil {
			log.Info("getting sources - can not create dir", "destPath", destinationPath)
			return err
		}

		cmd := j.getDecompileCommand(jobCtx, j.artifactPath, destinationPath)
		err := cmd.Run()
		if err != nil {
			log.Error(err, "failed to decompile file", "file", j.artifactPath)
			return err
		}
		log.Info("decompiled sources jar", "artifact", j.artifactPath, "source-decomile-dir", destinationPath)
		// Fernflower as it decompiles, keeps the same name.
		if err := moveFile(filepath.Join(destinationPath, filepath.Base(j.artifactPath)), sourceDestPath); err != nil {
			log.Error(err, "unable to move decompiled artifact to correct location", "souce-jar", sourceDestPath)
			return err
		}
		log.Info("decompiled sources jar", "artifact", j.artifactPath, "source-jar", sourceDestPath)
	}

	// This will determine if the artifact is already in the m2repo or not. if it is then we don't need to try and copy it.
	if ok := strings.Contains(j.artifactPath, j.m2Repo); !ok {
		// When we find a jar, and have a dep, we should pre-copy it to m2repo to reduce the network traffic.
		destPath := j.getJarDestPath(dep)
		outputLocationBase = filepath.Base(destPath)
		if err := CopyFile(j.artifactPath, destPath); err != nil {
			log.Error(err, fmt.Sprintf("failed copying jar to %s", destPath))
			return err
		}
		log.Info("copied jar file", "src", j.artifactPath, "dest", destPath)
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

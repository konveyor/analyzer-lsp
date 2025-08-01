package java

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
)

type Source interface {
	GetLocation() string
	Decompile(context.Context) error
	SourceResolver
}

type sourceArgs struct {
	location            string
	fernflower          string
	mavenSettingsFiles  string
	m2Repo              string
	dependencyPath      string
	openSourceDepLabels map[string]*depLabelItem
	disableMavenSearch  bool
	mvnInsecure         bool
	log                 logr.Logger
}

func NewSource(args sourceArgs) (Source, error) {
	if args.dependencyPath == "" {
		args.dependencyPath = "pom.xml"
	}
	args.m2Repo = getMavenLocalRepoPath(args.mavenSettingsFiles)

	extension := strings.ToLower(path.Ext(args.location))
	switch extension {
	case JavaArchive:
		return javaArchiveSource{args}
	case WebArchive:
		return webArchiveSource{args}, nil
	case EnterpriseArchive:
		return enterpriseArchiveSource{args}, nil
	default:
		resolver := getSourceResolver(args)
		if resolver == nil {
			return nil, fmt.Errorf("unable to find build tool for location: %s", args.location)
		}
		return &sourceCodeSource{sourceArgs: args, SourceResolver: resolver}, nil
	}
}

func getMavenLocalRepoPath(mvnSettingsFile string) string {
	args := []string{
		"help:evaluate", "-Dexpression=settings.localRepository", "-q", "-DforceStdout",
	}
	if mvnSettingsFile != "" {
		args = append(args, "-s", mvnSettingsFile)
	}
	cmd := exec.Command("mvn", args...)
	var outb bytes.Buffer
	cmd.Stdout = &outb
	err := cmd.Run()
	if err != nil {
		return ""
	}

	// check errors
	return outb.String()
}

// TODO if this gets too long, then we need to break it out.
type javaArchiveSource struct {
	sourceArgs
	// SourceResolver must be set, once we decompile and determine the build tool
	SourceResolver
}

func (j *javaArchiveSource) Decompile(ctx context.Context) error {

}

type webArchiveSource struct {
	sourceArgs
	SourceResolver
}

type enterpriseArchiveSource struct {
	sourceArgs
	SourceResolver
}

type sourceCodeSource struct {
	sourceArgs
	SourceResolver
}

// Decompile does not need to happen if the source is sourse code.
func (s *sourceCodeSource) Decompile(context.Context) error {
	return nil
}

func (s sourceArgs) GetLocation() string {
	return s.location
}

// This will decompile the entire archive, which will
func decompileArchive(ctx context.Context, location, fernflower string) (string, error) {
	// multiple java versions may be installed - chose $JAVA_HOME one
	java := filepath.Join(os.Getenv("JAVA_HOME"), "bin", "java")
	// -mpm (max processing method) is required to keep decomp time low
	tempDir, err := os.MkdirTemp("", "decompile-archive")
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(
		ctx, java, "-jar", fernflower, "-mpm=30", location, tempDir)
	err = cmd.Run()
	if err != nil {
		return "", err
	}
	return tempDir, nil
}

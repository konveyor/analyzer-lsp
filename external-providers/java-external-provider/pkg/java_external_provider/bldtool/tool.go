package bldtool

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency/labels"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
)

// keys used in dep.Extras for extra information about a dep
const (
	artifactIdKey = "artifactId"
	groupIdKey    = "groupId"
	pomPathKey    = "pomPath"
	baseDepKey    = "baseDep"
)

const (
	maven  = "maven"
	gradle = "gradle"
)

const (
	gradleDepErr   = "gradleErr"
	fallbackDepErr = "fallbackDepErr"
)

// TODO; Replace the mvn URI stuff in provider with the
// a specific maven downloader tool.
type Downloader interface {
	Download(context.Context) (string, error)
}

type BuildTool interface {
	GetDependencies(context.Context) (map[uri.URI][]provider.DepDAGItem, error)
	UseCache() (bool, error)
	GetCachedDepError(errorCached map[string]error) (error, bool)
	GetLocalRepoPath() string
	GetSourceFileLocation(string, string, string) (string, error)
	GetResolver(string) (dependency.Resolver, error)
	ShouldResolve() bool
}

type BuildToolOptions struct {
	Config             provider.InitConfig
	MvnSettingsFile    string
	MvnInsecure        bool
	MvnIndexPath       string
	DisableMavenSearch bool
	Labeler            labels.Labeler
	CleanBin           bool
	GradleTaskFile     string
}

func GetBuildTool(opts BuildToolOptions, log logr.Logger) BuildTool {
	extension := strings.ToLower(path.Ext(opts.Config.Location))
	isBinary := false
	if extension == dependency.JavaArchive || extension == dependency.EnterpriseArchive || extension == dependency.WebArchive {
		isBinary = true
	}

	if bt := findGradleBuild(opts, log); bt != nil {
		log.Info("getting gradle build tool")
		return bt
	} else if isBinary {
		log.Info("getting maven binary build tool")
		return getMavenBinaryBuildTool(opts, log)
	} else if bt := findPom(opts, log); bt != nil {
		log.Info("getting maven build tool")
		return bt
	}
	return nil
}

func getHash(path string) (string, error) {
	hash := sha256.New()
	var file *os.File
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("unable to open the pom file %s - %w", path, err)
	}
	if _, err = io.Copy(hash, file); err != nil {
		file.Close()
		return "", fmt.Errorf("unable to copy file to hash %s - %w", path, err)
	}
	file.Close()
	return string(hash.Sum(nil)), nil
}

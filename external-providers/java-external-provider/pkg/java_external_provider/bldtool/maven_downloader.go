package bldtool

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency"
)

func GetDownloader(location, settingsFile string, insecure bool, log logr.Logger) (Downloader, bool) {
	if strings.HasPrefix(location, dependency.MvnURIPrefix) {
		return &mavenDownloader{location: location, settingsFile: settingsFile, insecure: insecure, log: log}, true
	}
	return nil, false
}

type mavenDownloader struct {
	location     string
	settingsFile string
	insecure     bool
	log          logr.Logger
}

func (m *mavenDownloader) Download(ctx context.Context) (string, error) {
	mvnUri := strings.Replace(m.location, dependency.MvnURIPrefix, "", 1)
	// URI format is <group>:<artifact>:<version>:<classifier>@<path>
	// <path> is optional & points to a local path where it will be downloaded
	mvnCoordinates, destPath, _ := strings.Cut(mvnUri, "@")
	mvnCoordinatesParts := strings.Split(mvnCoordinates, ":")
	if mvnCoordinates == "" || len(mvnCoordinatesParts) < 3 {
		return "", fmt.Errorf("invalid maven coordinates in location %s, must be in format mvn://<group>:<artifact>:<version>:<classifier>@<path>", m.location)
	}
	outputDir := "."
	if destPath != "" {
		if stat, err := os.Stat(destPath); err != nil || !stat.IsDir() {
			return "", fmt.Errorf("output path does not exist or not a directory")
		}
		outputDir = destPath
	}
	mvnOptions := []string{
		"dependency:copy",
		fmt.Sprintf("-Dartifact=%s", mvnCoordinates),
		fmt.Sprintf("-DoutputDirectory=%s", outputDir),
	}
	if m.settingsFile != "" {
		mvnOptions = append(mvnOptions, "-s", m.settingsFile)
	}
	if m.insecure {
		mvnOptions = append(mvnOptions, "-Dmaven.wagon.http.ssl.insecure=true")
	}
	m.log.Info("downloading maven artifact", "artifact", mvnCoordinates, "options", mvnOptions)
	cmd := exec.CommandContext(ctx, "mvn", mvnOptions...)
	cmd.Dir = outputDir
	mvnOutput, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("error downloading java artifact %s - maven output: %s - with error %w", mvnUri, string(mvnOutput), err)
	}
	downloadedPath := filepath.Join(outputDir,
		fmt.Sprintf("%s.jar", strings.Join(mvnCoordinatesParts[1:3], "-")))
	if len(mvnCoordinatesParts) == 4 {
		downloadedPath = filepath.Join(outputDir,
			fmt.Sprintf("%s.%s", strings.Join(mvnCoordinatesParts[1:3], "-"), strings.ToLower(mvnCoordinatesParts[3])))
	}
	outputLinePattern := regexp.MustCompile(`.*?Copying.*?to (.*)`)
	for _, line := range strings.Split(string(mvnOutput), "\n") {
		if outputLinePattern.MatchString(line) {
			match := outputLinePattern.FindStringSubmatch(line)
			if match != nil {
				downloadedPath = match[1]
			}
		}
	}
	if _, err := os.Stat(downloadedPath); err != nil {
		return "", fmt.Errorf("failed to download maven artifact to path %s - %w", downloadedPath, err)
	}
	return downloadedPath, nil
}

package java

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
)

// decompileJava is a function that extracts the contents ofa Java archive (.jar|.war|.ear) and
// decompiles any .class files found using the fernflower decompiler and returns the path to
// the destition directory on success, error otherwise.
func decompileJava(ctx context.Context, log logr.Logger, location string) (string, error) {
	// Get the permissions of the Java archive file
	fileInfo, err := os.Stat(location)
	if err != nil {
		return "", err
	}

	// Create the destDir directory using the same permissions as the Java archive file
	// java.jar should become java-jar-decompiled
	destDir := filepath.Join(path.Dir(location), strings.Replace(path.Base(location), ".", "-", -1) + "-decompiled")
	err = os.MkdirAll(destDir, fileInfo.Mode())
	if err != nil {
		return "", err
	}

	archive, err := zip.OpenReader(location)
	if err != nil {
		return "", err
	}
	defer archive.Close()

	for _, f := range archive.File {
		// Stop processing if our context is cancelled
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		filePath := filepath.Join(destDir, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(filePath, f.Mode())
			continue
		}

		if err = os.MkdirAll(filepath.Dir(filePath), f.Mode()); err != nil {
			return "", err
		}

		dstFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return "", err
		}
		defer dstFile.Close()

		archiveFile, err := f.Open()
		if err != nil {
			return "", err
		}
		defer archiveFile.Close()

		if _, err := io.Copy(dstFile, archiveFile); err != nil {
			return "", err
		}

		// If we found a class file, decompile it
		if strings.HasSuffix(f.Name, ".class") {
			cmd := exec.CommandContext(ctx, "java", "-jar", "/bin/fernflower.jar", filePath, path.Dir(filePath))
			err := cmd.Run()
			if err != nil {
				log.Error(err, "Failed to decompile file", filePath)
			} else {
				log.Info("Decompiled file", filePath)
			}
		}
	}

	return destDir, nil
}

package provider

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func FilterFilePattern(regex string, filepath string) (bool, error) {
	// no pattern given, all files should match
	if regex == "" {
		return true, nil
	}

	if filepath == "" {
		return false, nil
	}

	filebool, err := regexp.Compile(regex)
	if err != nil {
		return false, err
	}

	return filebool.Match([]byte(filepath)), nil

}

func FindFilesMatchingPattern(root, pattern string) ([]string, error) {
	var regex *regexp.Regexp
	// if the regex doesn't compile, we'll default to using filepath.Match on the pattern directly
	regex, _ = regexp.Compile(pattern)
	matches := []string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		var matched bool
		if regex != nil {
			matched = regex.MatchString(d.Name())
		} else {
			// TODO(fabianvf): is a fileglob style pattern sufficient or do we need regexes?
			matched, err = filepath.Match(pattern, d.Name())
			if err != nil {
				return err
			}
		}
		if matched {
			matches = append(matches, path)
		}
		return nil
	})
	return matches, err
}

func GetFiles(configLocation string, filepaths []string, patterns ...string) ([]string, error) {
	var xmlFiles []string
	if len(filepaths) == 0 {
		for _, pattern := range patterns {
			files, err := FindFilesMatchingPattern(configLocation, pattern)
			if err != nil {
				xmlFiles = append(xmlFiles, pattern)
			} else {
				xmlFiles = append(xmlFiles, files...)
			}
		}
	} else if len(filepaths) == 1 {
		// Currently, rendering will render a list as a space separated paths as a single string.
		patterns := strings.Split(filepaths[0], " ")
		for _, pattern := range patterns {
			if p, err := filepath.Rel(configLocation, pattern); err == nil {
				pattern = p
			}
			files, err := FindFilesMatchingPattern(configLocation, pattern)
			if err != nil {
				xmlFiles = append(xmlFiles, pattern)
			} else {
				xmlFiles = append(xmlFiles, files...)
			}
		}
	} else {
		for _, pattern := range filepaths {
			files, err := FindFilesMatchingPattern(configLocation, pattern)
			if err != nil {
				continue
			} else {
				xmlFiles = append(xmlFiles, files...)
			}
		}
	}
	return xmlFiles, nil
}

// MultilineGrep searches for a multi-line pattern in a file and returns line number when matched
// window determines how many lines to load in mem at a time, uses ctx to abort search on timeout
// fails when a line in file overflows 64K, returns -1 and error on failure
func MultilineGrep(ctx context.Context, window int, path, pattern string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return -1, fmt.Errorf("failed to open file - %w", err)
	}
	defer file.Close()

	patternRegex, err := regexp.Compile(`(?s)` + pattern)
	if err != nil {
		return -1, fmt.Errorf("bad pattern - %w", err)
	}

	// make sure we never keep too big a chunk in memory
	window = int(math.Min(float64(window), 5))
	scanner := bufio.NewScanner(file)
	currLine := 1
	lines := make([]string, window)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return -1, fmt.Errorf("aborting search in file %s, timed out", path)
		default:
		}

		if len(lines) == window {
			lines = lines[1:]
		}
		line := scanner.Text()
		line = strings.ReplaceAll(line, "\t", "")
		line = strings.Trim(line, " ")
		lines = append(lines, line)
		content := strings.Join(lines, "\n")
		if patternRegex.MatchString(content) {
			return int(math.Max(1, float64(currLine)-float64(window)+1)), nil
		}
		currLine += 1
	}

	return -1, scanner.Err()
}

// GetIncludedPathsFromConfig returns validated includedPaths from provider settings
// if allowFilePaths is not set, path to a file is converted into a path to its base dir
func GetIncludedPathsFromConfig(i InitConfig, allowFilePaths bool) []string {
	validatedPaths := []string{}
	if includedPaths, ok := i.ProviderSpecificConfig[IncludedPathsConfigKey].([]interface{}); ok {
		for _, ipathRaw := range includedPaths {
			if ipath, ok := ipathRaw.(string); ok {
				absPath := ipath
				if !filepath.IsAbs(ipath) {
					if ab, err := filepath.Abs(
						filepath.Join(i.Location, ipath)); err == nil {
						absPath = ab
					}
				}
				if stat, err := os.Stat(absPath); err == nil {
					if allowFilePaths || stat.IsDir() {
						validatedPaths = append(validatedPaths, ipath)
					} else {
						validatedPaths = append(validatedPaths, filepath.Dir(ipath))
					}
				}
			}
		}
	}
	return validatedPaths
}

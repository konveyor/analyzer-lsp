package provider

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SearchCriteria defines a specific search criteria
// during search time, used for condition specific search
// this takes the highest priority when searching
type SearchCriteria struct {
	Patterns           []string
	ConditionFilepaths []string
}

// FileSearcher defines a baseline search & include/excludes from either rules scope or global
type FileSearcher struct {
	BasePath               string
	AdditionalPaths        []string
	IncludePathsOrPatterns []string
	ExcludePathsOrPatterns []string
}

func (f *FileSearcher) Search(s SearchCriteria) ([]string, error) {
	statFunc := newCachedOsStat()
	walkDirFunc := newCachedWalkDir()

	// Patterns from search criteria take the highest priority
	// they contain patterns from cond.ctx.Filepaths
	searchCriteriaPaths := []string{}
	if len(s.ConditionFilepaths) == 1 {
		// these are rendered filepaths and are usually relative
		for _, fp := range strings.Split(s.ConditionFilepaths[0], " ") {
			absFp := fp
			if !filepath.IsAbs(fp) {
				absFp = filepath.Join(f.BasePath, fp)
			}
			searchCriteriaPaths = append(searchCriteriaPaths, absFp)
		}
	} else {
		searchCriteriaPaths = append(searchCriteriaPaths, s.ConditionFilepaths...)
	}

	// Baseline included file paths take the next priority
	baseIncludedDirs,
		baseIncludedFiles,
		baseIncludedPatterns := splitPathsAndPatterns(statFunc, f.IncludePathsOrPatterns...)
	// If there were included dirs, find files from them
	walkErrors := []error{}
	filesFromIncludedDirs := []string{}
	for _, dir := range baseIncludedDirs {
		files, walkError := walkDirFunc(dir)
		if walkError != nil {
			walkErrors = append(walkErrors, walkError)
		}
		filesFromIncludedDirs = append(filesFromIncludedDirs, files...)
	}
	baseIncludedFiles = append(baseIncludedFiles, filesFromIncludedDirs...)
	baseIncludedFiles = dedupSlice(baseIncludedFiles...)

	// intersect search criteria paths with baseline, search criteria taking priority
	intersectedFiles := []string{}
	if len(searchCriteriaPaths) > 0 {
		for _, bfPath := range baseIncludedFiles {
			for _, scPath := range searchCriteriaPaths {
				if bfPath == scPath || filepath.Base(bfPath) == scPath {
					intersectedFiles = append(intersectedFiles, scPath)
				}
			}
		}
	}

	allFiles := []string{}
	if len(intersectedFiles) > 0 {
		// if there are any intersected files, that's
		// the most specific set we have found so far
		allFiles = dedupSlice(intersectedFiles...)
	} else if len(baseIncludedFiles) > 0 {
		// if there are baseline included files (global includes)
		// this is the next set of files we want to scope on
		allFiles = baseIncludedFiles
	} else {
		// if there are no included files so far we have
		// to search for all files in base path
		for _, path := range append(f.AdditionalPaths, f.BasePath) {
			files, err := walkDirFunc(path)
			if err != nil {
				walkErrors = append(walkErrors, err)
			}
			allFiles = append(allFiles, files...)
		}
	}

	// apply baseline include patterns
	allFiles = f.filterFilesByPathsOrPatterns(statFunc, baseIncludedPatterns, allFiles, false)
	// apply baseline exclude patterns
	allFiles = f.filterFilesByPathsOrPatterns(statFunc, f.ExcludePathsOrPatterns, allFiles, true)
	// finally apply any search patterns
	allFiles = f.filterFilesByPathsOrPatterns(statFunc, s.Patterns, allFiles, false)

	return allFiles, errors.Join(walkErrors...)
}

func (f *FileSearcher) filterFilesByPathsOrPatterns(statFunc cachedOsStat, patterns []string, files []string, filterOut bool) []string {
	if len(patterns) == 0 {
		return files
	}
	filtered := []string{}
	for _, file := range files {
		patternMatched := false
		for _, pattern := range patterns {
			// try matching these as file paths first
			absPath := pattern
			if !filepath.IsAbs(pattern) {
				absPath = filepath.Join(f.BasePath, pattern)
			}
			if stat, statErr := statFunc(absPath); statErr == nil {
				if stat.IsDir() && strings.HasPrefix(file, absPath) {
					patternMatched = true
				} else if !stat.IsDir() && (pattern == file || filepath.Base(file) == pattern) {
					patternMatched = true
				}
			} else {
				// try matching as go regex or glob pattern
				regex, regexErr := regexp.Compile(pattern)
				if regexErr == nil && (regex.MatchString(file) || regex.MatchString(filepath.Base(file))) {
					patternMatched = true
				} else if regexErr != nil {
					m, err := filepath.Match(pattern, file)
					if err == nil {
						patternMatched = m
					}
					m, err = filepath.Match(pattern, filepath.Base(file))
					if err == nil {
						patternMatched = m
					}
				}
			}
			// if this is filtering-in we can break early
			if patternMatched && !filterOut {
				break
			}
		}
		if filterOut && !patternMatched {
			filtered = append(filtered, file)
		}
		if !filterOut && patternMatched {
			filtered = append(filtered, file)
		}
	}
	return filtered
}

func dedupSlice(s ...string) []string {
	deduped := []string{}
	mem := map[string]any{}
	for _, i := range s {
		if _, ok := mem[i]; !ok {
			deduped = append(deduped, i)
			mem[i] = true
		}
	}
	return deduped
}

func splitPathsAndPatterns(statFunc cachedOsStat, pathsOrPatterns ...string) (dirs []string, files []string, patterns []string) {
	for _, pathOrPattern := range pathsOrPatterns {
		if stat, err := statFunc(pathOrPattern); err == nil {
			if stat.IsDir() {
				dirs = append(dirs, pathOrPattern)
			} else {
				files = append(files, pathOrPattern)
			}
		} else {
			patterns = append(patterns, pathOrPattern)
		}
	}
	return
}

type cachedWalkDir func(path string) ([]string, error)

func newCachedWalkDir() cachedWalkDir {
	cache := make(map[string]struct {
		files []string
		err   error
	})
	return func(basePath string) ([]string, error) {
		val, ok := cache[basePath]
		if !ok {
			files := []string{}
			err := filepath.WalkDir(basePath, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() {
					files = append(files, path)
				}
				return nil
			})
			cache[basePath] = struct {
				files []string
				err   error
			}{
				files: files,
				err:   err,
			}
			return files, err
		}
		return val.files, val.err
	}
}

type cachedOsStat func(path string) (os.FileInfo, error)

func newCachedOsStat() cachedOsStat {
	cache := make(map[string]struct {
		info os.FileInfo
		err  error
	})
	return func(path string) (os.FileInfo, error) {
		val, ok := cache[path]
		if !ok {
			stat, err := os.Stat(path)
			cache[path] = struct {
				info os.FileInfo
				err  error
			}{
				info: stat,
				err:  err,
			}
			return stat, err
		}
		return val.info, val.err
	}
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

func GetExcludedDirsFromConfig(i InitConfig) []string {
	validatedPaths := []string{}
	if excludedDirs, ok := i.ProviderSpecificConfig[ExcludedDirsConfigKey].([]interface{}); ok {
		for _, dir := range excludedDirs {
			if expath, ok := dir.(string); ok {
				ab := expath
				var err error
				if !filepath.IsAbs(expath) {
					if ab, err = filepath.Abs(expath); err == nil {
					}
				}
				validatedPaths = append(validatedPaths, ab)
			}
		}
	}
	return validatedPaths
}

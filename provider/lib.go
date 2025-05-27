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

	"github.com/go-logr/logr"
)

// FileSearcher takes global include / exclude patterns and base locations for search
type FileSearcher struct {
	BasePath string
	// additional search paths can be added e.g. working copy paths
	AdditionalPaths           []string
	ProviderConfigConstraints IncludeExcludeConstraints
	RuleScopeConstraints      IncludeExcludeConstraints
	// fail on first file operation error
	FailFast bool
	Log      logr.Logger
}

// SearchCriteria defines a specific search criteria
// during search time, used for condition specific search
// this takes the highest priority when searching
type SearchCriteria struct {
	Patterns           []string
	ConditionFilepaths []string
}

type IncludeExcludeConstraints struct {
	IncludePathsOrPatterns []string
	ExcludePathsOrPatterns []string
}

// Search searches files using SearchCriteria defining search constraints
// filters files by inclusion rules, applies exclusion after inclusion
// constraints take priority in order of -
// 1. Search time constraints (highest priority)
// 2. Rule scope constraints
// 3. Provider config level constraints
func (f *FileSearcher) Search(s SearchCriteria) ([]string, error) {
	statFunc := newCachedOsStat()
	walkDirFunc := newCachedWalkDir()
	walkErrors := []error{}

	f.Log.V(5).Info("searching for files", "criteria", s, "additionalPaths", f.AdditionalPaths,
		"ruleScopedConstraints", f.RuleScopeConstraints, "providerScopeConstraints", f.ProviderConfigConstraints)

	// Patterns from search criteria take the highest priority
	// they contain patterns from cond.ctx.Filepaths
	searchCriteriaPaths := []string{}
	for _, pathFromSearchCriteria := range s.ConditionFilepaths {
		// rendered paths are delimited by spaces
		searchCriteriaPaths = append(searchCriteriaPaths, strings.Split(pathFromSearchCriteria, " ")...)
	}
	_, searchCriteriaFiles, searchCriteriaPatterns := splitPathsAndPatterns(statFunc, searchCriteriaPaths...)
	if len(searchCriteriaPatterns) > 0 {
		allFiles := []string{}
		for _, path := range append(f.AdditionalPaths, f.BasePath) {
			files, walkError := walkDirFunc(path)
			if walkError != nil {
				if f.FailFast {
					return nil, fmt.Errorf("failed to walk dirs - %w", walkError)
				}
				walkErrors = append(walkErrors, walkError)
			}
			allFiles = append(allFiles, f.filterFilesByPathsOrPatterns(statFunc, searchCriteriaPatterns, files, false)...)
		}
		searchCriteriaFiles = append(searchCriteriaFiles, allFiles...)
	}
	f.Log.V(7).Info("found files from search criteria", "files", searchCriteriaFiles)

	// Constraints from provider and rule level take the next priority
	includedDirs, includedFiles, includedPatterns := splitPathsAndPatterns(statFunc,
		f.ProviderConfigConstraints.IncludePathsOrPatterns...)
	ruleLevelIncludedDirs,
		ruleLevelIncludedFiles,
		ruleLevelIncludedPatterns := splitPathsAndPatterns(statFunc,
		f.RuleScopeConstraints.IncludePathsOrPatterns...)
	// any rule level constraints override provider level constraints
	if len(ruleLevelIncludedDirs)+len(ruleLevelIncludedFiles)+len(ruleLevelIncludedPatterns) > 0 {
		includedDirs = ruleLevelIncludedDirs
		includedFiles = ruleLevelIncludedFiles
		includedPatterns = ruleLevelIncludedPatterns
	}

	// If there were included dirs, find files from them
	filesFromIncludedDirs := []string{}
	for _, dir := range includedDirs {
		files, walkError := walkDirFunc(dir)
		if walkError != nil {
			if f.FailFast {
				return nil, fmt.Errorf("failed to walk all dirs - %w", walkError)
			}
			walkErrors = append(walkErrors, walkError)
		}
		filesFromIncludedDirs = append(filesFromIncludedDirs, files...)
	}
	includedFiles = append(includedFiles, filesFromIncludedDirs...)
	includedFiles = dedupSlice(includedFiles...)
	f.Log.V(7).Info("found files from include scopes", "files", includedFiles)

	// intersect search criteria paths with paths we get from other constraints
	intersectedFiles := []string{}
	if len(searchCriteriaFiles) > 0 {
		if len(includedFiles) > 0 {
			for _, bfPath := range includedFiles {
				for _, scPath := range searchCriteriaFiles {
					if bfPath == scPath || filepath.Base(bfPath) == scPath {
						intersectedFiles = append(intersectedFiles, scPath)
					}
				}
			}
		} else {
			// if there are no inclusion rules, we
			// scope on everything in condition paths
			intersectedFiles = searchCriteriaFiles
		}
	}
	f.Log.V(9).Info("intersected files", "files", intersectedFiles)

	finalSearchResult := []string{}
	// if there are any additional paths to search
	// we need to include them e.g. working copies
	for _, path := range f.AdditionalPaths {
		files, walkError := walkDirFunc(path)
		if walkError != nil {
			if f.FailFast {
				return nil, fmt.Errorf("failed to walk all dirs - %w", walkError)
			}
			walkErrors = append(walkErrors, walkError)
		}
		f.Log.V(7).Info("found files at additional path", "path", path, "files", files)
		finalSearchResult = append(finalSearchResult, files...)
	}
	if len(intersectedFiles) > 0 {
		// if there are any intersected files, that's
		// the most specific set we have found so far
		finalSearchResult = append(finalSearchResult, dedupSlice(intersectedFiles...)...)
	} else if len(includedFiles) > 0 {
		// if there are baseline included files (rule or provider)
		// this is the next set of files we want to scope on
		finalSearchResult = append(finalSearchResult, includedFiles...)
		if len(includedPatterns) > 0 {
			files, walkError := walkDirFunc(f.BasePath)
			if walkError != nil {
				if f.FailFast {
					return nil, fmt.Errorf("failed to walk all dirs - %w", walkError)
				}
				walkErrors = append(walkErrors, walkError)
			}
			finalSearchResult = append(finalSearchResult,
				f.filterFilesByPathsOrPatterns(statFunc, includedPatterns, files, false)...)
		}
	} else {
		// if there are no included files so far we have
		// to search for all files in base path
		files, walkError := walkDirFunc(f.BasePath)
		if walkError != nil {
			if f.FailFast {
				return nil, fmt.Errorf("failed to walk all dirs - %w", walkError)
			}
			walkErrors = append(walkErrors, walkError)
		}
		finalSearchResult = append(finalSearchResult, files...)
	}

	// apply baseline include patterns and any search patterns
	finalSearchResult = f.filterFilesByPathsOrPatterns(statFunc, includedPatterns, finalSearchResult, false)
	// apply patterns from search criteria
	finalSearchResult = f.filterFilesByPathsOrPatterns(statFunc, s.Patterns, finalSearchResult, false)
	finalSearchResult = f.filterFilesByPathsOrPatterns(statFunc, searchCriteriaPaths, finalSearchResult, false)

	// finally, apply exclusion, rule scope takes priority over provider config
	if len(f.RuleScopeConstraints.ExcludePathsOrPatterns) > 0 {
		finalSearchResult = f.filterFilesByPathsOrPatterns(
			statFunc, f.RuleScopeConstraints.ExcludePathsOrPatterns, finalSearchResult, true)
	} else {
		finalSearchResult = f.filterFilesByPathsOrPatterns(
			statFunc, f.ProviderConfigConstraints.ExcludePathsOrPatterns, finalSearchResult, true)
	}

	f.Log.V(5).Info("returning file search result", "files", finalSearchResult)
	return finalSearchResult, errors.Join(walkErrors...)
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
				} else if !stat.IsDir() {
					if absPath == file {
						patternMatched = true
					}
					if filepath.Base(absPath) == pattern && pattern == filepath.Base(file) {
						patternMatched = true
					}
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

package provider

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
)

func FilterFilePattern(regex string, filepath string) (bool, error) {
	if regex == "" || filepath == "" {
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

func Getfiles(configLocation string, filepaths []string) ([]string, error) {
	var xmlFiles []string

	if len(filepaths) == 1 {
		// Currently, rendering will render a list as a space separated paths as a single string.
		patterns := strings.Split(filepaths[0], " ")
		for _, pattern := range patterns {
			files, err := FindFilesMatchingPattern(configLocation, pattern)
			if err != nil {
				// Something went wrong dealing with the pattern, so we'll assume the user input
				// is good and pass it on
				// TODO: If we're ever hitting this for real, we should investigate
				fmt.Printf("Unable to resolve pattern '%s': %v", pattern, err)
				xmlFiles = append(xmlFiles, pattern)
			} else {
				xmlFiles = append(xmlFiles, files...)
			}
		}
	} else {
		for _, pattern := range filepaths {
			files, err := FindFilesMatchingPattern(configLocation, pattern)
			if err != nil {
				xmlFiles = append(xmlFiles, pattern)
			} else {
				xmlFiles = append(xmlFiles, files...)
			}
		}
	}

	return xmlFiles, nil
}

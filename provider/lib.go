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
	matches := []string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		matched, err := FilterFilePattern(pattern, d.Name())
		if err != nil {
			return err
		}
		if matched {
			matches = append(matches, path)
		}
		return nil
	})
	return matches, err
}

func GetFiles(filePaths []string, rootpath string) ([]string, error) {
	var fileslist []string
	var err error
	if len(filePaths) == 0 {
		pattern := "*.xml"
		fileslist, err = FindFilesMatchingPattern(rootpath, pattern)
		if err != nil {
			fmt.Errorf("Unable to find files using pattern `%s`: %v", pattern, err)
		}
		xhtmlFiles, err := FindFilesMatchingPattern(rootpath, "*.xhtml")
		if err != nil {
			fmt.Errorf("Unable to find files using pattern `%s`: %v", "*.xhtml", err)
		}
		fileslist = append(fileslist, xhtmlFiles...)
	} else if len(filePaths) == 1 {
		// Currently, rendering will render a list as a space seperated paths as a single string.
		patterns := strings.Split(filePaths[0], " ")
		for _, pattern := range patterns {
			files, err := FindFilesMatchingPattern(rootpath, pattern)
			if err != nil {
				// Something went wrong dealing with the pattern, so we'll assume the user input
				// is good and pass it on
				// TODO(fabianvf): if we're ever hitting this for real we should investigate
				fmt.Printf("Unable to resolve pattern '%s': %v", pattern, err)
				fileslist = append(fileslist, pattern)
			} else {
				fileslist = append(fileslist, files...)
			}
		}
	} else {
		for _, pattern := range filePaths {
			files, err := FindFilesMatchingPattern(rootpath, pattern)
			if err != nil {
				fileslist = append(fileslist, pattern)
			} else {
				fileslist = append(fileslist, files...)
			}
		}
	}
	var absolutePaths []string
	for _, file := range fileslist {
		absPath, err := filepath.Abs(file)
		if err != nil {
			fmt.Printf("unable to get absolute path for '%s': %v\n", file, err)
			return absolutePaths, err
		} else {
			absolutePaths = append(absolutePaths, absPath)
		}

	}
	return absolutePaths, err
}

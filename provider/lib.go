package provider

import (
	"regexp"
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

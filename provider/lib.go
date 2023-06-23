package provider

import (
	"regexp"
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

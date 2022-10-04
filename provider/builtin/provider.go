package builtin

import (
	"context"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/konveyor/analyzer-lsp/jsonrpc2"
	"github.com/konveyor/analyzer-lsp/provider/lib"
)

var capabilities = []string{
	"filecontent",
	"file",
	"xml",
}

type builtinProvider struct {
	rpc *jsonrpc2.Conn
	ctx context.Context

	config lib.Config
}

func NewBuiltinProvider(config lib.Config) *builtinProvider {
	return &builtinProvider{
		config: config,
	}
}

func (p *builtinProvider) Capabilities() ([]string, error) {
	return capabilities, nil
}

func (p *builtinProvider) Evaluate(cap string, conditionInfo interface{}) (lib.ProviderEvaluateResponse, error) {
	response := lib.ProviderEvaluateResponse{}
	switch cap {
	case "file":
		pattern, ok := conditionInfo.(string)
		if !ok {
			return response, fmt.Errorf("Could not parse provided file pattern as string: %v", conditionInfo)
		}
		matchingFiles, err := findFilesMatchingPattern(p.config.Location, pattern)
		if err != nil {
			return response, fmt.Errorf("Unable to find files using pattern `%s`: %v", pattern, err)
		}

		response.Passed = len(matchingFiles) == 0

		for _, match := range matchingFiles {
			response.ConditionHitContext = append(response.ConditionHitContext, map[string]string{
				"filepath": match,
			})
		}
		return response, nil
	case "filecontent":
		pattern, ok := conditionInfo.(string)
		if !ok {
			return response, fmt.Errorf("Could not parse provided regex pattern as string: %v", conditionInfo)
		}
		var outputBytes []byte
		grep := exec.Command("grep", "-o", "-n", "-R", "-E", pattern, p.config.Location)
		outputBytes, err := grep.Output()
		if err != nil {
			return response, fmt.Errorf("Could not run grep with provided pattern %+v", err)
		}
		matches := strings.Split(strings.TrimSpace(string(outputBytes)), "\n")
		response.Passed = (len(matches) == 0)

		for _, match := range matches {
			//TODO(fabianvf): This will not work if there is a `:` in the filename, do we care?
			pieces := strings.SplitN(match, ":", 3)
			if len(pieces) != 3 {
				//TODO(fabianvf): Just log or return?
				return response, fmt.Errorf("Malformed response from grep, cannot parse %s with pattern {filepath}:{lineNumber}:{matchingText}", match)
			}
			response.ConditionHitContext = append(response.ConditionHitContext, map[string]string{
				"filepath":     pieces[0],
				"lineNumber":   pieces[1],
				"matchingText": pieces[2],
			})
		}
		return response, nil
	case "xml":
		return lib.ProviderEvaluateResponse{}, fmt.Errorf("%s not yet implemented", cap)
	default:
		return lib.ProviderEvaluateResponse{}, fmt.Errorf("Capability must be one of %v, not %s", capabilities, cap)
	}
}

func findFilesMatchingPattern(root, pattern string) ([]string, error) {
	matches := []string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// TODO(fabianvf): is a fileglob style pattern sufficient or do we need regexes?
		matched, err := filepath.Match(pattern, d.Name())
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

// We don't need to init anything
func (p *builtinProvider) Init(_ context.Context) error {
	return nil
}

package builtin

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"

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

		if len(matchingFiles) == 0 {
			response.Passed = true
		} else {
			response.Passed = false
			hitContext := map[string]string{}
			for _, match := range matchingFiles {
				// TODO(fabianvf) how should this be stored?
				hitContext[match] = ""
			}
			response.ConditionHitContext = append(response.ConditionHitContext, hitContext)
		}
		return response, nil

	case "filecontent":
		return lib.ProviderEvaluateResponse{}, fmt.Errorf("%s not yet implemented", cap)
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
			matches = append(matches, d.Name())
		}
		return nil
	})
	return matches, err
}

// We don't need to init anything
func (p *builtinProvider) Init(_ context.Context) error {
	return nil
}

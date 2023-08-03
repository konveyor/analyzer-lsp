package generic

import (
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
)

func (g *genericServiceClient) GetDependencies() (map[uri.URI][]*provider.Dep, error) {
	cmdStr, isString := g.config.ProviderSpecificConfig["dependencyProviderPath"].(string)
	if !isString {
		return nil, fmt.Errorf("dependency provider path is not a string")
	}
	// Expects dependency provider to output provider.Dep structs to stdout
	cmd := exec.Command(cmdStr)
	cmd.Dir = g.config.Location
	dataR, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	data := string(dataR)
	if len(data) == 0 {
		return nil, nil
	}
	m := map[uri.URI][]*provider.Dep{}
	err = json.Unmarshal([]byte(data), &m)
	if err != nil {
		return nil, err
	}
	return m, err
}

func (p *genericServiceClient) GetDependenciesDAG() (map[uri.URI][]provider.DepDAGItem, error) {
	return nil, nil
}

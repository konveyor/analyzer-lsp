package java

import (
	"context"
	"fmt"

	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
)

func (p *javaServiceClient) GetDependencies(ctx context.Context) (map[uri.URI][]*provider.Dep, error) {
	p.log.V(4).Info("running dependency analysis")

	m := map[uri.URI][]*provider.Dep{}
	ll, err := p.GetDependenciesDAG(ctx)
	if err != nil {
		return nil, err
	}
	for f, ds := range ll {
		deps := []*provider.Dep{}
		for _, dep := range ds {
			d := dep.Dep
			deps = append(deps, &d)
			deps = append(deps, provider.ConvertDagItemsToList(dep.AddedDeps)...)
		}
		m[f] = deps
	}
	return m, nil
}

func (p *javaServiceClient) GetDependenciesDAG(ctx context.Context) (map[uri.URI][]provider.DepDAGItem, error) {
	p.log.V(4).Info("running dependency analysis for DAG")
	var ll map[uri.URI][]konveyor.DepDAGItem

	p.log.Info("using bldtooL", "tool", fmt.Sprintf("%#v", p.buildTool))
	ll, err := p.buildTool.GetDependencies(ctx)
	if err != nil {
		// cache error
		return nil, err
	}
	if useCache {
		ll = p.depsCache
		return ll, nil
	} else {
		ll, err = p.buildTool.GetDependencies(ctx)
		if err != nil {
			// cache error
			return nil, err
		}
	}
	return ll, nil
}

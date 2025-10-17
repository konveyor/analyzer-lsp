package java

import (
	"context"

	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
)

func (p *javaServiceClient) GetDependencies(ctx context.Context) (map[uri.URI][]*provider.Dep, error) {
	p.log.V(4).Info("running dependency analysis")

	var m map[uri.URI][]*provider.Dep
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
	var ll map[uri.URI][]konveyor.DepDAGItem

	p.depsMutex.Lock()
	defer p.depsMutex.Unlock()

	useCache, err := p.buildTool.UseCache()
	if err != nil {
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
	p.depsCache = ll
	return ll, nil
}

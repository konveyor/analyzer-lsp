package engine

import (
	"fmt"

	"github.com/go-logr/logr"
)

const TemplateContextPathScopeKey = "konveyor.io/path-scope"

// Scopes apply to individual calls to the providers and will add inforamtion to the ConditionContext
// To apply the scope. It is the responsiblity of the provider to use these correctly.
type Scope interface {
	Name() string
	// For now this is the only place that we are considering adding a scope
	// in the future, we could scope other things
	AddToContext(*ConditionContext) error
}

type scopeWrapper struct {
	scopes []Scope
}

func (s *scopeWrapper) Name() string {
	name := ""
	for i, s := range s.scopes {
		if i == 0 {
			name = s.Name()

		} else {
			name = fmt.Sprintf("%s -- %s", name, s.Name())
		}
	}
	return name
}

func (s *scopeWrapper) AddToContext(conditionCTX *ConditionContext) error {
	for _, s := range s.scopes {
		err := s.AddToContext(conditionCTX)
		if err != nil {
			return err
		}
	}
	return nil
}

var _ Scope = &scopeWrapper{}

func NewScope(scopes ...Scope) Scope {
	return &scopeWrapper{scopes: scopes}
}

type includedPathScope struct {
	log   logr.Logger
	paths []string
}

var _ Scope = &includedPathScope{}

func (i *includedPathScope) Name() string {
	return "IncludedPathScope"
}

// This will only update conditionCTX if filepaths is not set.
func (i *includedPathScope) AddToContext(conditionCTX *ConditionContext) error {
	// If any chain template has the filepaths set, only use those.
	for k, chainTemplate := range conditionCTX.Template {
		if chainTemplate.Filepaths != nil && len(chainTemplate.Filepaths) > 0 {
			i.log.V(5).Info("includedPathScope not used because filepath set", "filepaths", chainTemplate.Filepaths, "key", k)
			return nil
		}
	}

	// if no As clauses have filepaths, then assume we need to add the special cased filepath for scopes here
	conditionCTX.Template[TemplateContextPathScopeKey] = ChainTemplate{
		Filepaths: i.paths,
		Extras:    nil,
	}
	return nil

}

func IncludedPathsScope(paths []string, log logr.Logger) Scope {
	return &includedPathScope{
		paths: paths,
		log:   log,
	}
}

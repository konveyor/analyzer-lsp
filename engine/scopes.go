package engine

import (
	"fmt"
	"net/url"
	"regexp"

	"github.com/go-logr/logr"
	"go.lsp.dev/uri"
)

const TemplateContextPathScopeKey = "konveyor.io/path-scope"

// Scopes apply to individual calls to the providers and will add inforamtion to the ConditionContext
// To apply the scope. It is the responsiblity of the provider to use these correctly.
type Scope interface {
	Name() string
	// For now this is the only place that we are considering adding a scope
	// in the future, we could scope other things
	AddToContext(*ConditionContext) error
	FilterResponse(IncidentContext) bool
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
	for _, scope := range s.scopes {
		err := scope.AddToContext(conditionCTX)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *scopeWrapper) FilterResponse(response IncidentContext) bool {
	for _, scope := range s.scopes {
		shouldFilter := scope.FilterResponse(response)
		if shouldFilter {
			return true
		}
	}
	return false
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

func (i *includedPathScope) FilterResponse(response IncidentContext) bool {
	// when there are no included paths set, everything is included
	if len(i.paths) == 0 {
		return false
	}
	for _, path := range i.paths {
		if string(response.FileURI) != "" && response.FileURI.Filename() == path {
			return false
		}
	}
	return true
}

func IncludedPathsScope(paths []string, log logr.Logger) Scope {
	return &includedPathScope{
		paths: paths,
		log:   log,
	}
}

type excludedPathsScope struct {
	paths []string
	log logr.Logger
}

var _ Scope = &excludedPathsScope{}

func (e *excludedPathsScope) Name() string {
	return "ExcludedPathsScope"
}

func (e *excludedPathsScope) AddToContext(conditionCtx *ConditionContext) error {
	templ := ChainTemplate{}
	if existingTempl, ok := conditionCtx.Template[TemplateContextPathScopeKey]; ok {
		templ = existingTempl
	}
	templ.ExcludedPaths = e.paths
	conditionCtx.Template[TemplateContextPathScopeKey] = templ
	return nil
}

func (e *excludedPathsScope) FilterResponse(response IncidentContext) bool {
	if response.FileURI == "" {
		return false
	}
	for _, path := range e.paths {
		e.log.V(5).Info("using path for filtering response", "path", path)
		pattern, err := regexp.Compile(path)
		if err != nil {
			e.log.V(5).Error(err, "invalid pattern", "pattern", path)
			continue
		}
		u, err := url.ParseRequestURI(string(response.FileURI))
		if err == nil && u.Scheme == uri.FileScheme && pattern.MatchString(response.FileURI.Filename()) {
			e.log.V(5).Info("excluding the file", "file", response.FileURI.Filename(), "pattern", pattern)
			return true
		}
	}
	return false
}

func ExcludedPathsScope(paths []string, log logr.Logger) Scope {
	return &excludedPathsScope{
		paths: paths,
		log: log.WithName("excludedPathScope"),
	}
}

package konveyor

import (
	"context"

	"github.com/konveyor/analyzer-lsp/engine"
	v1 "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
)

// TODO: Docs
type analyzer struct {
	ruleset    []engine.RuleSet
	engine     engine.RuleEngine
	cancelFunc context.CancelFunc
	ctx        context.Context
	providers  []Provider
}

var _ Analyzer = &analyzer{}

func (a *analyzer) Run(options ...EngineOption) []v1.RuleSet {
	return nil
}

func (a *analyzer) RuleLabels() []string {
	return nil
}

func (a *analyzer) RulesetFilepaths() map[string]string {
	return nil
}

func (a *analyzer) GetProviderForLanguage(language string) (Provider, bool) {
	return Provider{}, false
}

func (a *analyzer) GetProviders() []Provider {
	return nil
}

func (a *analyzer) GetDependencies(outputFilePath string, tree bool) error {
	return nil
}

func (a *analyzer) Stop() error {
	return nil
}

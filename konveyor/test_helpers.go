package konveyor

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/progress"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
)

// Mock implementations for testing

type mockProviderClient struct {
	capabilities         []provider.Capability
	initError            error
	startError           error
	prepareError         error
	stopCalled           bool
	getDepsError         error
	getDepsDAGError      error
	deps                 map[uri.URI][]*provider.Dep
	depsDAG              map[uri.URI][]provider.DepDAGItem
	additionalBuiltins   []provider.InitConfig
	serviceClient        provider.ServiceClient
}

func (m *mockProviderClient) Capabilities() []provider.Capability {
	return m.capabilities
}

func (m *mockProviderClient) Init(ctx context.Context, log logr.Logger, config provider.InitConfig) (provider.ServiceClient, provider.InitConfig, error) {
	if m.initError != nil {
		return nil, provider.InitConfig{}, m.initError
	}
	return m.serviceClient, provider.InitConfig{}, nil
}

func (m *mockProviderClient) ProviderInit(ctx context.Context, additionalBuiltins []provider.InitConfig) ([]provider.InitConfig, error) {
	if m.initError != nil {
		return nil, m.initError
	}
	return m.additionalBuiltins, nil
}

func (m *mockProviderClient) Stop() {
	m.stopCalled = true
}

func (m *mockProviderClient) Prepare(ctx context.Context, conditions []provider.ConditionsByCap) error {
	return m.prepareError
}

func (m *mockProviderClient) GetDependencies(ctx context.Context) (map[uri.URI][]*provider.Dep, error) {
	if m.getDepsError != nil {
		return nil, m.getDepsError
	}
	return m.deps, nil
}

func (m *mockProviderClient) GetDependenciesDAG(ctx context.Context) (map[uri.URI][]provider.DepDAGItem, error) {
	if m.getDepsDAGError != nil {
		return nil, m.getDepsDAGError
	}
	return m.depsDAG, nil
}

func (m *mockProviderClient) Evaluate(ctx context.Context, cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
	return provider.ProviderEvaluateResponse{}, nil
}

func (m *mockProviderClient) GetCodeSnip(ctx context.Context, location string, lineNumber int) (string, error) {
	return "", nil
}

func (m *mockProviderClient) NotifyFileChanges(ctx context.Context, changes ...provider.FileChange) error {
	return nil
}

type mockReporter struct {
	events []progress.Event
}

func (m *mockReporter) Report(event progress.Event) {
	m.events = append(m.events, event)
}

type mockRuleSelector struct{}

func (m *mockRuleSelector) Matches(meta *engine.RuleMeta) (bool, error) {
	return true, nil
}

type mockScope struct {
	name string
}

func (m *mockScope) Name() string {
	return m.name
}

func (m *mockScope) AddToContext(ctx *engine.ConditionContext) error {
	return nil
}

func (m *mockScope) FilterResponse(ctx engine.IncidentContext) bool {
	return true
}

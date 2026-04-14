package main

import (
	"context"
	"encoding/json"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	konveyor "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lsp.dev/uri"
)

// --- Mock AnalyzerService ---

type mockAnalyzerService struct {
	analyzeFunc           func(AnalyzeParams) ([]konveyor.RuleSet, error)
	getCachedResultsFunc  func() []konveyor.RuleSet
	notifyFileChangesFunc func([]provider.FileChange) error
	listProvidersFunc     func() []ProviderInfo
	getDependenciesFunc   func() ([]konveyor.DepsFlatItem, error)
	listRulesFunc         func() []RuleInfo
	getMigrationCtxFunc   func() MigrationContext
	stopFunc              func() error
}

func (m *mockAnalyzerService) Analyze(params AnalyzeParams) ([]konveyor.RuleSet, error) {
	if m.analyzeFunc != nil {
		return m.analyzeFunc(params)
	}
	return nil, nil
}

func (m *mockAnalyzerService) GetCachedResults() []konveyor.RuleSet {
	if m.getCachedResultsFunc != nil {
		return m.getCachedResultsFunc()
	}
	return nil
}

func (m *mockAnalyzerService) NotifyFileChanges(changes []provider.FileChange) error {
	if m.notifyFileChangesFunc != nil {
		return m.notifyFileChangesFunc(changes)
	}
	return nil
}

func (m *mockAnalyzerService) ListProviders() []ProviderInfo {
	if m.listProvidersFunc != nil {
		return m.listProvidersFunc()
	}
	return nil
}

func (m *mockAnalyzerService) GetDependencies() ([]konveyor.DepsFlatItem, error) {
	if m.getDependenciesFunc != nil {
		return m.getDependenciesFunc()
	}
	return nil, nil
}

func (m *mockAnalyzerService) ListRules() []RuleInfo {
	if m.listRulesFunc != nil {
		return m.listRulesFunc()
	}
	return nil
}

func (m *mockAnalyzerService) GetMigrationContext() MigrationContext {
	if m.getMigrationCtxFunc != nil {
		return m.getMigrationCtxFunc()
	}
	return MigrationContext{}
}

func (m *mockAnalyzerService) Stop() error {
	if m.stopFunc != nil {
		return m.stopFunc()
	}
	return nil
}

// --- Tool Registration Tests ---

func TestToolRegistration(t *testing.T) {
	svc := &mockAnalyzerService{}
	server := NewMCPServer(svc)

	serverTransport, clientTransport := mcpsdk.NewInMemoryTransports()
	ctx := context.Background()

	go func() {
		_, _ = server.Connect(ctx, serverTransport, nil)
	}()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer clientSession.Close()

	result, err := clientSession.ListTools(ctx, nil)
	require.NoError(t, err)

	toolNames := map[string]bool{}
	for _, tool := range result.Tools {
		toolNames[tool.Name] = true
	}

	expectedTools := []string{
		"analyze",
		"get_analysis_results",
		"notify_file_changes",
		"validate_rules",
		"list_rules",
		"list_providers",
		"get_dependencies",
		"analyze_incidents",
		"get_migration_context",
	}

	for _, name := range expectedTools {
		assert.True(t, toolNames[name], "tool %q should be registered", name)
	}
}

// --- Analyze Tool Tests ---

func TestAnalyzeTool_HappyPath(t *testing.T) {
	effort := 5
	svc := &mockAnalyzerService{
		analyzeFunc: func(params AnalyzeParams) ([]konveyor.RuleSet, error) {
			assert.Equal(t, "konveyor.io/target=eap8", params.LabelSelector)
			return []konveyor.RuleSet{
				{
					Name: "eap-rules",
					Violations: map[string]konveyor.Violation{
						"eap8-001": {
							Description: "EJB to CDI",
							Effort:      &effort,
							Incidents: []konveyor.Incident{
								{
									URI:        uri.URI("file:///src/MyBean.java"),
									Message:    "Replace EJB with CDI",
									LineNumber: intPtr(15),
								},
							},
						},
					},
				},
			}, nil
		},
	}

	server := NewMCPServer(svc)
	result := callTool(t, server, "analyze", map[string]interface{}{
		"label_selector": "konveyor.io/target=eap8",
	})

	require.False(t, result.IsError, "analyze should not return error")
	require.Len(t, result.Content, 1)

	text := result.Content[0].(*mcpsdk.TextContent).Text
	var rulesets []konveyor.RuleSet
	err := json.Unmarshal([]byte(text), &rulesets)
	require.NoError(t, err)
	require.Len(t, rulesets, 1)
	assert.Equal(t, "eap-rules", rulesets[0].Name)
}

func TestAnalyzeTool_PassesExcludedPaths(t *testing.T) {
	svc := &mockAnalyzerService{
		analyzeFunc: func(params AnalyzeParams) ([]konveyor.RuleSet, error) {
			assert.Equal(t, []string{"/src/legacy/"}, params.ExcludedPaths)
			assert.Equal(t, []string{"/src/"}, params.IncludedPaths)
			return []konveyor.RuleSet{}, nil
		},
	}

	server := NewMCPServer(svc)
	result := callTool(t, server, "analyze", map[string]interface{}{
		"included_paths": []string{"/src/"},
		"excluded_paths": []string{"/src/legacy/"},
	})

	require.False(t, result.IsError)
}

func TestAnalyzeTool_EmptyResults(t *testing.T) {
	svc := &mockAnalyzerService{
		analyzeFunc: func(params AnalyzeParams) ([]konveyor.RuleSet, error) {
			return []konveyor.RuleSet{}, nil
		},
	}

	server := NewMCPServer(svc)
	result := callTool(t, server, "analyze", map[string]interface{}{})

	require.False(t, result.IsError)
	text := result.Content[0].(*mcpsdk.TextContent).Text
	var rulesets []konveyor.RuleSet
	err := json.Unmarshal([]byte(text), &rulesets)
	require.NoError(t, err)
	assert.Empty(t, rulesets)
}

func TestAnalyzeTool_Error(t *testing.T) {
	svc := &mockAnalyzerService{
		analyzeFunc: func(params AnalyzeParams) ([]konveyor.RuleSet, error) {
			return nil, assert.AnError
		},
	}

	server := NewMCPServer(svc)
	result := callTool(t, server, "analyze", map[string]interface{}{})

	assert.True(t, result.IsError, "should return error when analysis fails")
}

// --- GetAnalysisResults Tool Tests ---

func TestGetAnalysisResultsTool(t *testing.T) {
	svc := &mockAnalyzerService{
		getCachedResultsFunc: func() []konveyor.RuleSet {
			return []konveyor.RuleSet{
				{Name: "cached-ruleset"},
			}
		},
	}

	server := NewMCPServer(svc)
	result := callTool(t, server, "get_analysis_results", map[string]interface{}{})

	require.False(t, result.IsError)
	text := result.Content[0].(*mcpsdk.TextContent).Text
	var rulesets []konveyor.RuleSet
	err := json.Unmarshal([]byte(text), &rulesets)
	require.NoError(t, err)
	require.Len(t, rulesets, 1)
	assert.Equal(t, "cached-ruleset", rulesets[0].Name)
}

// --- NotifyFileChanges Tool Tests ---

func TestNotifyFileChangesTool(t *testing.T) {
	var receivedChanges []provider.FileChange
	svc := &mockAnalyzerService{
		notifyFileChangesFunc: func(changes []provider.FileChange) error {
			receivedChanges = changes
			return nil
		},
	}

	server := NewMCPServer(svc)
	result := callTool(t, server, "notify_file_changes", map[string]interface{}{
		"changes": []map[string]interface{}{
			{"path": "/src/main.java", "saved": true},
		},
	})

	require.False(t, result.IsError)
	require.Len(t, receivedChanges, 1)
	assert.Equal(t, "/src/main.java", receivedChanges[0].Path)
}

// --- ListProviders Tool Tests ---

func TestListProvidersTool(t *testing.T) {
	svc := &mockAnalyzerService{
		listProvidersFunc: func() []ProviderInfo {
			return []ProviderInfo{
				{Name: "java", Capabilities: []string{"referenced", "dependency"}},
				{Name: "builtin", Capabilities: []string{"filecontent", "file"}},
			}
		},
	}

	server := NewMCPServer(svc)
	result := callTool(t, server, "list_providers", map[string]interface{}{})

	require.False(t, result.IsError)
	text := result.Content[0].(*mcpsdk.TextContent).Text
	var providers []ProviderInfo
	err := json.Unmarshal([]byte(text), &providers)
	require.NoError(t, err)
	assert.Len(t, providers, 2)
}

// --- ListRules Tool Tests ---

func TestListRulesTool(t *testing.T) {
	svc := &mockAnalyzerService{
		listRulesFunc: func() []RuleInfo {
			return []RuleInfo{
				{ID: "eap8-001", Description: "EJB migration", RuleSetName: "eap"},
			}
		},
	}

	server := NewMCPServer(svc)
	result := callTool(t, server, "list_rules", map[string]interface{}{})

	require.False(t, result.IsError)
	text := result.Content[0].(*mcpsdk.TextContent).Text
	var rules []RuleInfo
	err := json.Unmarshal([]byte(text), &rules)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, "eap8-001", rules[0].ID)
}

// --- ValidateRules Tool Tests ---

func TestValidateRulesTool_ValidYAML(t *testing.T) {
	svc := &mockAnalyzerService{}

	server := NewMCPServer(svc)
	result := callTool(t, server, "validate_rules", map[string]interface{}{
		"rules_content": `- name: test-ruleset
  description: A test ruleset
  rules:
    - ruleID: test-001
      message: test message
`,
	})

	require.False(t, result.IsError)
	text := result.Content[0].(*mcpsdk.TextContent).Text
	var vr validationResult
	err := json.Unmarshal([]byte(text), &vr)
	require.NoError(t, err)
	assert.True(t, vr.Valid)
}

func TestValidateRulesTool_InvalidYAML(t *testing.T) {
	svc := &mockAnalyzerService{}

	server := NewMCPServer(svc)
	result := callTool(t, server, "validate_rules", map[string]interface{}{
		"rules_content": `{invalid yaml: [`,
	})

	require.False(t, result.IsError) // tool-level success, validation reports errors in result
	text := result.Content[0].(*mcpsdk.TextContent).Text
	var vr validationResult
	err := json.Unmarshal([]byte(text), &vr)
	require.NoError(t, err)
	assert.False(t, vr.Valid)
	assert.NotEmpty(t, vr.Errors)
}

func TestValidateRulesTool_NoInput(t *testing.T) {
	svc := &mockAnalyzerService{}

	server := NewMCPServer(svc)
	result := callTool(t, server, "validate_rules", map[string]interface{}{})

	// Should be an error since neither rules_content nor rules_path was provided
	assert.True(t, result.IsError)
}

// --- GetDependencies Tool Tests ---

func TestGetDependenciesTool(t *testing.T) {
	svc := &mockAnalyzerService{
		getDependenciesFunc: func() ([]konveyor.DepsFlatItem, error) {
			return []konveyor.DepsFlatItem{
				{
					FileURI:  "file:///pom.xml",
					Provider: "java",
					Dependencies: []*konveyor.Dep{
						{Name: "org.springframework:spring-core", Version: "5.3.0"},
					},
				},
			}, nil
		},
	}

	server := NewMCPServer(svc)
	result := callTool(t, server, "get_dependencies", map[string]interface{}{})

	require.False(t, result.IsError)
	text := result.Content[0].(*mcpsdk.TextContent).Text
	var deps []konveyor.DepsFlatItem
	err := json.Unmarshal([]byte(text), &deps)
	require.NoError(t, err)
	require.Len(t, deps, 1)
	assert.Equal(t, "java", deps[0].Provider)
}

// --- AnalyzeIncidents Tool Tests ---

func TestAnalyzeIncidentsTool_FilterByFile(t *testing.T) {
	effort := 3
	svc := &mockAnalyzerService{
		getCachedResultsFunc: func() []konveyor.RuleSet {
			return []konveyor.RuleSet{
				{
					Name: "test",
					Violations: map[string]konveyor.Violation{
						"rule-001": {
							Description: "deprecated",
							Effort:      &effort,
							Incidents: []konveyor.Incident{
								{URI: uri.URI("file:///src/a.java"), Message: "in a"},
								{URI: uri.URI("file:///src/b.java"), Message: "in b"},
							},
						},
					},
				},
			}
		},
	}

	server := NewMCPServer(svc)
	result := callTool(t, server, "analyze_incidents", map[string]interface{}{
		"file_path": "a.java",
	})

	require.False(t, result.IsError)
	text := result.Content[0].(*mcpsdk.TextContent).Text

	// Should only contain the incident from a.java
	assert.Contains(t, text, "in a")
	assert.NotContains(t, text, "in b")
}

// --- GetMigrationContext Tool Tests ---

func TestGetMigrationContextTool(t *testing.T) {
	svc := &mockAnalyzerService{
		getMigrationCtxFunc: func() MigrationContext {
			return MigrationContext{
				LabelSelector: "konveyor.io/target=eap8",
				Sources:       []string{"eap6"},
				Targets:       []string{"eap8"},
			}
		},
	}

	server := NewMCPServer(svc)
	result := callTool(t, server, "get_migration_context", map[string]interface{}{})

	require.False(t, result.IsError)
	text := result.Content[0].(*mcpsdk.TextContent).Text
	var ctx MigrationContext
	err := json.Unmarshal([]byte(text), &ctx)
	require.NoError(t, err)
	assert.Equal(t, "konveyor.io/target=eap8", ctx.LabelSelector)
	assert.Equal(t, []string{"eap8"}, ctx.Targets)
}

// --- Helper ---

// callTool is a helper that calls a tool directly on the server using the low-level API.
func callTool(t *testing.T, server *mcpsdk.Server, toolName string, args map[string]interface{}) *mcpsdk.CallToolResult {
	t.Helper()

	ctx := context.Background()

	serverTransport, clientTransport := mcpsdk.NewInMemoryTransports()

	go func() {
		_, _ = server.Connect(ctx, serverTransport, nil)
	}()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer clientSession.Close()

	result, err := clientSession.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
	require.NoError(t, err)
	return result
}

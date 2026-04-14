package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	konveyor "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lsp.dev/uri"
)

// TestIntegration_FullWorkflow tests the complete MCP workflow:
// 1. List tools
// 2. Run analysis
// 3. Get cached results
// 4. Notify file changes
// 5. Run incremental analysis
// 6. Query incidents
func TestIntegration_FullWorkflow(t *testing.T) {
	effort := 5
	callCount := 0

	svc := &mockAnalyzerService{
		analyzeFunc: func(params AnalyzeParams) ([]konveyor.RuleSet, error) {
			callCount++
			return []konveyor.RuleSet{
				{
					Name: "eap-rules",
					Violations: map[string]konveyor.Violation{
						"eap8-001": {
							Description: "EJB to CDI migration",
							Effort:      &effort,
							Incidents: []konveyor.Incident{
								{
									URI:        uri.URI("file:///src/MyBean.java"),
									Message:    "Replace @EJB with @Inject",
									LineNumber: intPtr(15),
								},
							},
						},
					},
				},
			}, nil
		},
		getCachedResultsFunc: func() []konveyor.RuleSet {
			return []konveyor.RuleSet{
				{Name: "eap-rules-cached"},
			}
		},
		notifyFileChangesFunc: func(changes []provider.FileChange) error {
			return nil
		},
		listProvidersFunc: func() []ProviderInfo {
			return []ProviderInfo{
				{Name: "java", Capabilities: []string{"referenced"}},
			}
		},
		listRulesFunc: func() []RuleInfo {
			return []RuleInfo{
				{ID: "eap8-001", RuleSetName: "eap-rules"},
			}
		},
		getMigrationCtxFunc: func() MigrationContext {
			return MigrationContext{
				LabelSelector: "konveyor.io/target=eap8",
				Targets:       []string{"eap8"},
			}
		},
	}

	server := NewMCPServer(svc)
	ctx := context.Background()

	serverTransport, clientTransport := mcpsdk.NewInMemoryTransports()
	go func() {
		_, _ = server.Connect(ctx, serverTransport, nil)
	}()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{
		Name:    "integration-test",
		Version: "1.0.0",
	}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	// Step 1: List tools
	toolsResult, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, toolsResult.Tools, 9, "should have 9 tools registered")

	// Step 2: Run analysis
	analyzeResult, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name: "analyze",
		Arguments: map[string]any{
			"label_selector": "konveyor.io/target=eap8",
			"reset_cache":    true,
		},
	})
	require.NoError(t, err)
	require.False(t, analyzeResult.IsError)
	assert.Equal(t, 1, callCount, "analyze should have been called once")

	// Step 3: Get cached results
	cachedResult, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name: "get_analysis_results",
	})
	require.NoError(t, err)
	require.False(t, cachedResult.IsError)
	text := cachedResult.Content[0].(*mcpsdk.TextContent).Text
	assert.Contains(t, text, "eap-rules-cached")

	// Step 4: Notify file changes
	notifyResult, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name: "notify_file_changes",
		Arguments: map[string]any{
			"changes": []map[string]any{
				{"path": "/src/MyBean.java", "saved": true},
			},
		},
	})
	require.NoError(t, err)
	require.False(t, notifyResult.IsError)

	// Step 5: List providers
	providersResult, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name: "list_providers",
	})
	require.NoError(t, err)
	require.False(t, providersResult.IsError)
	text = providersResult.Content[0].(*mcpsdk.TextContent).Text
	assert.Contains(t, text, "java")

	// Step 6: List rules
	rulesResult, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name: "list_rules",
	})
	require.NoError(t, err)
	require.False(t, rulesResult.IsError)
	text = rulesResult.Content[0].(*mcpsdk.TextContent).Text
	assert.Contains(t, text, "eap8-001")

	// Step 7: Get migration context
	ctxResult, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name: "get_migration_context",
	})
	require.NoError(t, err)
	require.False(t, ctxResult.IsError)
	text = ctxResult.Content[0].(*mcpsdk.TextContent).Text
	assert.Contains(t, text, "eap8")

	// Step 8: Validate rules
	validateResult, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name: "validate_rules",
		Arguments: map[string]any{
			"rules_content": "- ruleID: test-001\n  message: test\n  when:\n    builtin.file:\n      pattern: \"*.go\"\n",
		},
	})
	require.NoError(t, err)
	require.False(t, validateResult.IsError)
	text = validateResult.Content[0].(*mcpsdk.TextContent).Text
	var vr validationResult
	err = json.Unmarshal([]byte(text), &vr)
	require.NoError(t, err)
	assert.True(t, vr.Valid)
}

// TestIntegration_HTTPTransport tests the streamable HTTP transport.
func TestIntegration_HTTPTransport(t *testing.T) {
	svc := &mockAnalyzerService{
		listProvidersFunc: func() []ProviderInfo {
			return []ProviderInfo{
				{Name: "builtin", Capabilities: []string{"filecontent"}},
			}
		},
	}

	server := NewMCPServer(svc)

	handler := mcpsdk.NewStreamableHTTPHandler(func(r *http.Request) *mcpsdk.Server {
		return server
	}, nil)

	ts := httptest.NewServer(handler)
	defer ts.Close()

	ctx := context.Background()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{
		Name:    "http-test",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, &mcpsdk.StreamableClientTransport{
		Endpoint: ts.URL,
	}, nil)
	require.NoError(t, err)
	defer session.Close()

	// List tools over HTTP
	toolsResult, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, toolsResult.Tools, 9)

	// Call a tool over HTTP
	result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name: "list_providers",
	})
	require.NoError(t, err)
	require.False(t, result.IsError)
	text := result.Content[0].(*mcpsdk.TextContent).Text
	assert.Contains(t, text, "builtin")
}

// TestIntegration_BearerAuth tests OAuth 2.1 Bearer token auth on HTTP transport.
func TestIntegration_BearerAuth(t *testing.T) {
	svc := &mockAnalyzerService{
		listProvidersFunc: func() []ProviderInfo {
			return []ProviderInfo{
				{Name: "java", Capabilities: []string{"referenced"}},
			}
		},
	}

	server := NewMCPServer(svc)
	token := "test-secret-token"

	mcpHandler := mcpsdk.NewStreamableHTTPHandler(func(r *http.Request) *mcpsdk.Server {
		return server
	}, nil)
	handler := bearerAuthMiddleware(token, mcpHandler)

	ts := httptest.NewServer(handler)
	defer ts.Close()

	ctx := context.Background()

	// Without token — should get 401
	resp, err := http.Get(ts.URL)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	// With wrong token — should get 401
	req, _ := http.NewRequest("GET", ts.URL, nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	// With correct token — should work (connect via MCP client)
	client := mcpsdk.NewClient(&mcpsdk.Implementation{
		Name:    "auth-test",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, &mcpsdk.StreamableClientTransport{
		Endpoint: ts.URL,
		HTTPClient: &http.Client{
			Transport: &bearerTransport{Token: token},
		},
	}, nil)
	require.NoError(t, err)
	defer session.Close()

	result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name: "list_providers",
	})
	require.NoError(t, err)
	require.False(t, result.IsError)
	text := result.Content[0].(*mcpsdk.TextContent).Text
	assert.Contains(t, text, "java")
}

// bearerTransport is an http.RoundTripper that adds a Bearer token.
type bearerTransport struct {
	Token string
}

func (t *bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+t.Token)
	return http.DefaultTransport.RoundTrip(req)
}

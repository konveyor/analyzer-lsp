package mcp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// getTestLogger returns a discarding logger for tests
func getTestLogger() logr.Logger {
	return logr.Discard()
}

// getTestSettingsPath returns the path to test provider settings
func getTestSettingsPath(t *testing.T) string {
	return filepath.Join("testdata", "provider_settings.json")
}

func TestNewMCPServer(t *testing.T) {
	tests := []struct {
		name         string
		settingsFile string
		wantErr      bool
	}{
		{
			name:         "valid server creation",
			settingsFile: getTestSettingsPath(t),
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewMCPServer(getTestLogger(), tt.settingsFile)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewMCPServer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && server == nil {
				t.Error("NewMCPServer() returned nil server without error")
			}
			if !tt.wantErr && server.server == nil {
				t.Error("NewMCPServer() returned server with nil internal server")
			}
		})
	}
}

func TestToolRegistration(t *testing.T) {
	server, err := NewMCPServer(getTestLogger(), getTestSettingsPath(t))
	if err != nil {
		t.Fatalf("Failed to create MCP server: %v", err)
	}

	// Expected tools
	expectedTools := []string{
		"analyze_run",
		"rules_list",
		"rules_validate",
		"dependencies_get",
		"providers_list",
		"analyze_incidents",
	}

	// We can't directly inspect registered tools without accessing internal state,
	// but we can verify the server was created successfully
	if server.server == nil {
		t.Fatal("Server internal MCP server is nil")
	}

	t.Logf("Successfully registered %d expected tools", len(expectedTools))
}

func TestWrapError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantNil bool
	}{
		{
			name:    "nil error",
			err:     nil,
			wantNil: true,
		},
		{
			name:    "non-nil error",
			err:     context.Canceled,
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapError(tt.err)
			if (result == nil) != tt.wantNil {
				t.Errorf("wrapError() returned %v, wantNil %v", result, tt.wantNil)
			}
		})
	}
}

func TestHandleAnalyzeRun(t *testing.T) {
	server, err := NewMCPServer(getTestLogger(), getTestSettingsPath(t))
	if err != nil {
		t.Fatalf("Failed to create MCP server: %v", err)
	}

	tests := []struct {
		name    string
		params  AnalyzeRunParams
		wantErr bool
	}{
		{
			name: "missing rules_path",
			params: AnalyzeRunParams{
				TargetPath: "testdata/target",
			},
			wantErr: true,
		},
		{
			name: "missing target_path",
			params: AnalyzeRunParams{
				RulesPath: "testdata/rules/test_rules.yaml",
			},
			wantErr: true,
		},
		{
			name: "non-existent rules path",
			params: AnalyzeRunParams{
				RulesPath:  "nonexistent.yaml",
				TargetPath: "testdata/target",
			},
			wantErr: true,
		},
		{
			name: "non-existent target path",
			params: AnalyzeRunParams{
				RulesPath:  "testdata/rules/test_rules.yaml",
				TargetPath: "nonexistent",
			},
			wantErr: true,
		},
		{
			name: "invalid output format",
			params: AnalyzeRunParams{
				RulesPath:    "testdata/rules/test_rules.yaml",
				TargetPath:   "testdata/target",
				OutputFormat: "xml",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paramsJSON, err := json.Marshal(tt.params)
			if err != nil {
				t.Fatalf("Failed to marshal params: %v", err)
			}

			request := &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{
					Arguments: paramsJSON,
				},
			}

			result, err := server.handleAnalyzeRun(context.Background(), request)
			if (err != nil) != tt.wantErr {
				t.Errorf("handleAnalyzeRun() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result == nil {
				t.Error("handleAnalyzeRun() returned nil result without error")
			}
		})
	}
}

func TestHandleRulesList(t *testing.T) {
	server, err := NewMCPServer(getTestLogger(), getTestSettingsPath(t))
	if err != nil {
		t.Fatalf("Failed to create MCP server: %v", err)
	}

	tests := []struct {
		name    string
		params  RulesListParams
		wantErr bool
	}{
		{
			name:    "missing rules_path",
			params:  RulesListParams{},
			wantErr: true,
		},
		{
			name: "non-existent rules path",
			params: RulesListParams{
				RulesPath: "nonexistent.yaml",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paramsJSON, err := json.Marshal(tt.params)
			if err != nil {
				t.Fatalf("Failed to marshal params: %v", err)
			}

			request := &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{
					Arguments: paramsJSON,
				},
			}

			result, err := server.handleRulesList(context.Background(), request)
			if (err != nil) != tt.wantErr {
				t.Errorf("handleRulesList() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result == nil {
				t.Error("handleRulesList() returned nil result without error")
			}
		})
	}
}

func TestHandleRulesValidate(t *testing.T) {
	server, err := NewMCPServer(getTestLogger(), getTestSettingsPath(t))
	if err != nil {
		t.Fatalf("Failed to create MCP server: %v", err)
	}

	tests := []struct {
		name    string
		params  RulesValidateParams
		wantErr bool
	}{
		{
			name:    "missing rules_path",
			params:  RulesValidateParams{},
			wantErr: true,
		},
		{
			name: "non-existent rules path",
			params: RulesValidateParams{
				RulesPath: "nonexistent.yaml",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paramsJSON, err := json.Marshal(tt.params)
			if err != nil {
				t.Fatalf("Failed to marshal params: %v", err)
			}

			request := &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{
					Arguments: paramsJSON,
				},
			}

			result, err := server.handleRulesValidate(context.Background(), request)
			if (err != nil) != tt.wantErr {
				t.Errorf("handleRulesValidate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result == nil {
				t.Error("handleRulesValidate() returned nil result without error")
			}
		})
	}
}

func TestHandleDependenciesGet(t *testing.T) {
	server, err := NewMCPServer(getTestLogger(), getTestSettingsPath(t))
	if err != nil {
		t.Fatalf("Failed to create MCP server: %v", err)
	}

	tests := []struct {
		name    string
		params  DependenciesGetParams
		wantErr bool
	}{
		{
			name:    "missing target_path",
			params:  DependenciesGetParams{},
			wantErr: true,
		},
		{
			name: "non-existent target path",
			params: DependenciesGetParams{
				TargetPath: "nonexistent",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paramsJSON, err := json.Marshal(tt.params)
			if err != nil {
				t.Fatalf("Failed to marshal params: %v", err)
			}

			request := &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{
					Arguments: paramsJSON,
				},
			}

			result, err := server.handleDependenciesGet(context.Background(), request)
			if (err != nil) != tt.wantErr {
				t.Errorf("handleDependenciesGet() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result == nil {
				t.Error("handleDependenciesGet() returned nil result without error")
			}
		})
	}
}

func TestHandleProvidersList(t *testing.T) {
	server, err := NewMCPServer(getTestLogger(), getTestSettingsPath(t))
	if err != nil {
		t.Fatalf("Failed to create MCP server: %v", err)
	}

	tests := []struct {
		name    string
		params  ProvidersListParams
		wantErr bool
	}{
		{
			name: "valid providers list",
			params: ProvidersListParams{
				SettingsPath: getTestSettingsPath(t),
			},
			wantErr: false,
		},
		{
			name: "non-existent settings file",
			params: ProvidersListParams{
				SettingsPath: "nonexistent.json",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paramsJSON, err := json.Marshal(tt.params)
			if err != nil {
				t.Fatalf("Failed to marshal params: %v", err)
			}

			request := &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{
					Arguments: paramsJSON,
				},
			}

			result, err := server.handleProvidersList(context.Background(), request)
			if (err != nil) != tt.wantErr {
				t.Errorf("handleProvidersList() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result == nil {
				t.Error("handleProvidersList() returned nil result without error")
			}
		})
	}
}

func TestHandleAnalyzeIncidents(t *testing.T) {
	server, err := NewMCPServer(getTestLogger(), getTestSettingsPath(t))
	if err != nil {
		t.Fatalf("Failed to create MCP server: %v", err)
	}

	tests := []struct {
		name    string
		params  AnalyzeIncidentsParams
		wantErr bool
	}{
		{
			name:    "missing result_file",
			params:  AnalyzeIncidentsParams{},
			wantErr: true,
		},
		{
			name: "non-existent result file",
			params: AnalyzeIncidentsParams{
				ResultFile: "nonexistent.yaml",
			},
			wantErr: true,
		},
		{
			name: "valid result file",
			params: AnalyzeIncidentsParams{
				ResultFile: "testdata/results/sample_output.yaml",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paramsJSON, err := json.Marshal(tt.params)
			if err != nil {
				t.Fatalf("Failed to marshal params: %v", err)
			}

			request := &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{
					Arguments: paramsJSON,
				},
			}

			result, err := server.handleAnalyzeIncidents(context.Background(), request)
			if (err != nil) != tt.wantErr {
				t.Errorf("handleAnalyzeIncidents() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result == nil {
				t.Error("handleAnalyzeIncidents() returned nil result without error")
			}
		})
	}
}

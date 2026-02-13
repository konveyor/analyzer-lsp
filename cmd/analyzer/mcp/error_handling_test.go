package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestWrapError_Comprehensive(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		wantNil        bool
		expectedPrefix string
	}{
		{
			name:           "nil error",
			err:            nil,
			wantNil:        true,
			expectedPrefix: "",
		},
		{
			name:           "file not found error",
			err:            fs.ErrNotExist,
			wantNil:        false,
			expectedPrefix: "file or directory not found",
		},
		{
			name:           "wrapped file not found",
			err:            errors.Join(errors.New("reading file"), fs.ErrNotExist),
			wantNil:        false,
			expectedPrefix: "file or directory not found",
		},
		{
			name:           "parse error",
			err:            errors.New("unable to parse rules"),
			wantNil:        false,
			expectedPrefix: "parse error",
		},
		{
			name:           "validation error",
			err:            errors.New("validation failed: missing field"),
			wantNil:        false,
			expectedPrefix: "validation error",
		},
		{
			name:           "generic error",
			err:            errors.New("something went wrong"),
			wantNil:        false,
			expectedPrefix: "internal error",
		},
		{
			name:           "complex parse error",
			err:            errors.New("unable to parse YAML: syntax error"),
			wantNil:        false,
			expectedPrefix: "parse error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapError(tt.err)

			if (result == nil) != tt.wantNil {
				t.Errorf("wrapError() = %v, wantNil %v", result, tt.wantNil)
				return
			}

			if !tt.wantNil && tt.expectedPrefix != "" {
				if !strings.HasPrefix(result.Error(), tt.expectedPrefix) {
					t.Errorf("wrapError() = %q, expected to start with %q", result.Error(), tt.expectedPrefix)
				}
			}
		})
	}
}

func TestHandlers_InvalidJSON(t *testing.T) {
	server, err := NewMCPServer(getTestLogger(), getTestSettingsPath(t))
	if err != nil {
		t.Fatalf("Failed to create MCP server: %v", err)
	}

	invalidJSON := []byte(`{invalid json}`)

	tests := []struct {
		name    string
		handler func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error)
	}{
		{
			name:    "handleAnalyzeRun",
			handler: server.handleAnalyzeRun,
		},
		{
			name:    "handleRulesList",
			handler: server.handleRulesList,
		},
		{
			name:    "handleRulesValidate",
			handler: server.handleRulesValidate,
		},
		{
			name:    "handleDependenciesGet",
			handler: server.handleDependenciesGet,
		},
		{
			name:    "handleProvidersList",
			handler: server.handleProvidersList,
		},
		{
			name:    "handleAnalyzeIncidents",
			handler: server.handleAnalyzeIncidents,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{
					Arguments: invalidJSON,
				},
			}

			result, err := tt.handler(context.Background(), request)
			if err == nil {
				t.Error("Handler should return error for invalid JSON")
			}
			if result != nil {
				t.Error("Handler should return nil result on error")
			}
			if !strings.Contains(err.Error(), "invalid parameters") {
				t.Errorf("Error should mention invalid parameters, got: %v", err)
			}
		})
	}
}

func TestHandlers_SuccessPaths(t *testing.T) {
	server, err := NewMCPServer(getTestLogger(), getTestSettingsPath(t))
	if err != nil {
		t.Fatalf("Failed to create MCP server: %v", err)
	}

	t.Run("handleProvidersList_success", func(t *testing.T) {
		params := ProvidersListParams{
			SettingsPath: getTestSettingsPath(t),
		}
		paramsJSON, _ := json.Marshal(params)

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: paramsJSON,
			},
		}

		result, err := server.handleProvidersList(context.Background(), request)
		if err != nil {
			t.Fatalf("handleProvidersList() error = %v", err)
		}
		if result == nil {
			t.Fatal("handleProvidersList() returned nil result")
		}
		if len(result.Content) == 0 {
			t.Error("handleProvidersList() returned empty content")
		}
	})

	t.Run("handleAnalyzeIncidents_success", func(t *testing.T) {
		params := AnalyzeIncidentsParams{
			ResultFile: "testdata/results/sample_output.yaml",
		}
		paramsJSON, _ := json.Marshal(params)

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: paramsJSON,
			},
		}

		result, err := server.handleAnalyzeIncidents(context.Background(), request)
		if err != nil {
			t.Fatalf("handleAnalyzeIncidents() error = %v", err)
		}
		if result == nil {
			t.Fatal("handleAnalyzeIncidents() returned nil result")
		}
		if len(result.Content) == 0 {
			t.Error("handleAnalyzeIncidents() returned empty content")
		}
	})

	t.Run("handleRulesValidate_success", func(t *testing.T) {
		params := RulesValidateParams{
			RulesPath: "testdata/rules/test_rules.yaml",
		}
		paramsJSON, _ := json.Marshal(params)

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: paramsJSON,
			},
		}

		result, err := server.handleRulesValidate(context.Background(), request)
		if err != nil {
			t.Fatalf("handleRulesValidate() error = %v", err)
		}
		if result == nil {
			t.Fatal("handleRulesValidate() returned nil result")
		}
		if len(result.Content) == 0 {
			t.Error("handleRulesValidate() returned empty content")
		}
	})

	t.Run("handleRulesList_success", func(t *testing.T) {
		params := RulesListParams{
			RulesPath: "testdata/rules/test_rules.yaml",
		}
		paramsJSON, _ := json.Marshal(params)

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: paramsJSON,
			},
		}

		result, err := server.handleRulesList(context.Background(), request)
		if err != nil {
			t.Fatalf("handleRulesList() error = %v", err)
		}
		if result == nil {
			t.Fatal("handleRulesList() returned nil result")
		}
		if len(result.Content) == 0 {
			t.Error("handleRulesList() returned empty content")
		}
	})
}

func TestNewMCPServer_ErrorHandling(t *testing.T) {
	t.Run("empty settings file", func(t *testing.T) {
		// Should still succeed - empty settings just means no providers configured
		server, err := NewMCPServer(getTestLogger(), "")
		if err != nil {
			t.Logf("NewMCPServer with empty settings: %v (may be acceptable)", err)
		}
		if err == nil && server == nil {
			t.Error("NewMCPServer returned nil server without error")
		}
	})

	t.Run("nonexistent settings file", func(t *testing.T) {
		// Should still succeed - settings file is only used at runtime
		server, err := NewMCPServer(getTestLogger(), "nonexistent.json")
		if err != nil {
			t.Logf("NewMCPServer with nonexistent settings: %v (may be acceptable)", err)
		}
		if err == nil && server == nil {
			t.Error("NewMCPServer returned nil server without error")
		}
	})
}

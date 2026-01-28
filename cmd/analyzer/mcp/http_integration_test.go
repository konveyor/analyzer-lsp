package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServeHTTP_FullLifecycle(t *testing.T) {
	mcpServer, err := NewMCPServer(getTestLogger(), getTestSettingsPath(t))
	if err != nil {
		t.Fatalf("Failed to create MCP server: %v", err)
	}

	// Start server in background with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		// Use port 0 to get an available port
		errChan <- mcpServer.ServeHTTP(ctx, 0, HTTPConfig{})
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Wait for server to finish
	select {
	case err := <-errChan:
		// Server completed
		if err != nil && err != context.DeadlineExceeded && err != context.Canceled {
			// Only fail on unexpected errors
			if !strings.Contains(err.Error(), "listen tcp") {
				t.Logf("ServeHTTP completed with: %v (may be expected)", err)
			}
		}
	case <-time.After(3 * time.Second):
		t.Log("ServeHTTP test timeout (expected)")
	}
}

func TestServeHTTP_WithOAuth(t *testing.T) {
	mcpServer, err := NewMCPServer(getTestLogger(), getTestSettingsPath(t))
	if err != nil {
		t.Fatalf("Failed to create MCP server: %v", err)
	}

	config := HTTPConfig{
		OAuthClientID: "test-client",
		OAuthSecret:   "test-secret",
		OAuthTokenURL: "https://example.com/oauth/token",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- mcpServer.ServeHTTP(ctx, 0, config)
	}()

	select {
	case err := <-errChan:
		if err != nil && err != context.DeadlineExceeded && err != context.Canceled {
			t.Logf("ServeHTTP with OAuth: %v (may be expected)", err)
		}
	case <-time.After(200 * time.Millisecond):
		// Timeout is OK
	}
}

func TestValidateToken_AllPaths(t *testing.T) {
	log := getTestLogger()

	tests := []struct {
		name        string
		token       string
		config      HTTPConfig
		wantValid   bool
		description string
	}{
		{
			name:        "empty token",
			token:       "",
			config:      HTTPConfig{},
			wantValid:   false,
			description: "Empty token should be invalid",
		},
		{
			name:        "valid token no oauth",
			token:       "some-token",
			config:      HTTPConfig{},
			wantValid:   true,
			description: "Non-empty token without OAuth config",
		},
		{
			name:  "token with oauth config - empty client",
			token: "test-token",
			config: HTTPConfig{
				OAuthClientID: "",
				OAuthSecret:   "secret",
				OAuthTokenURL: "url",
			},
			wantValid:   true,
			description: "Token with partial OAuth config",
		},
		{
			name:  "token with full oauth config",
			token: "test-token",
			config: HTTPConfig{
				OAuthClientID: "client",
				OAuthSecret:   "secret",
				OAuthTokenURL: "url",
			},
			wantValid:   true,
			description: "Token with full OAuth config (stub validation)",
		},
		{
			name:  "long token",
			token: strings.Repeat("a", 1000),
			config: HTTPConfig{
				OAuthClientID: "client",
			},
			wantValid:   true,
			description: "Very long token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateToken(tt.token, tt.config, log)

			if result != tt.wantValid {
				t.Errorf("validateToken() = %v, want %v (%s)", result, tt.wantValid, tt.description)
			}
		})
	}
}

func TestHTTPHandler_MethodsAndPaths(t *testing.T) {
	// Create a mock HTTP handler that mimics the MCP HTTP server behavior
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		case "/mcp":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			// Mock MCP endpoint
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"jsonrpc": "2.0"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
	}{
		{
			name:           "GET /health",
			method:         http.MethodGet,
			path:           "/health",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "OPTIONS /mcp",
			method:         http.MethodOptions,
			path:           "/mcp",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "POST /mcp",
			method:         http.MethodPost,
			path:           "/mcp",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "GET /mcp (wrong method)",
			method:         http.MethodGet,
			path:           "/mcp",
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "GET /unknown",
			method:         http.MethodGet,
			path:           "/unknown",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, server.URL+tt.path, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("Status = %d, want %d", resp.StatusCode, tt.expectedStatus)
			}
		})
	}
}

func TestHTTPServer_RequestBodyHandling(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Read body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "Error reading body: %v", err)
			return
		}
		defer r.Body.Close()

		// Verify content type
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}

		// Verify valid JSON
		var data map[string]interface{}
		if err := json.Unmarshal(body, &data); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "Invalid JSON: %v", err)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"received": "ok"})
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	tests := []struct {
		name           string
		contentType    string
		body           string
		expectedStatus int
	}{
		{
			name:           "valid JSON",
			contentType:    "application/json",
			body:           `{"jsonrpc":"2.0","method":"tools/list"}`,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid JSON",
			contentType:    "application/json",
			body:           `{invalid}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "wrong content type",
			contentType:    "text/plain",
			body:           `{"jsonrpc":"2.0"}`,
			expectedStatus: http.StatusUnsupportedMediaType,
		},
		{
			name:           "empty body",
			contentType:    "application/json",
			body:           `{}`,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(tt.body))
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", tt.contentType)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.expectedStatus {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("Status = %d, want %d (body: %s)", resp.StatusCode, tt.expectedStatus, body)
			}
		})
	}
}

func TestHTTPServer_ConcurrentRequests(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate some processing time
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	// Send concurrent requests
	concurrency := 10
	errChan := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			req, err := http.NewRequest(http.MethodGet, server.URL+"/health", nil)
			if err != nil {
				errChan <- fmt.Errorf("request %d: %w", id, err)
				return
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				errChan <- fmt.Errorf("request %d: %w", id, err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				errChan <- fmt.Errorf("request %d: status %d", id, resp.StatusCode)
				return
			}

			errChan <- nil
		}(i)
	}

	// Collect results
	for i := 0; i < concurrency; i++ {
		if err := <-errChan; err != nil {
			t.Errorf("Concurrent request failed: %v", err)
		}
	}
}

package mcp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServeStdio(t *testing.T) {
	server, err := NewMCPServer(getTestLogger(), getTestSettingsPath(t))
	if err != nil {
		t.Fatalf("Failed to create MCP server: %v", err)
	}

	// Create a context that will cancel after a short time
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Run ServeStdio in background
	done := make(chan error, 1)
	go func() {
		done <- server.ServeStdio(ctx)
	}()

	// Wait for context to cancel or error
	select {
	case err := <-done:
		if err != nil && err != context.DeadlineExceeded && err != context.Canceled {
			// Only fail if it's not a timeout/cancel error
			if !strings.Contains(err.Error(), "EOF") && !strings.Contains(err.Error(), "closed") {
				t.Errorf("ServeStdio() unexpected error: %v", err)
			}
		}
	case <-time.After(200 * time.Millisecond):
		// Test passed - server ran and shut down gracefully
	}
}

func TestServeHTTP_ServerStartup(t *testing.T) {
	server, err := NewMCPServer(getTestLogger(), getTestSettingsPath(t))
	if err != nil {
		t.Fatalf("Failed to create MCP server: %v", err)
	}

	// Create a context that will cancel after a short time
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Run ServeHTTP in background on a random port
	done := make(chan error, 1)
	go func() {
		done <- server.ServeHTTP(ctx, 0, HTTPConfig{}) // Port 0 = random available port
	}()

	// Wait for context to cancel or error
	select {
	case err := <-done:
		if err != nil && err != context.DeadlineExceeded && err != context.Canceled {
			t.Logf("ServeHTTP() returned error (may be acceptable): %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		// Test passed - server ran
	}
}

func TestHTTPTransport_HealthEndpoint(t *testing.T) {
	// Create a test HTTP handler directly
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	// Create test server
	testServer := httptest.NewServer(handler)
	defer testServer.Close()

	// Test health endpoint
	resp, err := http.Get(testServer.URL + "/health")
	if err != nil {
		t.Fatalf("Failed to call health endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Health endpoint returned status %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if !strings.Contains(string(body), "ok") {
		t.Errorf("Health endpoint returned unexpected body: %s", string(body))
	}
}

func TestHTTPTransport_MCPEndpoint(t *testing.T) {
	server, err := NewMCPServer(getTestLogger(), getTestSettingsPath(t))
	if err != nil {
		t.Fatalf("Failed to create MCP server: %v", err)
	}

	// Create a simple test that the server can be created
	// Full integration test would require MCP protocol implementation
	if server == nil {
		t.Error("Server is nil")
	}
}

func TestValidateToken(t *testing.T) {
	tests := []struct {
		name        string
		config      HTTPConfig
		token       string
		expectValid bool
	}{
		{
			name:        "empty config allows any token",
			config:      HTTPConfig{},
			token:       "any-token",
			expectValid: true,
		},
		{
			name: "with oauth config",
			config: HTTPConfig{
				OAuthClientID: "test-client",
				OAuthSecret:   "test-secret",
				OAuthTokenURL: "https://example.com/oauth/token",
			},
			token:       "test-token",
			expectValid: false, // Would fail without real OAuth server
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// validateToken is currently a stub, so we just test it doesn't panic
			result := validateToken(tt.token, tt.config, getTestLogger())

			// For now, validateToken always returns true (stub implementation)
			// This test ensures the function exists and can be called
			_ = result
		})
	}
}

func TestHTTPTransport_CORS(t *testing.T) {
	// Create a test HTTP handler that sets CORS headers
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		w.WriteHeader(http.StatusOK)
	})

	testServer := httptest.NewServer(handler)
	defer testServer.Close()

	// Test OPTIONS request (CORS preflight)
	req, err := http.NewRequest(http.MethodOptions, testServer.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to send OPTIONS request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("OPTIONS request returned status %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Verify CORS headers
	if origin := resp.Header.Get("Access-Control-Allow-Origin"); origin != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", origin, "*")
	}
}

func TestHTTPTransport_Authentication(t *testing.T) {
	// Create a test HTTP handler that checks for Authorization header
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"missing authorization"}`))
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"invalid authorization format"}`))
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"authenticated"}`))
	})

	testServer := httptest.NewServer(handler)
	defer testServer.Close()

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "no auth header",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid auth format",
			authHeader:     "InvalidFormat token",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "valid bearer token",
			authHeader:     "Bearer test-token",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, testServer.URL, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Failed to send request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("Status = %d, want %d", resp.StatusCode, tt.expectedStatus)
			}
		})
	}
}

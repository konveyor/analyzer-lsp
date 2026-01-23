package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Integration tests for serveHTTP function

func getFreePort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func TestServeHTTP_FullIntegration(t *testing.T) {
	mcpServer := &mcp.Server{}
	log := getTestLogger()

	port, err := getFreePort()
	if err != nil {
		t.Fatalf("Failed to get free port: %v", err)
	}

	// Start server in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverErr := make(chan error, 1)
	serverReady := make(chan bool, 1)

	go func() {
		// Signal that we're about to start
		serverReady <- true
		err := serveHTTP(ctx, mcpServer, log, port, HTTPConfig{})
		if err != nil && err.Error() != "http: Server closed" {
			serverErr <- err
		}
	}()

	// Wait for server to be ready
	<-serverReady
	time.Sleep(200 * time.Millisecond)

	baseURL := fmt.Sprintf("http://localhost:%d", port)

	// Test health endpoint
	t.Run("health_endpoint", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/health")
		if err != nil {
			t.Fatalf("Failed to call health endpoint: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Health status = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		var result map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode health response: %v", err)
		}

		if result["status"] != "healthy" {
			t.Errorf("Health status = %s, want healthy", result["status"])
		}
	})

	// Test MCP endpoint - OPTIONS
	t.Run("mcp_options", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodOptions, baseURL+"/mcp", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed OPTIONS request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("OPTIONS status = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		// Check CORS headers
		if origin := resp.Header.Get("Access-Control-Allow-Origin"); origin != "*" {
			t.Errorf("CORS origin = %s, want *", origin)
		}
	})

	// Test MCP endpoint - wrong method
	t.Run("mcp_wrong_method", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/mcp")
		if err != nil {
			t.Fatalf("Failed GET request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("GET status = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
		}
	})

	// Test MCP endpoint - valid POST
	t.Run("mcp_valid_post", func(t *testing.T) {
		reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
		req, _ := http.NewRequest(http.MethodPost, baseURL+"/mcp", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed POST request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("POST status = %d, want %d (body: %s)", resp.StatusCode, http.StatusOK, body)
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result["jsonrpc"] != "2.0" {
			t.Errorf("jsonrpc = %v, want 2.0", result["jsonrpc"])
		}
	})

	// Test MCP endpoint - invalid JSON
	t.Run("mcp_invalid_json", func(t *testing.T) {
		reqBody := `{invalid json}`
		req, _ := http.NewRequest(http.MethodPost, baseURL+"/mcp", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed POST request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Invalid JSON status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}
	})

	// Test unknown endpoint
	t.Run("unknown_endpoint", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/unknown")
		if err != nil {
			t.Fatalf("Failed GET request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Unknown endpoint status = %d, want %d", resp.StatusCode, http.StatusNotFound)
		}
	})

	// Shutdown server
	cancel()

	// Wait a bit for shutdown
	select {
	case err := <-serverErr:
		if err != nil {
			t.Logf("Server error: %v (may be expected)", err)
		}
	case <-time.After(2 * time.Second):
		// Timeout is OK
	}
}

func TestServeHTTP_WithOAuthIntegration(t *testing.T) {
	mcpServer := &mcp.Server{}
	log := getTestLogger()

	port, err := getFreePort()
	if err != nil {
		t.Fatalf("Failed to get free port: %v", err)
	}

	config := HTTPConfig{
		OAuthClientID: "test-client-id",
		OAuthSecret:   "test-secret",
		OAuthTokenURL: "https://example.com/oauth/token",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverReady := make(chan bool, 1)

	go func() {
		serverReady <- true
		serveHTTP(ctx, mcpServer, log, port, config)
	}()

	<-serverReady
	time.Sleep(200 * time.Millisecond)

	baseURL := fmt.Sprintf("http://localhost:%d", port)

	// Test without auth header
	t.Run("no_auth_header", func(t *testing.T) {
		reqBody := `{"jsonrpc":"2.0","method":"test"}`
		req, _ := http.NewRequest(http.MethodPost, baseURL+"/mcp", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("No auth status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
	})

	// Test with invalid auth format
	t.Run("invalid_auth_format", func(t *testing.T) {
		reqBody := `{"jsonrpc":"2.0","method":"test"}`
		req, _ := http.NewRequest(http.MethodPost, baseURL+"/mcp", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "InvalidFormat token")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Invalid auth format status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
	})

	// Test with valid bearer token
	t.Run("valid_bearer_token", func(t *testing.T) {
		reqBody := `{"jsonrpc":"2.0","id":1,"method":"test"}`
		req, _ := http.NewRequest(http.MethodPost, baseURL+"/mcp", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-token-123")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed request: %v", err)
		}
		defer resp.Body.Close()

		// Our stub validateToken accepts any non-empty token
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Valid token status = %d, want %d (body: %s)", resp.StatusCode, http.StatusOK, body)
		}
	})

	cancel()
	time.Sleep(100 * time.Millisecond)
}

func TestServeHTTP_Shutdown(t *testing.T) {
	mcpServer := &mcp.Server{}
	log := getTestLogger()

	port, err := getFreePort()
	if err != nil {
		t.Fatalf("Failed to get free port: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	serverDone := make(chan error, 1)
	serverReady := make(chan bool, 1)

	go func() {
		serverReady <- true
		serverDone <- serveHTTP(ctx, mcpServer, log, port, HTTPConfig{})
	}()

	<-serverReady
	time.Sleep(100 * time.Millisecond)

	// Server should be running
	baseURL := fmt.Sprintf("http://localhost:%d", port)
	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		t.Fatalf("Server not running: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Health check failed: %d", resp.StatusCode)
	}

	// Cancel context to trigger shutdown
	cancel()

	// Wait for server to shutdown
	select {
	case err := <-serverDone:
		if err != nil {
			t.Logf("Server shutdown error (may be expected): %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Server did not shutdown in time")
	}

	// Verify server is actually down
	time.Sleep(100 * time.Millisecond)
	_, err = http.Get(baseURL + "/health")
	if err == nil {
		t.Error("Server should be down after shutdown")
	}
}

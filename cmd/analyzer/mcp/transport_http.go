package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// serveHTTP starts the MCP server using HTTP transport with OAuth 2.1 authentication
func serveHTTP(ctx context.Context, server *mcp.Server, log logr.Logger, port int, config HTTPConfig) error {
	log.Info("starting MCP server with HTTP transport", "port", port)

	// Create HTTP handler
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "healthy",
			"server": "konveyor-analyzer-mcp",
		})
	})

	// MCP endpoint with authentication
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		// Add CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Handle preflight requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Only accept POST requests
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// OAuth 2.1 authentication (if configured)
		if config.OAuthClientID != "" {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}

			// Validate Bearer token
			if !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			if !validateToken(token, config, log) {
				http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
				return
			}
		}

		// Limit request size to prevent abuse (10MB)
		r.Body = http.MaxBytesReader(w, r.Body, 10*1024*1024)

		// Parse MCP request
		var mcpRequest map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&mcpRequest); err != nil {
			http.Error(w, "Invalid JSON request", http.StatusBadRequest)
			return
		}

		// Log the request
		log.V(7).Info("received MCP request", "method", mcpRequest["method"])

		// Forward to MCP server (note: the SDK may not have a direct HTTP handler)
		// For now, we'll return a stub response
		// TODO: Integrate with actual MCP SDK HTTP handler when available
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      mcpRequest["id"],
			"result":  map[string]string{"status": "not_implemented"},
		})
	})

	// Create HTTP server
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		log.Info("HTTP server listening", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		log.Info("shutting down HTTP server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-serverErr:
		return err
	}
}

// validateToken validates an OAuth 2.1 Bearer token
// This is a simplified implementation - in production, you would validate
// against the OAuth token introspection endpoint
func validateToken(token string, config HTTPConfig, log logr.Logger) bool {
	// TODO: Implement actual OAuth 2.1 token validation
	// This should:
	// 1. Validate token signature (if JWT)
	// 2. Check token expiration
	// 3. Verify token against OAuth server introspection endpoint
	// 4. Check token scopes/claims

	// For now, just check if token is not empty
	if token == "" {
		return false
	}

	// In a real implementation, you would make a request to config.OAuthTokenURL
	// to validate the token
	log.V(7).Info("validating token", "token_length", len(token))

	// Placeholder: Accept any non-empty token
	// TODO: Implement proper validation
	return true
}

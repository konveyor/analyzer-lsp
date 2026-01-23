package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	logrusr "github.com/bombsimon/logrusr/v3"
	"github.com/konveyor/analyzer-lsp/cmd/analyzer/mcp"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	mcpTransport      string
	mcpPort           int
	mcpLogLevel       int
	mcpOAuthClientID  string
	mcpOAuthSecret    string
	mcpOAuthTokenURL  string
)

func MCPCmd() *cobra.Command {
	mcpCmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start MCP server for AI assistant integration",
		Long:  "Model Context Protocol (MCP) server that exposes Konveyor Analyzer capabilities to AI assistants and other MCP clients",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Setup logging to stderr
			logrusLog := logrus.New()
			logrusLog.SetOutput(os.Stderr)
			logrusLog.SetFormatter(&logrus.TextFormatter{})
			logrusLog.SetLevel(logrus.Level(mcpLogLevel))
			log := logrusr.New(logrusLog)

			// Create context with cancellation for graceful shutdown
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Setup signal handling
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

			// Create and start the MCP server
			server, err := mcp.NewMCPServer(log, settingsFile)
			if err != nil {
				log.Error(err, "failed to create MCP server")
				return err
			}

			// Start server in goroutine
			errChan := make(chan error, 1)
			go func() {
				log.Info("starting MCP server", "transport", mcpTransport)
				var err error
				switch mcpTransport {
				case "stdio":
					err = server.ServeStdio(ctx)
				case "http":
					err = server.ServeHTTP(ctx, mcpPort, mcp.HTTPConfig{
						OAuthClientID: mcpOAuthClientID,
						OAuthSecret:   mcpOAuthSecret,
						OAuthTokenURL: mcpOAuthTokenURL,
					})
				default:
					log.Error(nil, "unsupported transport type", "transport", mcpTransport)
					err = nil
				}
				errChan <- err
			}()

			// Wait for shutdown signal or error
			select {
			case <-sigChan:
				log.Info("received shutdown signal, stopping server")
				cancel()
			case err := <-errChan:
				if err != nil {
					log.Error(err, "server error")
					return err
				}
			}

			return nil
		},
	}

	// Define flags
	mcpCmd.Flags().StringVar(&mcpTransport, "transport", "stdio", "Transport type (stdio or http)")
	mcpCmd.Flags().IntVar(&mcpPort, "port", 8080, "Port for HTTP transport")
	mcpCmd.Flags().IntVar(&mcpLogLevel, "log-level", 5, "Log level (0-9, higher is more verbose)")
	mcpCmd.Flags().StringVar(&mcpOAuthClientID, "oauth-client-id", os.Getenv("MCP_OAUTH_CLIENT_ID"), "OAuth 2.1 client ID")
	mcpCmd.Flags().StringVar(&mcpOAuthSecret, "oauth-client-secret", os.Getenv("MCP_OAUTH_CLIENT_SECRET"), "OAuth 2.1 client secret")
	mcpCmd.Flags().StringVar(&mcpOAuthTokenURL, "oauth-token-url", os.Getenv("MCP_OAUTH_TOKEN_URL"), "OAuth 2.1 token URL")

	return mcpCmd
}

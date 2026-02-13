package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPServer wraps the MCP server and provides tool handling
type MCPServer struct {
	server       *mcp.Server
	log          logr.Logger
	settingsFile string
}

// HTTPConfig holds HTTP transport configuration
type HTTPConfig struct {
	OAuthClientID string
	OAuthSecret   string
	OAuthTokenURL string
}

// NewMCPServer creates a new MCP server with all tools registered
func NewMCPServer(log logr.Logger, settingsFile string) (*MCPServer, error) {
	s := &MCPServer{
		log:          log,
		settingsFile: settingsFile,
	}

	// Create the MCP server instance
	mcpServer := mcp.NewServer(
		&mcp.Implementation{
			Name:    "konveyor-analyzer-mcp",
			Version: "0.1.0",
		},
		nil,
	)

	// Register all tools
	if err := s.registerTools(mcpServer); err != nil {
		return nil, fmt.Errorf("failed to register tools: %w", err)
	}

	s.server = mcpServer
	return s, nil
}

// registerTools registers all available MCP tools
func (s *MCPServer) registerTools(server *mcp.Server) error {
	// Tool 1: analyze_run
	server.AddTool(
		&mcp.Tool{
			Name:        "analyze_run",
			Description: "Run analysis on a codebase using specified rules",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"rules_path": {"type": "string", "description": "Path to rules file or directory"},
					"target_path": {"type": "string", "description": "Path to codebase to analyze"},
					"label_selector": {"type": "string", "description": "Label filter expression (optional)"},
					"output_format": {"type": "string", "description": "Output format: 'json' or 'yaml' (default: yaml)", "enum": ["json", "yaml"]},
					"incident_limit": {"type": "integer", "description": "Maximum incidents per rule (default: 1500)"}
				},
				"required": ["rules_path", "target_path"]
			}`),
		},
		s.handleAnalyzeRun,
	)

	// Tool 2: rules_list
	server.AddTool(
		&mcp.Tool{
			Name:        "rules_list",
			Description: "List all available rules from a rules file or directory",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"rules_path": {"type": "string", "description": "Path to rules file or directory"},
					"label_filter": {"type": "string", "description": "Filter rules by label expression (optional)"}
				},
				"required": ["rules_path"]
			}`),
		},
		s.handleRulesList,
	)

	// Tool 3: rules_validate
	server.AddTool(
		&mcp.Tool{
			Name:        "rules_validate",
			Description: "Validate rules syntax and structure",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"rules_path": {"type": "string", "description": "Path to rules file or directory to validate"}
				},
				"required": ["rules_path"]
			}`),
		},
		s.handleRulesValidate,
	)

	// Tool 4: dependencies_get
	server.AddTool(
		&mcp.Tool{
			Name:        "dependencies_get",
			Description: "Get dependencies from a codebase",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"target_path": {"type": "string", "description": "Path to codebase"},
					"provider_settings": {"type": "string", "description": "Path to provider settings (optional)"},
					"tree_format": {"type": "boolean", "description": "Output as tree structure instead of flat list (default: false)"},
					"label_selector": {"type": "string", "description": "Filter dependencies by label expression (optional)"}
				},
				"required": ["target_path"]
			}`),
		},
		s.handleDependenciesGet,
	)

	// Tool 5: providers_list
	server.AddTool(
		&mcp.Tool{
			Name:        "providers_list",
			Description: "List all available analysis providers and their capabilities",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"settings_path": {"type": "string", "description": "Path to provider settings file (optional, default: provider_settings.json)"}
				}
			}`),
		},
		s.handleProvidersList,
	)

	// Tool 6: analyze_incidents
	server.AddTool(
		&mcp.Tool{
			Name:        "analyze_incidents",
			Description: "Query and filter incidents from an analysis result file",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"result_file": {"type": "string", "description": "Path to analysis output file"},
					"rule_id": {"type": "string", "description": "Filter by specific rule ID (optional)"},
					"limit": {"type": "integer", "description": "Maximum number of incidents to return (optional)"}
				},
				"required": ["result_file"]
			}`),
		},
		s.handleAnalyzeIncidents,
	)

	return nil
}

// wrapError converts common errors to MCP protocol errors
func wrapError(err error) error {
	if err == nil {
		return nil
	}

	// Map common errors to appropriate error codes
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("file or directory not found: %w", err)
	case strings.Contains(err.Error(), "unable to parse"):
		return fmt.Errorf("parse error: %w", err)
	case strings.Contains(err.Error(), "validation"):
		return fmt.Errorf("validation error: %w", err)
	default:
		return fmt.Errorf("internal error: %w", err)
	}
}

// Tool handlers delegate to the tool implementations
func (s *MCPServer) handleAnalyzeRun(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var params AnalyzeRunParams
	if err := json.Unmarshal(request.Params.Arguments, &params); err != nil {
		return nil, wrapError(fmt.Errorf("invalid parameters: %w", err))
	}

	result, err := analyzeRun(ctx, s.log, s.settingsFile, params)
	if err != nil {
		return nil, wrapError(err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: result},
		},
	}, nil
}

func (s *MCPServer) handleRulesList(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var params RulesListParams
	if err := json.Unmarshal(request.Params.Arguments, &params); err != nil {
		return nil, wrapError(fmt.Errorf("invalid parameters: %w", err))
	}

	result, err := rulesList(ctx, s.log, s.settingsFile, params)
	if err != nil {
		return nil, wrapError(err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: result},
		},
	}, nil
}

func (s *MCPServer) handleRulesValidate(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var params RulesValidateParams
	if err := json.Unmarshal(request.Params.Arguments, &params); err != nil {
		return nil, wrapError(fmt.Errorf("invalid parameters: %w", err))
	}

	result, err := rulesValidate(ctx, s.log, s.settingsFile, params)
	if err != nil {
		return nil, wrapError(err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: result},
		},
	}, nil
}

func (s *MCPServer) handleDependenciesGet(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var params DependenciesGetParams
	if err := json.Unmarshal(request.Params.Arguments, &params); err != nil {
		return nil, wrapError(fmt.Errorf("invalid parameters: %w", err))
	}

	result, err := dependenciesGet(ctx, s.log, s.settingsFile, params)
	if err != nil {
		return nil, wrapError(err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: result},
		},
	}, nil
}

func (s *MCPServer) handleProvidersList(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var params ProvidersListParams
	if err := json.Unmarshal(request.Params.Arguments, &params); err != nil {
		return nil, wrapError(fmt.Errorf("invalid parameters: %w", err))
	}

	result, err := providersList(ctx, s.log, s.settingsFile, params)
	if err != nil {
		return nil, wrapError(err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: result},
		},
	}, nil
}

func (s *MCPServer) handleAnalyzeIncidents(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var params AnalyzeIncidentsParams
	if err := json.Unmarshal(request.Params.Arguments, &params); err != nil {
		return nil, wrapError(fmt.Errorf("invalid parameters: %w", err))
	}

	result, err := analyzeIncidents(ctx, s.log, params)
	if err != nil {
		return nil, wrapError(err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: result},
		},
	}, nil
}

// ServeStdio starts the MCP server using stdio transport
func (s *MCPServer) ServeStdio(ctx context.Context) error {
	s.log.Info("starting MCP server with stdio transport")

	// Create stdio transport
	transport := &mcp.StdioTransport{}

	// Connect the server to the transport
	session, err := s.server.Connect(ctx, transport, nil)
	if err != nil {
		s.log.Error(err, "failed to connect server to stdio transport")
		return err
	}
	defer session.Close()

	// Wait for context cancellation
	<-ctx.Done()
	s.log.Info("stdio server stopped")
	return nil
}

// ServeHTTP starts the MCP server using HTTP transport
func (s *MCPServer) ServeHTTP(ctx context.Context, port int, config HTTPConfig) error {
	return serveHTTP(ctx, s.server, s.log, port, config)
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	konveyor "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	"gopkg.in/yaml.v2"
)

// NewMCPServer creates a new MCP server with all tools registered.
func NewMCPServer(svc AnalyzerService) *mcpsdk.Server {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "konveyor-analyzer",
		Version: "0.10.0",
	}, nil)

	registerAnalyzeTool(server, svc)
	registerGetAnalysisResultsTool(server, svc)
	registerNotifyFileChangesTool(server, svc)
	registerValidateRulesTool(server, svc)
	registerListRulesTool(server, svc)
	registerListProvidersTool(server, svc)
	registerGetDependenciesTool(server, svc)
	registerAnalyzeIncidentsTool(server, svc)
	registerGetMigrationContextTool(server, svc)

	return server
}

// --- analyze ---

type analyzeInput struct {
	LabelSelector    string   `json:"label_selector,omitempty" jsonschema:"Label selector to filter rules (e.g. konveyor.io/target=eap8)"`
	IncidentSelector string   `json:"incident_selector,omitempty" jsonschema:"Selector for filtering incidents in results"`
	IncludedPaths    []string `json:"included_paths,omitempty" jsonschema:"File paths to scope analysis to (for incremental re-analysis)"`
	ExcludedPaths    []string `json:"excluded_paths,omitempty" jsonschema:"File paths to exclude from analysis"`
	ResetCache       bool     `json:"reset_cache,omitempty" jsonschema:"If true forces a full re-analysis ignoring cached results"`
}

func registerAnalyzeTool(server *mcpsdk.Server, svc AnalyzerService) {
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "analyze",
		Description: "Run static code analysis using Konveyor rules. Returns violations with file locations, descriptions, and effort estimates. Supports incremental analysis via included_paths for fast re-analysis of changed files.",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input analyzeInput) (*mcpsdk.CallToolResult, any, error) {
		rulesets, err := svc.Analyze(AnalyzeParams{
			LabelSelector:    input.LabelSelector,
			IncidentSelector: input.IncidentSelector,
			IncludedPaths:    input.IncludedPaths,
			ExcludedPaths:    input.ExcludedPaths,
			ResetCache:       input.ResetCache,
		})
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(rulesets)
	})
}

// --- get_analysis_results ---

type getAnalysisResultsInput struct{}

func registerGetAnalysisResultsTool(server *mcpsdk.Server, svc AnalyzerService) {
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "get_analysis_results",
		Description: "Return cached analysis results from the most recent analysis run without re-executing. Fast way to retrieve previously computed violations.",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input getAnalysisResultsInput) (*mcpsdk.CallToolResult, any, error) {
		rulesets := svc.GetCachedResults()
		return jsonResult(rulesets)
	})
}

// --- notify_file_changes ---

type notifyFileChangesInput struct {
	Changes []fileChange `json:"changes" jsonschema:"List of file changes to notify providers about"`
}

type fileChange struct {
	Path    string `json:"path" jsonschema:"File path (e.g. /path/to/file.java)"`
	Content string `json:"content,omitempty" jsonschema:"Optional file content"`
	Saved   bool   `json:"saved,omitempty" jsonschema:"Whether the file has been saved to disk"`
}

func registerNotifyFileChangesTool(server *mcpsdk.Server, svc AnalyzerService) {
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "notify_file_changes",
		Description: "Notify analysis providers that files have changed. Call this before running incremental analysis with included_paths so providers can update their internal state.",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input notifyFileChangesInput) (*mcpsdk.CallToolResult, any, error) {
		changes := make([]provider.FileChange, len(input.Changes))
		for i, c := range input.Changes {
			changes[i] = provider.FileChange{
				Path:    c.Path,
				Content: c.Content,
				Saved:   c.Saved,
			}
		}
		err := svc.NotifyFileChanges(changes)
		if err != nil {
			return nil, nil, err
		}
		return textResult("File changes notified successfully")
	})
}

// --- validate_rules ---

type validateRulesInput struct {
	RulesContent string `json:"rules_content,omitempty" jsonschema:"Inline YAML rule content to validate"`
	RulesPath    string `json:"rules_path,omitempty" jsonschema:"Path to a rule YAML file to validate"`
}

type validationResult struct {
	Valid    bool     `json:"valid"`
	Errors  []string `json:"errors,omitempty"`
	Message string   `json:"message,omitempty"`
}

func registerValidateRulesTool(server *mcpsdk.Server, svc AnalyzerService) {
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "validate_rules",
		Description: "Validate Konveyor rule YAML syntax and structure. Provide either inline YAML content or a file path. Returns validation errors if any.",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input validateRulesInput) (*mcpsdk.CallToolResult, any, error) {
		content := input.RulesContent
		if content == "" && input.RulesPath != "" {
			data, err := os.ReadFile(input.RulesPath)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to read rules file: %w", err)
			}
			content = string(data)
		}
		if content == "" {
			return nil, nil, fmt.Errorf("either rules_content or rules_path must be provided")
		}

		result := validateRuleYAML(content)
		return jsonResult(result)
	})
}

func validateRuleYAML(content string) validationResult {
	var parsed interface{}
	err := yaml.Unmarshal([]byte(content), &parsed)
	if err != nil {
		return validationResult{
			Valid:  false,
			Errors: []string{fmt.Sprintf("YAML parse error: %v", err)},
		}
	}

	// Validate structure: should be a list or a map with expected fields
	errors := []string{}

	switch v := parsed.(type) {
	case []interface{}:
		for i, item := range v {
			if m, ok := item.(map[interface{}]interface{}); ok {
				if errs := validateRulesetStructure(m); len(errs) > 0 {
					for _, e := range errs {
						errors = append(errors, fmt.Sprintf("ruleset[%d]: %s", i, e))
					}
				}
			} else {
				errors = append(errors, fmt.Sprintf("ruleset[%d]: expected a mapping, got %T", i, item))
			}
		}
	case map[interface{}]interface{}:
		if errs := validateRulesetStructure(v); len(errs) > 0 {
			errors = append(errors, errs...)
		}
	default:
		errors = append(errors, fmt.Sprintf("expected a YAML mapping or list, got %T", parsed))
	}

	if len(errors) > 0 {
		return validationResult{Valid: false, Errors: errors}
	}
	return validationResult{Valid: true, Message: "Rule YAML is valid"}
}

func validateRulesetStructure(m map[interface{}]interface{}) []string {
	var errors []string
	knownKeys := map[string]bool{
		"name": true, "description": true, "labels": true,
		"tags": true, "rules": true, "when": true, "message": true,
		"ruleID": true, "effort": true, "perform": true,
		"category": true, "customVariables": true, "links": true,
		"tag": true,
	}

	for k := range m {
		key := fmt.Sprintf("%v", k)
		if !knownKeys[key] {
			// Not an error, just note unknown keys for extensibility
			_ = key
		}
	}

	// Check that if "rules" is present, it's a list
	if rules, ok := m["rules"]; ok {
		if _, ok := rules.([]interface{}); !ok && rules != nil {
			errors = append(errors, "'rules' field must be a list")
		}
	}

	return errors
}

// --- list_rules ---

type listRulesInput struct {
	LabelSelector string `json:"label_selector,omitempty" jsonschema:"Optional label selector to filter rules"`
}

func registerListRulesTool(server *mcpsdk.Server, svc AnalyzerService) {
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "list_rules",
		Description: "List all loaded analysis rules with their IDs, descriptions, labels, and ruleset names. Optionally filter by label selector.",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input listRulesInput) (*mcpsdk.CallToolResult, any, error) {
		rules := svc.ListRules()

		// Filter by label selector if provided
		if input.LabelSelector != "" {
			filtered := []RuleInfo{}
			for _, r := range rules {
				for _, label := range r.Labels {
					if strings.Contains(label, input.LabelSelector) {
						filtered = append(filtered, r)
						break
					}
				}
			}
			rules = filtered
		}

		return jsonResult(rules)
	})
}

// --- list_providers ---

type listProvidersInput struct{}

func registerListProvidersTool(server *mcpsdk.Server, svc AnalyzerService) {
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "list_providers",
		Description: "List all available analysis providers and their capabilities (e.g. java, builtin, go, python).",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input listProvidersInput) (*mcpsdk.CallToolResult, any, error) {
		providers := svc.ListProviders()
		return jsonResult(providers)
	})
}

// --- get_dependencies ---

type getDependenciesInput struct {
	LabelSelector string `json:"label_selector,omitempty" jsonschema:"Optional label selector to filter dependencies"`
}

func registerGetDependenciesTool(server *mcpsdk.Server, svc AnalyzerService) {
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "get_dependencies",
		Description: "Extract project dependencies from all providers. Returns a flat list of dependencies with provider info and file URIs.",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input getDependenciesInput) (*mcpsdk.CallToolResult, any, error) {
		deps, err := svc.GetDependencies()
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(deps)
	})
}

// --- analyze_incidents ---

type analyzeIncidentsInput struct {
	FilePath string `json:"file_path,omitempty" jsonschema:"Filter incidents by file path (substring match)"`
	RuleID   string `json:"rule_id,omitempty" jsonschema:"Filter incidents by rule ID"`
	Category string `json:"category,omitempty" jsonschema:"Filter by category: potential, optional, or mandatory"`
}

func registerAnalyzeIncidentsTool(server *mcpsdk.Server, svc AnalyzerService) {
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "analyze_incidents",
		Description: "Query and filter incidents from the most recent analysis results. Filter by file path, rule ID, or category to narrow down specific violations.",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input analyzeIncidentsInput) (*mcpsdk.CallToolResult, any, error) {
		rulesets := svc.GetCachedResults()

		type incidentResult struct {
			RuleSetName   string             `json:"ruleset_name"`
			RuleID        string             `json:"rule_id"`
			Description   string             `json:"description"`
			Category      *konveyor.Category `json:"category,omitempty"`
			Effort        *int               `json:"effort,omitempty"`
			konveyor.Incident
		}

		var results []incidentResult
		for _, rs := range rulesets {
			for ruleID, v := range rs.Violations {
				if input.RuleID != "" && ruleID != input.RuleID {
					continue
				}
				if input.Category != "" && (v.Category == nil || string(*v.Category) != input.Category) {
					continue
				}
				for _, inc := range v.Incidents {
					if input.FilePath != "" && !strings.Contains(string(inc.URI), input.FilePath) {
						continue
					}
					results = append(results, incidentResult{
						RuleSetName: rs.Name,
						RuleID:      ruleID,
						Description: v.Description,
						Category:    v.Category,
						Effort:      v.Effort,
						Incident:    inc,
					})
				}
			}
		}

		return jsonResult(results)
	})
}

// --- get_migration_context ---

type getMigrationContextInput struct{}

func registerGetMigrationContextTool(server *mcpsdk.Server, svc AnalyzerService) {
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "get_migration_context",
		Description: "Get the current migration context including active label selectors and source/target technologies. Useful for understanding what migration path is being analyzed.",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input getMigrationContextInput) (*mcpsdk.CallToolResult, any, error) {
		migCtx := svc.GetMigrationContext()
		return jsonResult(migCtx)
	})
}

// --- Helpers ---

func jsonResult(v interface{}) (*mcpsdk.CallToolResult, any, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal result: %w", err)
	}
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: string(data)},
		},
	}, nil, nil
}

func textResult(msg string) (*mcpsdk.CallToolResult, any, error) {
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: msg},
		},
	}, nil, nil
}

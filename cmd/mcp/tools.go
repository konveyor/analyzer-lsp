package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/konveyor/analyzer-lsp/engine/labels"
	konveyor "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/parser"
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
		if input.IncidentSelector != "" {
			rulesets = filterByIncidentSelector(rulesets, input.IncidentSelector)
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
		Description: "Validate Konveyor rule YAML syntax and structure. Provide either inline YAML content or a file path. Uses the full rule parser to check for structural issues like missing ruleIDs, invalid conditions, and malformed fields.",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input validateRulesInput) (*mcpsdk.CallToolResult, any, error) {
		if input.RulesContent == "" && input.RulesPath == "" {
			return nil, nil, fmt.Errorf("either rules_content or rules_path must be provided")
		}

		rulePath := input.RulesPath
		if input.RulesContent != "" {
			// Write inline content to a temp file for the parser
			tmpDir, err := os.MkdirTemp("", "konveyor-validate-*")
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create temp dir: %w", err)
			}
			defer os.RemoveAll(tmpDir)

			tmpFile := filepath.Join(tmpDir, "rules.yaml")
			if err := os.WriteFile(tmpFile, []byte(input.RulesContent), 0600); err != nil {
				return nil, nil, fmt.Errorf("failed to write temp rules file: %w", err)
			}
			rulePath = tmpFile
		}

		result := validateRulesWithParser(rulePath)
		return jsonResult(result)
	})
}

func validateRulesWithParser(rulePath string) validationResult {
	// First check basic YAML syntax
	data, err := os.ReadFile(rulePath)
	if err != nil {
		return validationResult{
			Valid:  false,
			Errors: []string{fmt.Sprintf("failed to read rules file: %v", err)},
		}
	}

	var parsed any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return validationResult{
			Valid:  false,
			Errors: []string{fmt.Sprintf("YAML parse error: %v", err)},
		}
	}

	// Use the real rule parser for deep structural validation.
	// Without providers, rules referencing providers will be skipped
	// (not errored), but structural issues will be caught.
	ruleParser := parser.RuleParser{
		ProviderNameToClient: map[string]provider.InternalProviderClient{},
		Log:                  logr.Discard(),
	}

	rulesets, _, _, parseErr := ruleParser.LoadRules(rulePath)
	if parseErr != nil {
		return validationResult{
			Valid:  false,
			Errors: []string{parseErr.Error()},
		}
	}

	ruleCount := 0
	for _, rs := range rulesets {
		ruleCount += len(rs.Rules)
	}

	return validationResult{
		Valid:   true,
		Message: fmt.Sprintf("Rule YAML is valid: %d ruleset(s), %d rule(s) loaded", len(rulesets), ruleCount),
	}
}

// --- list_rules ---

type listRulesInput struct {
	LabelSelector string `json:"label_selector,omitempty" jsonschema:"Optional label selector to filter rules"`
}

// ruleInfoLabeled adapts RuleInfo to the labels.Labeled interface.
type ruleInfoLabeled RuleInfo

func (r ruleInfoLabeled) GetLabels() []string {
	return r.Labels
}

func registerListRulesTool(server *mcpsdk.Server, svc AnalyzerService) {
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "list_rules",
		Description: "List all loaded analysis rules with their IDs, descriptions, labels, and ruleset names. Optionally filter by label selector.",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input listRulesInput) (*mcpsdk.CallToolResult, any, error) {
		rules := svc.ListRules()

		// Filter by label selector if provided
		if input.LabelSelector != "" {
			sel, err := labels.NewLabelSelector[ruleInfoLabeled](input.LabelSelector, nil)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid label selector: %w", err)
			}
			filtered := []RuleInfo{}
			for _, r := range rules {
				matched, matchErr := sel.Matches(ruleInfoLabeled(r))
				if matchErr != nil {
					continue
				}
				if matched {
					filtered = append(filtered, r)
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

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/konveyor/analyzer-lsp/core"
	"github.com/konveyor/analyzer-lsp/engine"
	konveyor "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	"gopkg.in/yaml.v2"
)

// Config holds the MCP server configuration parsed from CLI flags.
type Config struct {
	Rules              string
	ProviderConfig     string
	LabelSelector      string
	IncidentLimit      int
	CodeSnipLimit      int
	ContextLines       int
	Transport          string // "stdio" or "http"
	HTTPAddr           string // address for HTTP transport
	Verbosity          int
}

// Run starts the MCP server with the given configuration.
func Run(cfg Config) error {
	ctx := context.Background()

	// Log to stderr so stdout stays clean for stdio MCP transport.
	stdr.SetVerbosity(cfg.Verbosity)
	logger := stdr.New(log.New(os.Stderr, "", log.LstdFlags|log.Lshortfile))

	// Build the analyzer service
	svc, err := newCoreAnalyzerService(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to initialize analyzer: %w", err)
	}
	defer svc.Stop()

	server := NewMCPServer(svc)

	switch cfg.Transport {
	case "http":
		handler := mcpsdk.NewStreamableHTTPHandler(func(r *http.Request) *mcpsdk.Server {
			return server
		}, nil)
		log.Printf("MCP server listening on %s", cfg.HTTPAddr)
		return http.ListenAndServe(cfg.HTTPAddr, handler)
	default: // stdio
		return server.Run(ctx, &mcpsdk.StdioTransport{})
	}
}

// --- core.Analyzer-backed AnalyzerService ---

type coreAnalyzerService struct {
	analyzer      core.Analyzer
	cache         *IncidentsCache
	labelSelector string
	logger        logr.Logger
}

func newCoreAnalyzerService(ctx context.Context, cfg Config, logger logr.Logger) (*coreAnalyzerService, error) {
	opts := []core.AnalyzerOption{
		core.WithContext(ctx),
		core.WithLogger(logger),
	}

	if cfg.ProviderConfig != "" {
		opts = append(opts, core.WithProviderConfigFilePath(cfg.ProviderConfig))
	}
	if cfg.IncidentLimit > 0 {
		opts = append(opts, core.WithIncidentLimit(cfg.IncidentLimit))
	}
	if cfg.CodeSnipLimit > 0 {
		opts = append(opts, core.WithCodeSnipLimit(cfg.CodeSnipLimit))
	}
	if cfg.ContextLines > 0 {
		opts = append(opts, core.WithContextLinesLimit(cfg.ContextLines))
	}
	if cfg.LabelSelector != "" {
		opts = append(opts, core.WithLabelSelector(cfg.LabelSelector))
	}

	rules := strings.Split(cfg.Rules, ",")
	for i := range rules {
		rules[i] = strings.TrimSpace(rules[i])
	}
	opts = append(opts, core.WithRuleFilepaths(rules))

	analyzer, err := core.NewAnalyzer(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create analyzer: %w", err)
	}

	// Parse rules
	_, err = analyzer.ParseRules()
	if err != nil {
		return nil, fmt.Errorf("failed to parse rules: %w", err)
	}

	// Start providers
	if err := analyzer.ProviderStart(); err != nil {
		return nil, fmt.Errorf("failed to start providers: %w", err)
	}

	return &coreAnalyzerService{
		analyzer:      analyzer,
		cache:         NewIncidentsCache(logger),
		labelSelector: cfg.LabelSelector,
		logger:        logger,
	}, nil
}

func (s *coreAnalyzerService) Analyze(params AnalyzeParams) ([]konveyor.RuleSet, error) {
	// If no scopes and not resetting cache, return cached results
	if len(params.IncludedPaths) == 0 && !params.ResetCache && s.cache.Len() > 0 {
		s.logger.Info("returning cached results")
		return s.cache.ToRulesets(), nil
	}

	engineOpts := []core.EngineOption{}
	var scopes []engine.Scope
	if len(params.IncludedPaths) > 0 {
		scopes = append(scopes, engine.IncludedPathsScope(params.IncludedPaths, s.logger))
	}
	if len(params.ExcludedPaths) > 0 {
		scopes = append(scopes, engine.ExcludedPathsScope(params.ExcludedPaths, s.logger))
	}
	if len(scopes) > 0 {
		engineOpts = append(engineOpts, core.WithScope(engine.NewScope(scopes...)))
	}

	// Run analysis
	rulesets := s.analyzer.Run(engineOpts...)

	// Update cache
	if len(params.IncludedPaths) == 0 {
		s.cache.SetFromRulesets(rulesets)
	} else {
		s.cache.UpdateFromRulesets(rulesets, params.IncludedPaths)
	}

	return s.cache.ToRulesets(), nil
}

func (s *coreAnalyzerService) GetCachedResults() []konveyor.RuleSet {
	return s.cache.ToRulesets()
}

func (s *coreAnalyzerService) NotifyFileChanges(changes []provider.FileChange) error {
	ctx := context.Background()
	providers := s.analyzer.GetProviders()
	var errs []error
	for _, p := range providers {
		s.logger.Info("notifying provider of file changes", "provider", p.Name, "changes", len(changes))
		if err := p.NotifyFileChanges(ctx, changes...); err != nil {
			s.logger.Error(err, "failed to notify provider", "provider", p.Name)
			errs = append(errs, fmt.Errorf("provider %s: %w", p.Name, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to notify some providers: %v", errs)
	}
	return nil
}

func (s *coreAnalyzerService) ListProviders() []ProviderInfo {
	providers := s.analyzer.GetProviders()
	result := make([]ProviderInfo, 0, len(providers))
	for _, p := range providers {
		caps := []string{}
		for _, c := range p.Capabilities() {
			caps = append(caps, c.Name)
		}
		result = append(result, ProviderInfo{
			Name:         p.Name,
			Capabilities: caps,
		})
	}
	return result
}

func (s *coreAnalyzerService) GetDependencies() ([]konveyor.DepsFlatItem, error) {
	// core.Analyzer.GetDependencies writes to a file. We'll use a temp file
	// and read it back, or we can return empty for now and enhance later.
	tmpFile, err := os.CreateTemp("", "konveyor-deps-*.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	if err := s.analyzer.GetDependencies(tmpPath, false); err != nil {
		return nil, err
	}

	// Read the deps file
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read deps file: %w", err)
	}

	if len(data) == 0 {
		return []konveyor.DepsFlatItem{}, nil
	}

	// The output is YAML
	var deps []konveyor.DepsFlatItem
	if err := yamlUnmarshal(data, &deps); err != nil {
		return nil, fmt.Errorf("failed to parse deps: %w", err)
	}
	return deps, nil
}

func (s *coreAnalyzerService) ListRules() []RuleInfo {
	ruleSets := s.analyzer.RuleSets()
	var rules []RuleInfo
	for _, rs := range ruleSets {
		for _, r := range rs.Rules {
			rules = append(rules, RuleInfo{
				ID:          r.RuleID,
				Description: r.Description,
				Labels:      r.Labels,
				RuleSetName: rs.Name,
			})
		}
	}
	return rules
}

func (s *coreAnalyzerService) GetMigrationContext() MigrationContext {
	mc := MigrationContext{
		LabelSelector: s.labelSelector,
	}

	// Infer sources/targets from rule labels
	labels := s.analyzer.RuleLabels()
	for _, label := range labels {
		if source, ok := strings.CutPrefix(label, konveyor.SourceTechnologyLabel+"="); ok {
			mc.Sources = append(mc.Sources, source)
		}
		if target, ok := strings.CutPrefix(label, konveyor.TargetTechnologyLabel+"="); ok {
			mc.Targets = append(mc.Targets, target)
		}
	}
	return mc
}

func (s *coreAnalyzerService) Stop() error {
	return s.analyzer.Stop()
}

// yamlUnmarshal wraps yaml.Unmarshal.
func yamlUnmarshal(data []byte, v any) error {
	return yaml.Unmarshal(data, v)
}

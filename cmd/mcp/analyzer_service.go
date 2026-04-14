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
	"github.com/konveyor/analyzer-lsp/engine/labels"
	konveyor "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
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
	OAuthToken         string // Bearer token for HTTP transport auth
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
		mcpHandler := mcpsdk.NewStreamableHTTPHandler(func(r *http.Request) *mcpsdk.Server {
			return server
		}, nil)
		var handler http.Handler = mcpHandler
		if cfg.OAuthToken != "" {
			handler = bearerAuthMiddleware(cfg.OAuthToken, mcpHandler)
			log.Printf("MCP server listening on %s (auth enabled)", cfg.HTTPAddr)
		} else {
			log.Printf("MCP server listening on %s (no auth)", cfg.HTTPAddr)
		}
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
	ctx := context.Background()
	providers := s.analyzer.GetProviders(core.FilterByCapability("dependency"))
	if len(providers) == 0 {
		return []konveyor.DepsFlatItem{}, nil
	}

	var depsFlat []konveyor.DepsFlatItem
	var errs []error
	for _, prov := range providers {
		deps, err := prov.GetDependencies(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("provider %s: %w", prov.Name, err))
			continue
		}
		for u, ds := range deps {
			depsFlat = append(depsFlat, konveyor.DepsFlatItem{
				Provider:     prov.Name,
				FileURI:      string(u),
				Dependencies: ds,
			})
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("failed to get dependencies: %v", errs)
	}
	return depsFlat, nil
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

// bearerAuthMiddleware validates OAuth 2.1 Bearer tokens on HTTP transport.
func bearerAuthMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != token {
			w.Header().Set("WWW-Authenticate", `Bearer`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// incidentVariables adapts an incident's Variables map to the labels.Labeled interface
// so it can be matched with a label selector. This mirrors engine/internal.VariableLabelSelector.
type incidentVariables map[string]interface{}

func (v incidentVariables) GetLabels() []string {
	if len(v) == 0 {
		return []string{""}
	}
	s := make([]string, 0, len(v))
	for k, val := range v {
		s = append(s, fmt.Sprintf("%s=%s", k, val))
	}
	return s
}

func matchVariables(elem string, items []string) bool {
	for _, i := range items {
		if strings.Contains(elem, ".") {
			if strings.Contains(i, fmt.Sprintf("%v.", elem)) {
				return true
			}
		}
		if i == elem {
			return true
		}
	}
	return false
}

// filterByIncidentSelector applies an incident selector expression to filter
// incidents in the results, matching the behavior of engine's incident selector.
func filterByIncidentSelector(rulesets []konveyor.RuleSet, selector string) []konveyor.RuleSet {
	sel, err := labels.NewLabelSelector[incidentVariables](selector, matchVariables)
	if err != nil {
		return rulesets
	}

	filtered := make([]konveyor.RuleSet, 0, len(rulesets))
	for _, rs := range rulesets {
		newRS := konveyor.RuleSet{
			Name:        rs.Name,
			Description: rs.Description,
			Tags:        rs.Tags,
		}
		violations := map[string]konveyor.Violation{}
		for ruleID, v := range rs.Violations {
			var incidents []konveyor.Incident
			for _, inc := range v.Incidents {
				vars := incidentVariables(inc.Variables)
				matched, matchErr := sel.Matches(vars)
				if matchErr != nil || matched {
					incidents = append(incidents, inc)
				}
			}
			if len(incidents) > 0 {
				v.Incidents = incidents
				violations[ruleID] = v
			}
		}
		if len(violations) > 0 {
			newRS.Violations = violations
			filtered = append(filtered, newRS)
		}
	}
	return filtered
}

package main

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	konveyor "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
)

// AnalyzerService defines the interface that the MCP tool handlers use
// to interact with the analysis engine. This decouples tool handlers
// from the concrete core.Analyzer implementation, enabling testing
// with mocks.
type AnalyzerService interface {
	// Analyze runs analysis with the given parameters.
	// If includedPaths is non-empty and resetCache is false, returns cached results.
	// Otherwise runs the engine and updates the cache.
	Analyze(params AnalyzeParams) ([]konveyor.RuleSet, error)

	// GetCachedResults returns the current cached analysis results
	// without running a new analysis.
	GetCachedResults() []konveyor.RuleSet

	// NotifyFileChanges notifies all providers of file changes
	// for incremental analysis support.
	NotifyFileChanges(changes []provider.FileChange) error

	// ListProviders returns information about available providers.
	ListProviders() []ProviderInfo

	// GetDependencies returns the project dependencies.
	GetDependencies() ([]konveyor.DepsFlatItem, error)

	// ListRules returns metadata about loaded rules.
	ListRules() []RuleInfo

	// GetMigrationContext returns the current migration context
	// (active label selectors, source/target info).
	GetMigrationContext() MigrationContext

	// Stop cleans up all resources.
	Stop() error
}

// AnalyzeParams are the parameters for an analysis run.
type AnalyzeParams struct {
	LabelSelector    string   `json:"label_selector,omitempty"`
	IncidentSelector string   `json:"incident_selector,omitempty"`
	IncludedPaths    []string `json:"included_paths,omitempty"`
	ExcludedPaths    []string `json:"excluded_paths,omitempty"`
	ResetCache       bool     `json:"reset_cache,omitempty"`
}

// ProviderInfo describes an available analysis provider.
type ProviderInfo struct {
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities"`
}

// RuleInfo describes a loaded rule.
type RuleInfo struct {
	ID          string   `json:"id"`
	Description string   `json:"description,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	RuleSetName string   `json:"ruleset_name,omitempty"`
}

// MigrationContext describes the current migration configuration.
type MigrationContext struct {
	LabelSelector string   `json:"label_selector,omitempty"`
	Sources       []string `json:"sources,omitempty"`
	Targets       []string `json:"targets,omitempty"`
}

// --- Incidents Cache ---
// Adapted from kai-analyzer-rpc/pkg/service/cache.go

// CacheValue stores a single incident with its parent violation and ruleset context.
type CacheValue struct {
	Incident      konveyor.Incident
	ViolationName string
	Violation     konveyor.Violation
	Ruleset       konveyor.RuleSet
}

// IncidentsCache is a thread-safe cache keyed by file path.
type IncidentsCache struct {
	cache  map[string][]CacheValue
	logger logr.Logger
	mu     sync.RWMutex
}

func NewIncidentsCache(logger logr.Logger) *IncidentsCache {
	return &IncidentsCache{
		cache:  make(map[string][]CacheValue),
		logger: logger,
	}
}

func (c *IncidentsCache) Add(path string, value CacheValue) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := normalizePath(path)
	c.cache[key] = append(c.cache[key], value)
}

func (c *IncidentsCache) Delete(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, normalizePath(path))
}

func (c *IncidentsCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}

func (c *IncidentsCache) Entries() map[string][]CacheValue {
	c.mu.RLock()
	defer c.mu.RUnlock()
	clone := make(map[string][]CacheValue, len(c.cache))
	for k, v := range c.cache {
		cloned := make([]CacheValue, len(v))
		copy(cloned, v)
		clone[k] = cloned
	}
	return clone
}

// SetFromRulesets replaces the entire cache from a full analysis run.
func (c *IncidentsCache) SetFromRulesets(rulesets []konveyor.RuleSet) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string][]CacheValue)
	c.addRulesetsLocked(rulesets)
}

// UpdateFromRulesets invalidates cache for the given paths, then adds new results.
func (c *IncidentsCache) UpdateFromRulesets(rulesets []konveyor.RuleSet, paths []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range paths {
		delete(c.cache, normalizePath(p))
	}
	c.addRulesetsLocked(rulesets)
}

func (c *IncidentsCache) addRulesetsLocked(rulesets []konveyor.RuleSet) {
	for _, r := range rulesets {
		for violationName, v := range r.Violations {
			for _, i := range v.Incidents {
				key := normalizePath(i.URI.Filename())
				c.cache[key] = append(c.cache[key], CacheValue{
					Incident:      i,
					ViolationName: violationName,
					Violation: konveyor.Violation{
						Description: v.Description,
						Category:    v.Category,
						Labels:      v.Labels,
					},
					Ruleset: konveyor.RuleSet{
						Name:        r.Name,
						Description: r.Description,
						Tags:        r.Tags,
					},
				})
			}
		}
	}
}

// ToRulesets reconstructs RuleSet slice from the cache.
func (c *IncidentsCache) ToRulesets() []konveyor.RuleSet {
	c.mu.RLock()
	defer c.mu.RUnlock()

	ruleSetMap := map[string]konveyor.RuleSet{}
	for _, cacheValues := range c.cache {
		for _, cv := range cacheValues {
			rs, ok := ruleSetMap[cv.Ruleset.Name]
			if !ok {
				rs = cv.Ruleset
				rs.Violations = map[string]konveyor.Violation{}
			}
			if vio, ok := rs.Violations[cv.ViolationName]; ok {
				vio.Incidents = append(vio.Incidents, cv.Incident)
				rs.Violations[cv.ViolationName] = vio
			} else {
				vio := cv.Violation
				vio.Incidents = []konveyor.Incident{cv.Incident}
				rs.Violations[cv.ViolationName] = vio
			}
			ruleSetMap[cv.Ruleset.Name] = rs
		}
	}

	result := make([]konveyor.RuleSet, 0, len(ruleSetMap))
	for _, rs := range ruleSetMap {
		result = append(result, rs)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

func normalizePath(path string) string {
	cleaned := filepath.Clean(path)
	vol := filepath.VolumeName(cleaned)
	if vol != "" {
		cleaned = strings.ToUpper(vol) + cleaned[len(vol):]
	}
	return filepath.ToSlash(cleaned)
}

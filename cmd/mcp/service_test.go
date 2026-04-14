package main

import (
	"testing"

	"github.com/go-logr/logr"
	konveyor "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lsp.dev/uri"
)

func TestIncidentsCache_SetFromRulesets(t *testing.T) {
	cache := NewIncidentsCache(logr.Discard())

	effort := 3
	rulesets := []konveyor.RuleSet{
		{
			Name:        "test-ruleset",
			Description: "Test ruleset",
			Violations: map[string]konveyor.Violation{
				"rule-001": {
					Description: "Use of deprecated API",
					Effort:      &effort,
					Incidents: []konveyor.Incident{
						{
							URI:        uri.URI("file:///src/main.java"),
							Message:    "Deprecated API call",
							LineNumber: intPtr(42),
						},
						{
							URI:        uri.URI("file:///src/util.java"),
							Message:    "Another deprecated call",
							LineNumber: intPtr(10),
						},
					},
				},
			},
		},
	}

	cache.SetFromRulesets(rulesets)

	assert.Equal(t, 2, cache.Len(), "cache should have 2 file entries")

	entries := cache.Entries()
	assert.Contains(t, entries, "/src/main.java")
	assert.Contains(t, entries, "/src/util.java")
}

func TestIncidentsCache_ToRulesets(t *testing.T) {
	cache := NewIncidentsCache(logr.Discard())

	effort := 3
	original := []konveyor.RuleSet{
		{
			Name: "test-ruleset",
			Violations: map[string]konveyor.Violation{
				"rule-001": {
					Description: "Deprecated API",
					Effort:      &effort,
					Incidents: []konveyor.Incident{
						{
							URI:        uri.URI("file:///src/main.java"),
							Message:    "Found deprecated call",
							LineNumber: intPtr(42),
						},
					},
				},
			},
		},
	}

	cache.SetFromRulesets(original)
	reconstructed := cache.ToRulesets()

	require.Len(t, reconstructed, 1)
	assert.Equal(t, "test-ruleset", reconstructed[0].Name)
	require.Contains(t, reconstructed[0].Violations, "rule-001")
	assert.Len(t, reconstructed[0].Violations["rule-001"].Incidents, 1)
	assert.Equal(t, "Found deprecated call", reconstructed[0].Violations["rule-001"].Incidents[0].Message)
}

func TestIncidentsCache_UpdateFromRulesets(t *testing.T) {
	cache := NewIncidentsCache(logr.Discard())

	// Initial full analysis
	cache.SetFromRulesets([]konveyor.RuleSet{
		{
			Name: "rs",
			Violations: map[string]konveyor.Violation{
				"rule-001": {
					Description: "deprecated",
					Incidents: []konveyor.Incident{
						{URI: uri.URI("file:///src/a.java"), Message: "old-a", LineNumber: intPtr(1)},
						{URI: uri.URI("file:///src/b.java"), Message: "old-b", LineNumber: intPtr(2)},
					},
				},
			},
		},
	})

	assert.Equal(t, 2, cache.Len())

	// Partial update: file a.java was changed, re-analyzed, violation moved
	cache.UpdateFromRulesets([]konveyor.RuleSet{
		{
			Name: "rs",
			Violations: map[string]konveyor.Violation{
				"rule-001": {
					Description: "deprecated",
					Incidents: []konveyor.Incident{
						{URI: uri.URI("file:///src/a.java"), Message: "new-a", LineNumber: intPtr(50)},
					},
				},
			},
		},
	}, []string{"/src/a.java"})

	// Cache should still have 2 entries
	assert.Equal(t, 2, cache.Len())

	rulesets := cache.ToRulesets()
	require.Len(t, rulesets, 1)

	incidents := rulesets[0].Violations["rule-001"].Incidents
	// Should have 2 incidents: updated a.java + unchanged b.java
	assert.Len(t, incidents, 2)

	messages := map[string]bool{}
	for _, inc := range incidents {
		messages[inc.Message] = true
	}
	assert.True(t, messages["new-a"], "should have updated incident for a.java")
	assert.True(t, messages["old-b"], "should still have original incident for b.java")
	assert.False(t, messages["old-a"], "should NOT have old incident for a.java")
}

func TestIncidentsCache_EmptyCache(t *testing.T) {
	cache := NewIncidentsCache(logr.Discard())

	assert.Equal(t, 0, cache.Len())
	assert.Empty(t, cache.Entries())
	assert.Empty(t, cache.ToRulesets())
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/src/main.java", "/src/main.java"},
		{"/src/../src/main.java", "/src/main.java"},
		{"/src/./main.java", "/src/main.java"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, normalizePath(tt.input))
		})
	}
}

func intPtr(i int) *int {
	return &i
}

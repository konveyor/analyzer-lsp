package parser_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/bombsimon/logrusr/v3"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	ruleparser "github.com/konveyor/analyzer-lsp/parser"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/sirupsen/logrus"
)

func TestRuleExtrasPassthrough(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.Level(5))
	logger := logrusr.New(log)

	parser := ruleparser.RuleParser{
		ProviderNameToClient: map[string]provider.InternalProviderClient{
			"builtin": testProvider{
				caps: []provider.Capability{{
					Name: "file",
				}},
			},
		},
		Log: logger,
	}

	// Load the test rule with extras
	testFile := filepath.Join("testdata", "rule-with-extras.yaml")
	rules, _, _, err := parser.LoadRule(testFile)
	if err != nil {
		t.Fatalf("Failed to load rule: %v", err)
	}

	if len(rules) != 1 {
		t.Fatalf("Expected 1 rule, got %d", len(rules))
	}

	rule := rules[0]

	// Verify basic fields are still loaded correctly
	if rule.RuleID != "javax-to-jakarta-00001" {
		t.Errorf("Expected ruleID 'javax-to-jakarta-00001', got '%s'", rule.RuleID)
	}

	if rule.Effort == nil || *rule.Effort != 1 {
		t.Errorf("Expected effort 1, got %v", rule.Effort)
	}

	// Verify extras field is populated
	if rule.Extras == nil {
		t.Fatal("Expected Extras to be populated, got nil")
	}

	// Check migration_complexity
	if complexity, ok := rule.Extras["migration_complexity"].(string); ok {
		if complexity != "trivial" {
			t.Errorf("Expected migration_complexity 'trivial', got '%s'", complexity)
		}
	} else {
		t.Error("Expected migration_complexity in Extras")
	}

	// Check domain
	if domain, ok := rule.Extras["domain"].(string); ok {
		if domain != "jakarta-migration" {
			t.Errorf("Expected domain 'jakarta-migration', got '%s'", domain)
		}
	} else {
		t.Error("Expected domain in Extras")
	}

	// Check metadata
	if metadata, ok := rule.Extras["metadata"].(map[interface{}]interface{}); ok {
		if severity, ok := metadata["severity"].(string); ok {
			if severity != "high" {
				t.Errorf("Expected metadata.severity 'high', got '%s'", severity)
			}
		} else {
			t.Error("Expected metadata.severity in Extras")
		}
	} else {
		t.Error("Expected metadata in Extras")
	}

	t.Log("Extras field contents:", rule.Extras)
}

func TestViolationExtrasJSON(t *testing.T) {
	// Test that extras are properly marshaled to JSON in a violation
	extras := map[string]interface{}{
		"migration_complexity": "trivial",
		"domain":               "jakarta-migration",
		"metadata": map[string]interface{}{
			"severity":      "high",
			"confidence":    "high",
			"migrationPath": "automated",
		},
	}

	extrasJSON, err := json.Marshal(extras)
	if err != nil {
		t.Fatalf("Failed to marshal extras: %v", err)
	}

	violation := konveyor.Violation{
		Description: "Test violation",
		Extras:      extrasJSON,
	}

	// Verify we can unmarshal the extras from the violation
	var unmarshaled map[string]interface{}
	if err := json.Unmarshal(violation.Extras, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal violation extras: %v", err)
	}

	if complexity, ok := unmarshaled["migration_complexity"].(string); ok {
		if complexity != "trivial" {
			t.Errorf("Expected migration_complexity 'trivial', got '%s'", complexity)
		}
	} else {
		t.Error("Expected migration_complexity in unmarshaled extras")
	}

	t.Log("Successfully marshaled and unmarshaled extras in violation")
}

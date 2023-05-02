package hubapi

import (
	"encoding/json"

	"go.lsp.dev/uri"
)

type RuleSet struct {
	Name        string               `yaml:"name,omitempty"`
	Description string               `yaml:"description,omitempty"`
	Source      *RuleSetTechnology   `yaml:"source,omitempty"`
	Target      *RuleSetTechnology   `yaml:"target,omitempty"`
	Labels      []string             `yaml:"labels,omitempty"`
	Tags        []string             `yaml:"tags,omitempty"`
	Violations  map[string]Violation `yaml:"violations,omitempty"`
}

type RuleSetTechnology struct {
	ID           string `json:"id,omitempty"`
	VersionRange string `json:"version_range,omitempty"`
}

type Category string

var (
	// Potential - rule states that there may be issue, but the rule author is unsure.
	// This is used when you are trying to communicate that something may be wrong/incorrect
	// But individual situations may require more context.
	Potential Category = "potential"
	// Optional - rule states that there is an issue, but that this issue can be fixed later.
	// Primary use case is when migrating frameworks, and something has a deprecated notice,
	// You should fix this but it wont break the migration/upgrade.
	Optional Category = "optional"
	// Mandatory - rule states that there is an issue that must be fixed.
	// This is used, based on the ruleset, to tell the user that this is a real issue.
	// For migrations, this means it must be fixed or the upgrade with fail.
	Mandatory Category = "mandatory"
)

type Violation struct {
	// AnalysisID id of the analysis that generated this output
	// TODO: we don't know exactly what this looks like yet but that is ok.
	//AnalysisID     string     `json:"analysisID"`

	// Description text description about the violation
	// TODO: we don't have this in the rule as of today.
	Description string `yaml:"description"`

	// Category category of the violation
	// TODO: add this to rules
	Category *Category `yaml:"category,omitempty"`

	Labels []string `yaml:"labels,omitempty"`

	// Tags list of tags generated for the applications
	Tags []string `yaml:"tags,omitempty"`

	// Incidents list of instances of violation found
	Incidents []Incident `yaml:"incidents"`

	// ExternalLinks hyperlinks to external sources of docs, fixes
	Links []Link `yaml:"links,omitempty"`

	// Extras reserved for additional data
	Extras json.RawMessage

	// Effort defines expected story points for this incident
	Effort *int `yaml:"effort,omitempty"`
}

// Incident defines instance of a violation
type Incident struct {
	// URI defines location in the codebase where violation is found
	URI uri.URI `yaml:"uri"`
	// Message text description about the incident
	Message  string `yaml:"message"`
	CodeSnip string `yaml:"codeSnip,omitempty"`
	// Extras reserved for additional data
	//Extras json.RawMessage
	Variables map[string]interface{} `yaml:"variables,omitempty"`
}

// Link defines an external hyperlink
type Link struct {
	URL string `json:"url"`
	// Title optional description
	Title string `json:"title,omitempty"`
}

package hubapi

import (
	"encoding/json"
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
	Potential   Category = "potential"
	Information Category = "information"
	Mandatory   Category = "mandatory"
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
	URI string `yaml:"uri"`
	// Message text description about the incident
	Message string `yaml:"message"`
	// Extras reserved for additional data
	//Extras json.RawMessage
	Extras map[string]interface{}
}

// Link defines an external hyperlink
type Link struct {
	URL string `json:"url"`
	// Title optional description
	Title string `json:"title,omitempty"`
}

package hubapi

import "encoding/json"

type Violation struct {
	// AnalysisID id of the analysis that generated this output
	// TODO: we don't know exactly what this looks like yet but that is ok.
	//AnalysisID     string     `json:"analysisID"`

	// RuleID id of the rule for this violation
	// TODO: we need to add this to the rule format.
	RuleID string `yaml:"ruleID"`

	// Description text description about the violation
	// TODO: we don't have this in the rule as of today.
	Description string `yaml:"description"`

	// Category category of the violation
	// TODO: add this to rules
	Category string `yaml:"category",omitempty"`

	// Incidents list of instances of violation found
	Incidents []Incident `yaml:"incidents"`

	// Extras reserved for additional data
	Extras json.RawMessage
}

// Incident defines instance of a violation
type Incident struct {
	// URI defines location in the codebase where violation is found
	URI string `yaml:"uri"`
	// Effort defines expected story points for this incident
	Effort int `yaml:"effort,omitempty"`
	// Message text description about the incident
	Message string `yaml:"message"`
	// ExternalLinks hyperlinks to external sources of docs, fixes
	ExternalLinks []Link `json:"externalLinks"`
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

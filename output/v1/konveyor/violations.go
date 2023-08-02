package konveyor

import (
	"encoding/json"

	"go.lsp.dev/uri"
)

const (
	SourceTechnologyLabel = "konveyor.io/source"
	TargetTechnologyLabel = "konveyor.io/target"
)

type RuleSet struct {
	// Name is a name for the ruleset.
	Name string `yaml:"name,omitempty" json:"name,omitempty"`
	// Description text description for the ruleset.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	// Tags list of generated tags from the rules in this ruleset.
	Tags []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	// Violations is a map containing violations generated for the
	// matched rules in this ruleset. Keys are rule IDs, values are
	// their respective generated violations.
	Violations map[string]Violation `yaml:"violations,omitempty" json:"violations,omitempty"`
	// Errors is a map containing errors generated during evaluation
	// of rules in this ruleset. Keys are rule IDs, values are
	// their respective generated errors.
	Errors map[string]string `yaml:"errors,omitempty" json:"errors,omitempty"`
	// Unmatched is a list of rule IDs of the rules that weren't matched.
	Unmatched []string `yaml:"unmatched,omitempty" json:"unmatched,omitempty"`
	// Skipped is a list of rule IDs that were skipped
	Skipped []string `yaml:"skipped,omitempty" json:"skipped,omitempty"`
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
	// Description text description about the violation
	// TODO: we don't have this in the rule as of today.
	Description string `yaml:"description" json:"description"`

	// Category category of the violation
	// TODO: add this to rules
	Category *Category `yaml:"category,omitempty" json:"category,omitempty"`

	Labels []string `yaml:"labels,omitempty" json:"labels,omitempty"`

	// Incidents list of instances of violation found
	Incidents []Incident `yaml:"incidents" json:"incidents"`

	// ExternalLinks hyperlinks to external sources of docs, fixes
	Links []Link `yaml:"links,omitempty" json:"links,omitempty"`

	// Extras reserved for additional data
	Extras json.RawMessage

	// Effort defines expected story points for this incident
	Effort *int `yaml:"effort,omitempty" json:"effort,omitempty"`
}

// Incident defines instance of a violation
type Incident struct {
	// URI defines location in the codebase where violation is found
	URI uri.URI `yaml:"uri" json:"uri"`
	// Message text description about the incident
	Message  string `yaml:"message" json:"message"`
	CodeSnip string `yaml:"codeSnip,omitempty" json:"codeSnip,omitempty"`
	// Extras reserved for additional data
	//Extras json.RawMessage
	LineNumber *int                   `yaml:"lineNumber,omitempty" json:"lineNumber,omitempty"`
	Variables  map[string]interface{} `yaml:"variables,omitempty" json:"variables,omitempty"`
}

// Link defines an external hyperlink
type Link struct {
	URL string `yaml:"url" json:"url"`
	// Title optional description
	Title string `yaml:"title,omitempty" json:"title,omitempty"`
}

type Dep struct {
	Name               string                 `json:"name,omitempty" yaml:"name,omitempty"`
	Version            string                 `json:"version,omitempty" yaml:"version,omitempty"`
	Type               string                 `json:"type,omitempty" yaml:"type,omitempty"`
	Indirect           bool                   `json:"indirect,omitempty" yaml:"indirect,omitempty"`
	ResolvedIdentifier string                 `json:"resolvedIdentifier,omitempty" yaml:"resolvedIdentifier,omitempty"`
	Extras             map[string]interface{} `json:"extras,omitempty" yaml:"extras,omitempty"`
	Labels             []string               `json:"labels,omitempty" yaml:"labels,omitempty"`
	FileURIPrefix      string                 `json:"prefix,omitempty" yaml:"prefix,omitempty"`
}

func (d *Dep) GetLabels() []string {
	return d.Labels
}

type DepDAGItem struct {
	Dep       Dep          `yaml:"dep,omitempty" json:"dep,omitempty"`
	AddedDeps []DepDAGItem `yaml:"addedDep,omitempty" json:"addedDep,omitempty"`
}

type DepsFlatItem struct {
	FileURI      string `yaml:"fileURI" json:"fileURI"`
	Provider     string `yaml:"provider" json:"provider"`
	Dependencies []*Dep `yaml:"dependencies" json:"dependencies"`
}

type DepsTreeItem struct {
	FileURI      string       `yaml:"fileURI" json:"fileURI"`
	Provider     string       `yaml:"provider" json:"provider"`
	Dependencies []DepDAGItem `yaml:"dependencies" json:"dependencies"`
}

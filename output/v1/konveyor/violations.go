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
	Name string `yaml:"name,omitempty"`
	// Description text description for the ruleset.
	Description string `yaml:"description,omitempty"`
	// Tags list of generated tags from the rules in this ruleset.
	Tags []string `yaml:"tags,omitempty"`
	// Violations is a map containing violations generated for the
	// matched rules in this ruleset. Keys are rule IDs, values are
	// their respective generated violations.
	Violations map[string]Violation `yaml:"violations,omitempty"`
	// Errors is a map containing errors generated during evaluation
	// of rules in this ruleset. Keys are rule IDs, values are
	// their respective generated errors.
	Errors map[string]string `yaml:"errors,omitempty"`
	// Unmatched is a list of rule IDs of the rules that weren't matched.
	Unmatched []string `yaml:"unmatched,omitempty"`
	// Skipped is a list of rule IDs that were skipped
	Skipped []string `yaml:"skipped,omitempty"`
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
	Description string `yaml:"description"`

	// Category category of the violation
	// TODO: add this to rules
	Category *Category `yaml:"category,omitempty"`

	Labels []string `yaml:"labels,omitempty"`

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
	LineNumber *int                   `yaml:"lineNumber,omitempty"`
	Variables  map[string]interface{} `yaml:"variables,omitempty"`
}

// Link defines an external hyperlink
type Link struct {
	URL string `yaml:"url"`
	// Title optional description
	Title string `yaml:"title,omitempty"`
}

type Dep struct {
	Name               string                 `json:"name,omitempty" yaml:"name,omitempty"`
	Version            string                 `json:"version,omitempty" yaml:"version,omitempty"`
	Type               string                 `json:"type,omitempty" yaml:"type,omitempty"`
	Indirect           bool                   `json:"indirect,omitempty" yaml:"indirect,omitempty"`
	ResolvedIdentifier string                 `json:"sha,omitempty" yaml:"sha,omitempty"`
	Extras             map[string]interface{} `json:"extras,omitempty" yaml:"extras,omitempty"`
	Labels             []string               `json:"labels,omitempty" yaml:"labels,omitempty"`
}

func (d *Dep) GetLabels() []string {
	return d.Labels
}

type DepDAGItem struct {
	Dep       Dep          `json:"dep,omitempty"`
	AddedDeps []DepDAGItem `json:"addedDep,omitempty"`
}

type DepsFlatItem struct {
	FileURI      string `yaml:"fileURI"`
	Provider     string `yaml:"provider"`
	Dependencies []*Dep `yaml:"dependencies"`
}

type DepsTreeItem struct {
	FileURI      string       `yaml:"fileURI"`
	Provider     string       `yaml:"provider"`
	Dependencies []DepDAGItem `yaml:"dependencies"`
}

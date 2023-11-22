package konveyor

import (
	"encoding/json"
	"sort"
	"strings"

	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
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

// Sorts all fields in a canonical way on a RuleSet
func (r *RuleSet) sortFields() {
	sort.Strings(r.Tags)
	sort.Strings(r.Unmatched)
	sort.Strings(r.Skipped)
}

func (r RuleSet) MarshalYAML() (interface{}, error) {
	r.sortFields()
	return r, nil
}

// NOTE(jsussman): Might have some performance issues.
//
// MarshalYAML's return value is (interface{}, error), meaning it simply
// propagates the object forward, unmarshalling whatever you return.
// MarshalJSON's return value is ([]byte, error), meaning you have to build the
// JSON yourself. You'd think we can simply call r.sortFields and then
// json.Marshal(r) like MarshalYAML, but that causes infinite recursion.
func (r RuleSet) MarshalJSON() ([]byte, error) {
	b, err := yaml.Marshal(r)
	if err != nil {
		return b, err
	}

	m := map[string]any{}
	err = yaml.Unmarshal(b, &m)
	if err != nil {
		return nil, err
	}

	return json.Marshal(m)
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
	Extras json.RawMessage `yaml:"extras,omitempty" json:"extras,omitempty"`

	// Effort defines expected story points for this incident
	Effort *int `yaml:"effort,omitempty" json:"effort,omitempty"`
}

// Sorts all fields in a canonical way on a Violation
func (v *Violation) sortFields() {
	sort.Strings(v.Labels)

	sort.SliceStable(v.Incidents, func(i, j int) bool {
		return v.Incidents[i].cmpLess(&v.Incidents[j])
	})

	sort.SliceStable(v.Links, func(i, j int) bool {
		return v.Links[i].cmpLess(&v.Links[j])
	})
}

func (v Violation) MarshalYAML() (interface{}, error) {
	v.sortFields()
	return v, nil
}

func (v Violation) MarshalJSON() ([]byte, error) {
	b, err := yaml.Marshal(v)
	if err != nil {
		return b, err
	}

	m := map[string]any{}
	err = yaml.Unmarshal(b, &m)
	if err != nil {
		return nil, err
	}

	return json.Marshal(m)
}

// Incident defines instance of a violation
type Incident struct {
	// URI defines location in the codebase where violation is found
	URI uri.URI `yaml:"uri" json:"uri"`

	// Message text description about the incident
	Message  string `yaml:"message" json:"message"`
	CodeSnip string `yaml:"codeSnip,omitempty" json:"codeSnip,omitempty"`

	// Extras reserved for additional data
	// Extras json.RawMessage
	LineNumber *int                   `yaml:"lineNumber,omitempty" json:"lineNumber,omitempty"`
	Variables  map[string]interface{} `yaml:"variables,omitempty" json:"variables,omitempty"`
}

// Lexicographically compares two Incidents
func (i *Incident) cmpLess(other *Incident) bool {
	if i.URI != other.URI {
		return i.URI < other.URI
	}

	if i.Message != other.Message {
		return i.Message < other.Message
	}

	if i.CodeSnip != other.CodeSnip {
		return i.CodeSnip < other.CodeSnip
	}

	if i.LineNumber == nil {
		x := 0
		i.LineNumber = &x
	}

	if other.LineNumber == nil {
		x := 0
		other.LineNumber = &x
	}

	if *(*i).LineNumber != *(*other).LineNumber {
		return *(*i).LineNumber < *(*other).LineNumber
	}

	return false
}

// Link defines an external hyperlink
type Link struct {
	URL string `yaml:"url" json:"url"`

	// Title optional description
	Title string `yaml:"title,omitempty" json:"title,omitempty"`
}

// Lexicographically compares two Links
func (l *Link) cmpLess(other *Link) bool {
	if l.Title != other.Title {
		return l.Title < other.Title
	}

	if l.URL != other.URL {
		return l.URL < other.URL
	}

	return false
}

type Dep struct {
	Name       string `json:"name,omitempty" yaml:"name,omitempty"`
	Version    string `json:"version,omitempty" yaml:"version,omitempty"`
	Classifier string `json:"classifier,omitempty" yaml:"classifier,omitempty"`
	// TODO The so-called "type" is the "scope" in Maven speak
	Type               string                 `json:"type,omitempty" yaml:"type,omitempty"`
	Indirect           bool                   `json:"indirect,omitempty" yaml:"indirect,omitempty"`
	ResolvedIdentifier string                 `json:"resolvedIdentifier,omitempty" yaml:"resolvedIdentifier,omitempty"`
	Extras             map[string]interface{} `json:"extras,omitempty" yaml:"extras,omitempty"`
	Labels             []string               `json:"labels,omitempty" yaml:"labels,omitempty"`
	FileURIPrefix      string                 `json:"prefix,omitempty" yaml:"prefix,omitempty"`
}

// Sorts all fields in a canonical way on a Dep
func (d *Dep) sortFields() {
	sort.Strings(d.Labels)
}

// Lexicographically compares two Deps
func (d Dep) cmpLess(other Dep) bool {
	if d.Name != other.Name {
		return d.Name < other.Name
	}

	if d.Version != other.Version {
		return d.Version < other.Version
	}

	if d.Type != other.Type {
		return d.Type < other.Type
	}

	if d.Indirect != other.Indirect {
		return !d.Indirect && other.Indirect
	}

	if d.ResolvedIdentifier != other.ResolvedIdentifier {
		return d.ResolvedIdentifier < other.ResolvedIdentifier
	}

	dLabels := strings.Join(d.Labels, ",")
	otherLabels := strings.Join(other.Labels, ",")
	if dLabels != otherLabels {
		return dLabels < otherLabels
	}

	if d.FileURIPrefix != other.FileURIPrefix {
		return d.FileURIPrefix < other.FileURIPrefix
	}

	return false
}

func (d Dep) MarshalYAML() (interface{}, error) {
	d.sortFields()
	return d, nil
}

func (d Dep) MarshalJSON() ([]byte, error) {
	b, err := yaml.Marshal(d)
	if err != nil {
		return b, err
	}

	m := map[string]any{}
	err = yaml.Unmarshal(b, &m)
	if err != nil {
		return nil, err
	}

	return json.Marshal(m)
}

func (d *Dep) GetLabels() []string {
	return d.Labels
}

type DepDAGItem struct {
	Dep       Dep          `yaml:"dep,omitempty" json:"dep,omitempty"`
	AddedDeps []DepDAGItem `yaml:"addedDep,omitempty" json:"addedDep,omitempty"`
}

// Sorts all fields in a canonical way on a DepDAGItem
func (d *DepDAGItem) sortFields() {
	for i := range d.AddedDeps {
		d.AddedDeps[i].sortFields()
	}

	sort.SliceStable(d.AddedDeps, func(i int, j int) bool {
		return d.AddedDeps[i].Dep.cmpLess(d.AddedDeps[j].Dep)
	})
}

func (d DepDAGItem) MarshalYAML() (interface{}, error) {
	d.sortFields()
	return d, nil
}

func (d DepDAGItem) MarshalJSON() ([]byte, error) {
	b, err := yaml.Marshal(d)
	if err != nil {
		return b, err
	}

	m := map[string]any{}
	err = yaml.Unmarshal(b, &m)
	if err != nil {
		return nil, err
	}

	return json.Marshal(m)
}

type DepsFlatItem struct {
	FileURI      string `yaml:"fileURI" json:"fileURI"`
	Provider     string `yaml:"provider" json:"provider"`
	Dependencies []*Dep `yaml:"dependencies" json:"dependencies"`
}

// Sorts all fields in a canonical way on a DepsFlatItem
func (d *DepsFlatItem) sortFields() {
	sort.SliceStable(d.Dependencies, func(i, j int) bool {
		return (*d.Dependencies[i]).cmpLess(*d.Dependencies[j])
	})
}

func (d DepsFlatItem) MarshalYAML() (interface{}, error) {
	d.sortFields()
	return d, nil
}

func (d DepsFlatItem) MarshalJSON() ([]byte, error) {
	b, err := yaml.Marshal(d)
	if err != nil {
		return b, err
	}

	m := map[string]any{}
	err = yaml.Unmarshal(b, &m)
	if err != nil {
		return nil, err
	}

	return json.Marshal(m)
}

type DepsTreeItem struct {
	FileURI      string       `yaml:"fileURI" json:"fileURI"`
	Provider     string       `yaml:"provider" json:"provider"`
	Dependencies []DepDAGItem `yaml:"dependencies" json:"dependencies"`
}

// Sorts all fields in a canonical way on a DepsTreeItem
func (d *DepsTreeItem) sortFields() {
	// Technically doesn't traverse the entire tree
	sort.SliceStable(d.Dependencies, func(i, j int) bool {
		return d.Dependencies[i].Dep.cmpLess(d.Dependencies[j].Dep)
	})
}

func (d DepsTreeItem) MarshalYAML() (interface{}, error) {
	d.sortFields()
	return d, nil
}

func (d DepsTreeItem) MarshalJSON() ([]byte, error) {
	b, err := yaml.Marshal(d)
	if err != nil {
		return b, err
	}

	m := map[string]any{}
	err = yaml.Unmarshal(b, &m)
	if err != nil {
		return nil, err
	}

	return json.Marshal(m)
}

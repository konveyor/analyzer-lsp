package provider

import (
	"context"
	"os"
	"regexp"
	"strings"

	"github.com/cbroglie/mustache"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-logr/logr"
	"github.com/hashicorp/go-version"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/tracing"
	"go.lsp.dev/uri"
	"go.opentelemetry.io/otel/attribute"
	"gopkg.in/yaml.v2"
)

// This will need a better name, may we want to move it to top level
// Will be used by providers for common interface way of passing in configuration values.
var builtinConfig = Config{
	Name: "builtin",
}

func init() {
	c, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	builtinConfig.InitConfig = []InitConfig{
		{
			Location: c,
		},
	}
}

type UnimplementedDependenciesComponent struct{}

// We don't have dependencies
func (p *UnimplementedDependenciesComponent) GetDependencies() ([]Dep, uri.URI, error) {
	return nil, "", nil
}

// We don't have dependencies
func (p *UnimplementedDependenciesComponent) GetDependenciesDAG() ([]DepDAGItem, uri.URI, error) {
	return nil, "", nil
}

type Capability struct {
	Name            string
	TemplateContext openapi3.SchemaRef
}

type Config struct {
	Name       string       `yaml:"name,omitempty" json:"name,omitempty"`
	BinaryPath string       `yaml:"binaryPath,omitempty" json:"binaryPath,omitempty"`
	Address    string       `yaml:"address,omitempty" json:"address,omitempty"`
	InitConfig []InitConfig `yaml:"initConfig,omitempty" json:"initConfig,omitempty"`
}

type AnalysisMode string

const (
	FullAnalysisMode       AnalysisMode = "full"
	SourceOnlyAnalysisMode AnalysisMode = "source-only"
)

type InitConfig struct {
	// This is the location of the code base that the
	// Provider will be responisble for parsing
	Location string `yaml:"location,omitempty" json:"location,omitempty"`

	// This is the path to look for the dependencies for the project.
	// It is relative to the Location
	DependencyPath string `yaml:"dependencyPath,omitempty" json:"dependencyPath,omitempty"`

	LSPServerPath string `yaml:"lspServerPath,omitempty" json:"lspServerPath,omitempty"`

	AnalysisMode AnalysisMode `yaml:"analysisMode" json:"analysisMode"`

	// This will have to be defined for each provider
	ProviderSpecificConfig map[string]interface{} `yaml:"providerSpecificConfig,omitempty" json:"providerSpecificConfig,omitempty"`
}

func GetConfig(filepath string) ([]Config, error) {
	content, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	configs := []Config{}

	err = yaml.Unmarshal(content, &configs)
	if err != nil {
		return nil, err
	}
	foundBuiltin := false
	for _, c := range configs {
		if c.Name == builtinConfig.Name {
			foundBuiltin = true
		}
	}
	if !foundBuiltin {
		configs = append(configs, builtinConfig)
	}

	return configs, nil

}

type ProviderEvaluateResponse struct {
	Matched         bool                   `yaml:"matched"`
	Incidents       []IncidentContext      `yaml:"incidents"`
	TemplateContext map[string]interface{} `yaml:"templateContext"`
}

type IncidentContext struct {
	FileURI      uri.URI                `yaml:"fileURI"`
	Effort       *int                   `yaml:"effort,omitempty"`
	LineNumber   *int                   `yaml:"lineNumber,omitempty"`
	Variables    map[string]interface{} `yaml:"variables,omitempty"`
	Links        []ExternalLinks        `yaml:"externalLink,omitempty"`
	CodeLocation *Location              `yaml:"location,omitempty"`
}

type Location struct {
	StartPosition Position
	EndPosition   Position
}

type Position struct {
	/*Line defined:
	 * Line position in a document (zero-based).
	 * If a line number is greater than the number of lines in a document, it defaults back to the number of lines in the document.
	 * If a line number is negative, it defaults to 0.
	 */
	Line float64 `json:"line"`

	/*Character defined:
	 * Character offset on a line in a document (zero-based). Assuming that the line is
	 * represented as a string, the `character` value represents the gap between the
	 * `character` and `character + 1`.
	 *
	 * If the character value is greater than the line length it defaults back to the
	 * line length.
	 * If a line number is negative, it defaults to 0.
	 */
	Character float64 `json:"character"`
}

type ExternalLinks struct {
	URL   string `yaml:"url"`
	Title string `yaml:"title"`
}

type ProviderContext struct {
	Tags     map[string]interface{}          `yaml:"tags"`
	Template map[string]engine.ChainTemplate `yaml:"template"`
}

func HasCapability(caps []Capability, name string) bool {
	for _, cap := range caps {
		if cap.Name == name {
			return true
		}
	}
	return false
}

func FullResponseFromServiceClients(clients []ServiceClient, cap string, conditionInfo []byte) (ProviderEvaluateResponse, error) {
	fullResp := ProviderEvaluateResponse{
		Matched:         false,
		Incidents:       []IncidentContext{},
		TemplateContext: map[string]interface{}{},
	}
	for _, c := range clients {
		r, err := c.Evaluate(cap, conditionInfo)
		if err != nil {
			return fullResp, err
		}
		if !fullResp.Matched {
			fullResp.Matched = r.Matched
		}
		fullResp.Incidents = append(fullResp.Incidents, r.Incidents...)
		for k, v := range r.TemplateContext {
			fullResp.TemplateContext[k] = v
		}
	}
	return fullResp, nil
}

func FullDepsResponse(clients []ServiceClient) ([]Dep, uri.URI, error) {
	deps := []Dep{}
	var uri uri.URI
	for _, c := range clients {
		r, u, err := c.GetDependencies()
		if err != nil {
			return nil, uri, err
		}
		deps = append(deps, r...)
		uri = u
	}
	return deps, uri, nil
}

func FullDepDAGResponse(clients []ServiceClient) ([]DepDAGItem, uri.URI, error) {
	deps := []DepDAGItem{}
	var uri uri.URI
	for _, c := range clients {
		r, u, err := c.GetDependenciesDAG()
		if err != nil {
			return nil, uri, err
		}
		uri = u
		deps = append(deps, r...)
	}
	return deps, uri, nil
}

// InternalInit interface is going to be used to init the full config of a provider.
// used by the engine/analyzer to get a provider ready.
type InternalInit interface {
	ProviderInit(context.Context) error
}

type InternalProviderClient interface {
	InternalInit
	Client
}

type Client interface {
	BaseClient
	ServiceClient
}

type BaseClient interface {
	Capabilities() []Capability
	Init(context.Context, logr.Logger, InitConfig) (ServiceClient, error)
}

// For some period of time during POC this will be in tree, in the future we need to write something that can do this w/ external binaries
type ServiceClient interface {
	Evaluate(cap string, conditionInfo []byte) (ProviderEvaluateResponse, error)

	Stop()

	// GetDependencies will get the dependencies
	// It is the responsibility of the provider to determine how that is done
	GetDependencies() ([]Dep, uri.URI, error)
	// GetDependencies will get the dependencies and return them as a linked list
	// Top level items are direct dependencies, the rest are indirect dependencies
	GetDependenciesDAG() ([]DepDAGItem, uri.URI, error)
}

type Dep struct {
	Name               string                 `json:"name,omitempty"`
	Version            string                 `json:"version,omitempty"`
	Type               string                 `json:"type,omitempty"`
	Indirect           bool                   `json:"indirect,omitempty"`
	ResolvedIdentifier string                 `json:"sha,omitempty"`
	Extras             map[string]interface{} `json:"extras,omitempty"`
}

type DepDAGItem struct {
	Dep       Dep          `json:"dep,omitempty"`
	AddedDeps []DepDAGItem `json:"addedDep,omitempty"`
}

type Startable interface {
	Start(context.Context) error
}

type CodeSnipProvider struct {
	Providers []engine.CodeSnip
}

var _ engine.CodeSnip = &CodeSnipProvider{}

func (p CodeSnipProvider) GetCodeSnip(u uri.URI, l engine.Location) (string, error) {
	for _, p := range p.Providers {
		snip, err := p.GetCodeSnip(u, l)
		if err == nil && snip != "" {
			return snip, nil
		}
	}
	return "", nil
}

type ProviderCondition struct {
	Client        ServiceClient
	Capability    string
	ConditionInfo interface{}
	Rule          engine.Rule
	Ignore        bool
}

func (p *ProviderCondition) Ignorable() bool {
	return p.Ignore
}

func (p *ProviderCondition) Evaluate(ctx context.Context, log logr.Logger, condCtx engine.ConditionContext) (engine.ConditionResponse, error) {
	_, span := tracing.StartNewSpan(
		ctx, "provider-condition", attribute.Key("cap").String(p.Capability))
	defer span.End()

	providerInfo := struct {
		ProviderContext `yaml:",inline"`
		Capability      map[string]interface{} `yaml:",inline"`
	}{
		ProviderContext: ProviderContext{
			Tags:     condCtx.Tags,
			Template: condCtx.Template,
		},
		Capability: map[string]interface{}{
			p.Capability: p.ConditionInfo,
		},
	}

	serializedInfo, err := yaml.Marshal(providerInfo)
	if err != nil {
		//TODO(fabianvf)
		panic(err)
	}
	templatedInfo, err := templateCondition(serializedInfo, condCtx.Template)
	if err != nil {
		//TODO(fabianvf)
		panic(err)
	}
	span.SetAttributes(attribute.Key("condition").String(string(templatedInfo)))
	resp, err := p.Client.Evaluate(p.Capability, templatedInfo)
	if err != nil {
		// If an error always just return the empty
		return engine.ConditionResponse{}, err
	}

	incidents := []engine.IncidentContext{}
	for _, inc := range resp.Incidents {
		i := engine.IncidentContext{
			FileURI:    inc.FileURI,
			Effort:     inc.Effort,
			LineNumber: inc.LineNumber,
			Variables:  inc.Variables,
			Links:      p.Rule.Perform.Message.Links,
		}

		if inc.CodeLocation != nil {
			i.CodeLocation = &engine.Location{
				StartPosition: engine.Position{
					Line:      int(inc.CodeLocation.StartPosition.Line),
					Character: int(inc.CodeLocation.StartPosition.Character),
				},
				EndPosition: engine.Position{
					Line:      int(inc.CodeLocation.EndPosition.Line),
					Character: int(inc.CodeLocation.EndPosition.Character),
				},
			}
		}
		incidents = append(incidents, i)
	}
	cr := engine.ConditionResponse{
		Matched:         resp.Matched,
		TemplateContext: resp.TemplateContext,
		Incidents:       incidents,
	}

	log.V(8).Info("condition response", "ruleID", p.Rule.RuleID, "response", cr, "cap", p.Capability, "conditionInfo", p.ConditionInfo, "client", p.Client)
	return cr, nil

}

func templateCondition(condition []byte, ctx map[string]engine.ChainTemplate) ([]byte, error) {
	//TODO(shanw-hurley):
	// this is needed because for the initial yaml read, we convert this to a string,
	// then when it is used here, we need the value to be whatever is in the context and not
	// a string nested in the type.
	// This may require some documentation, but I believe that it should be fine.
	// example:
	// xml:
	//   filepaths: '{{poms.filepaths}}'
	//    xpath: //dependencies/dependency
	// converted to
	// xml:
	//   filepaths: {{poms.filepaths}}
	//   xpath: //dependencies/dependency
	s := strings.ReplaceAll(string(condition), `'{{`, "{{")
	s = strings.ReplaceAll(s, `}}'`, "}}")

	s, err := mustache.Render(s, true, ctx)
	if err != nil {
		return nil, err
	}
	return []byte(s), nil
}

// TODO where should this go
type DependencyCondition struct {
	Upperbound string
	Lowerbound string
	Name       string
	// NameRegex will be a valid go regex that will be used to
	// search the name of a given dependency.
	// Examples include kubernetes* or jakarta-.*-2.2.
	NameRegex string

	Client Client
}

func (dc DependencyCondition) Evaluate(ctx context.Context, log logr.Logger, condCtx engine.ConditionContext) (engine.ConditionResponse, error) {
	resp := engine.ConditionResponse{}
	deps, file, err := dc.Client.GetDependencies()
	if err != nil {
		return resp, err
	}
	regex, err := regexp.Compile(dc.NameRegex)
	if err != nil {
		return resp, err
	}
	matchedDeps := []*Dep{}
	for _, dep := range deps {
		if dep.Name == dc.Name {
			matchedDeps = append(matchedDeps, &dep)
			break
		}
		if dc.NameRegex != "" && regex.MatchString(dep.Name) {
			matchedDeps = append(matchedDeps, &dep)
		}
	}

	if len(matchedDeps) == 0 {
		return resp, nil
	}

	for _, matchedDep := range matchedDeps {
		if matchedDep.Version == "" || (dc.Lowerbound == "" && dc.Upperbound == "") {
			resp.Matched = true
			resp.Incidents = append(resp.Incidents, engine.IncidentContext{
				FileURI: file,
				Variables: map[string]interface{}{
					"name":    matchedDep.Name,
					"version": matchedDep.Version,
					"type":    matchedDep.Type,
				},
			})
			// For now, lets leave this TODO to figure out what we should be setting in the context
			resp.TemplateContext = map[string]interface{}{
				"name":    matchedDep.Name,
				"version": matchedDep.Version,
			}
			continue
		}

		depVersion, err := getVersion(matchedDep.Version)
		if err != nil {
			return resp, err
		}

		constraintPieces := []string{}
		if dc.Lowerbound != "" {
			var v string
			lb, err := getVersion(dc.Lowerbound)
			if err != nil {
				v = dc.Lowerbound
			} else {
				v = lb.Original()
			}
			constraintPieces = append(constraintPieces, ">= "+v)
		}
		if dc.Upperbound != "" {
			var v string
			ub, err := getVersion(dc.Upperbound)
			if err != nil {
				v = dc.Upperbound
			} else {
				v = ub.Original()
			}
			constraintPieces = append(constraintPieces, "<= "+v)
		}
		constraints, err := version.NewConstraint(strings.Join(constraintPieces, ", "))
		if err != nil {
			return resp, err
		}

		resp.Matched = constraints.Check(depVersion)
		resp.Incidents = append(resp.Incidents, engine.IncidentContext{
			FileURI: file,
			Variables: map[string]interface{}{
				"name":    matchedDep.Name,
				"version": matchedDep.Version,
			},
		})
		resp.TemplateContext = map[string]interface{}{
			"name":    matchedDep.Name,
			"version": matchedDep.Version,
		}
	}

	return resp, nil
}

// TODO(fabianvf): We need to strip out the go-version library for a more lenient
// one, since it breaks on the `.RELEASE` and `.Final` suffixes which are common in Java.
// This function will extract only a numeric version pattern and strip out those suffixes.
// In the long term we'll probably need to write a version comparison library from scratch.
func getVersion(depVersion string) (*version.Version, error) {
	v, err := version.NewVersion(depVersion)
	if err == nil {
		return v, nil
	}
	// Parsing failed so we'll try to extract a version and parse that
	re := regexp.MustCompile("v?([0-9]+(?:\\.[0-9]+)*)")
	matches := re.FindStringSubmatch(depVersion)

	// The group is matching twice for some reason, double-check it's just a dup match
	trueMatches := map[string]bool{}
	for _, match := range matches {
		trueMatches[match] = true
	}
	if len(trueMatches) != 1 {
		return nil, err
	}
	return version.NewVersion(matches[0])
}

// Convert Dag Item List to flat list.
func ConvertDagItemsToList(items []DepDAGItem) []Dep {
	deps := []Dep{}
	for _, i := range items {
		deps = append(deps, i.Dep)
		deps = append(deps, ConvertDagItemsToList(i.AddedDeps)...)
	}
	return deps
}

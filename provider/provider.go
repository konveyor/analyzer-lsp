package provider

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/cbroglie/mustache"
	"github.com/go-logr/logr"
	"github.com/hashicorp/go-version"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/engine/labels"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/tracing"
	jsonschema "github.com/swaggest/jsonschema-go"
	"github.com/swaggest/openapi-go/openapi3"
	"go.lsp.dev/uri"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/net/http/httpproxy"
	"gopkg.in/yaml.v2"
)

const (
	// Dep source label is a label key that any provider can use, to label the dependencies as coming from a particular source.
	// Examples from java are: open-source and internal. A provider can also have a user provide file that will tell them which
	// depdendencies to label as this value. This label will be used to filter out these dependencies from a given analysis
	DepSourceLabel   = "konveyor.io/dep-source"
	DepLanguageLabel = "konveyor.io/language"
	DepExcludeLabel  = "konveyor.io/exclude"
	// LspServerPath is a provider specific config used to specify path to a LSP server
	LspServerPathConfigKey = "lspServerPath"
)

// We need to make these Vars, because you can not take a pointer of the constant.
var (
	SchemaTypeString openapi3.SchemaType = openapi3.SchemaTypeString
	SchemaTypeArray  openapi3.SchemaType = openapi3.SchemaTypeArray
	SchemaTypeObject openapi3.SchemaType = openapi3.SchemaTypeObject
	SchemaTypeNumber openapi3.SchemaType = openapi3.SchemaTypeInteger
	SchemaTypeBool   openapi3.SchemaType = openapi3.SchemaTypeBoolean
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
func (p *UnimplementedDependenciesComponent) GetDependencies(ctx context.Context) (map[uri.URI][]*Dep, error) {
	return nil, nil
}

// We don't have dependencies
func (p *UnimplementedDependenciesComponent) GetDependenciesDAG(ctx context.Context) (map[uri.URI][]DepDAGItem, error) {
	return nil, nil
}

type Capability struct {
	Name   string
	Input  openapi3.SchemaOrRef
	Output openapi3.SchemaOrRef
}

type Config struct {
	Name         string       `yaml:"name,omitempty" json:"name,omitempty"`
	BinaryPath   string       `yaml:"binaryPath,omitempty" json:"binaryPath,omitempty"`
	Address      string       `yaml:"address,omitempty" json:"address,omitempty"`
	Proxy        *Proxy       `yaml:"proxyConfig,omitempty" json:"proxyConfig,omitempty"`
	InitConfig   []InitConfig `yaml:"initConfig,omitempty" json:"initConfig,omitempty"`
	ContextLines int
}

type Proxy httpproxy.Config

func (p Proxy) ToEnvVars() map[string]string {
	proxy := map[string]string{}
	if p.HTTPProxy != "" {
		proxy["http_proxy"] = p.HTTPProxy
	}
	if p.HTTPSProxy != "" {
		proxy["https_proxy"] = p.HTTPSProxy
	}
	if p.NoProxy != "" {
		proxy["no_proxy"] = p.NoProxy
	}
	return proxy
}

type AnalysisMode string

const (
	FullAnalysisMode       AnalysisMode = "full"
	SourceOnlyAnalysisMode AnalysisMode = "source-only"
)

type InitConfig struct {
	// This is the location of the code base that the
	// Provider will be responisble for parsing
	// TODO: rootUri, which is what this maps to in the LSP spec, is deprecated.
	// We should instead use workspaceFolders.
	Location string `yaml:"location,omitempty" json:"location,omitempty"`

	// This is the path to look for the dependencies for the project.
	// It is relative to the Location
	// TODO: This only allows for one directory for dependencies. Use DependencyFolders instead
	DependencyPath string `yaml:"dependencyPath,omitempty" json:"dependencyPath,omitempty"`

	// It would be nice to get workspacefolders working

	// // The folders for the workspace. Maps to workspaceFolders in the LSP spec
	// WorkspaceFolders []string `yaml:"workspaceFolders,omitempty" json:"workspaceFolders,omitempty"`

	// // The folders for the dependencies. Also maps to workspaceFolders in the LSP
	// // spec. These folders will not be inlcuded in search results for things like
	// // 'referenced'.
	// DependencyFolders []string `yaml:"dependencyFolders,omitempty" json:"dependencyFolders,omitempty"`

	AnalysisMode AnalysisMode `yaml:"analysisMode" json:"analysisMode"`

	// This will have to be defined for each provider
	ProviderSpecificConfig map[string]interface{} `yaml:"providerSpecificConfig,omitempty" json:"providerSpecificConfig,omitempty"`

	Proxy *Proxy `yaml:"proxyConfig,omitempty" json:"proxyConfig,omitempty"`
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
	for idx := range configs {
		c := &configs[idx]
		if c.Name == builtinConfig.Name {
			foundBuiltin = true
		}
		// default to system-wide proxy
		if c.Proxy == nil {
			c.Proxy = (*Proxy)(httpproxy.FromEnvironment())
		}
		for jdx := range c.InitConfig {
			ic := &c.InitConfig[jdx]
			// if a specific proxy config not present
			// use provider wide config
			if ic.Proxy == nil {
				ic.Proxy = c.Proxy
			}
		}
	}
	if !foundBuiltin {
		configs = append(configs, builtinConfig)
	}

	// Validate provider names for duplicate providers.
	if err := validateProviderName(configs); err != nil {
		return nil, err
	}

	return configs, nil

}

func validateProviderName(configs []Config) error {
	providerNames := make(map[string]bool)
	for _, config := range configs {
		name := strings.TrimSpace(config.Name)
		// Check if the provider name is empty
		if name == "" {
			return fmt.Errorf("provider name should not be empty")
		}
		// Check the provider already exist in providerNames map
		if providerNames[name] {
			return fmt.Errorf("duplicate providers found: %s", name)
		}
		providerNames[name] = true
	}
	return nil
}

type ProviderEvaluateResponse struct {
	Matched         bool                   `yaml:"matched"`
	Incidents       []IncidentContext      `yaml:"incidents"`
	TemplateContext map[string]interface{} `yaml:"templateContext"`
}

type IncidentContext struct {
	FileURI              uri.URI                `yaml:"fileURI"`
	Effort               *int                   `yaml:"effort,omitempty"`
	LineNumber           *int                   `yaml:"lineNumber,omitempty"`
	Variables            map[string]interface{} `yaml:"variables,omitempty"`
	Links                []ExternalLinks        `yaml:"externalLink,omitempty"`
	CodeLocation         *Location              `yaml:"location,omitempty"`
	IsDependencyIncident bool
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

func FullResponseFromServiceClients(ctx context.Context, clients []ServiceClient, cap string, conditionInfo []byte) (ProviderEvaluateResponse, error) {
	fullResp := ProviderEvaluateResponse{
		Matched:         false,
		Incidents:       []IncidentContext{},
		TemplateContext: map[string]interface{}{},
	}
	for _, c := range clients {
		r, err := c.Evaluate(ctx, cap, conditionInfo)
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

func FullDepsResponse(ctx context.Context, clients []ServiceClient) (map[uri.URI][]*Dep, error) {
	deps := map[uri.URI][]*Dep{}
	for _, c := range clients {
		r, err := c.GetDependencies(ctx)
		if err != nil {
			return nil, err
		}
		for k, v := range r {
			deps[k] = v
		}
		deps = deduplicateDependencies(deps)
	}
	return deps, nil
}

func FullDepDAGResponse(ctx context.Context, clients []ServiceClient) (map[uri.URI][]DepDAGItem, error) {
	deps := map[uri.URI][]DepDAGItem{}
	for _, c := range clients {
		r, err := c.GetDependenciesDAG(ctx)
		if err != nil {
			return nil, err
		}
		for k, v := range r {
			deps[k] = v
		}
	}
	return deps, nil
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
	Evaluate(ctx context.Context, cap string, conditionInfo []byte) (ProviderEvaluateResponse, error)

	Stop()

	// GetDependencies will get the dependencies
	// It is the responsibility of the provider to determine how that is done
	GetDependencies(ctx context.Context) (map[uri.URI][]*Dep, error)
	// GetDependencies will get the dependencies and return them as a linked list
	// Top level items are direct dependencies, the rest are indirect dependencies
	GetDependenciesDAG(ctx context.Context) (map[uri.URI][]DepDAGItem, error)
}

type DependencyLocationResolver interface {
	GetLocation(ctx context.Context, dep konveyor.Dep) (engine.Location, error)
}

type Dep = konveyor.Dep
type DepDAGItem = konveyor.DepDAGItem
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
	Client           ServiceClient
	Capability       string
	ConditionInfo    interface{}
	Rule             engine.Rule
	Ignore           bool
	DepLabelSelector *labels.LabelSelector[*Dep]
}

func (p ProviderCondition) Ignorable() bool {
	return p.Ignore
}

func (p ProviderCondition) Evaluate(ctx context.Context, log logr.Logger, condCtx engine.ConditionContext) (engine.ConditionResponse, error) {
	ctx, span := tracing.StartNewSpan(
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
	resp, err := p.Client.Evaluate(ctx, p.Capability, templatedInfo)
	if err != nil {
		// If an error always just return the empty
		return engine.ConditionResponse{}, err
	}

	var deps map[uri.URI][]*Dep
	if p.DepLabelSelector != nil {
		deps, err = p.Client.GetDependencies(ctx)
		if err != nil {
			return engine.ConditionResponse{}, err
		}
		deps = deduplicateDependencies(deps)
	}

	incidents := []engine.IncidentContext{}
	for _, inc := range resp.Incidents {
		// filter out incidents that don't match the dep label selector
		if matched, err := matchDepLabelSelector(p.DepLabelSelector, inc, deps); err != nil {
			log.V(5).Error(err, "failed to match dep label selector")
			return engine.ConditionResponse{}, err
		} else if !matched {
			continue
		}
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

	// If there are no incidents, don't generate any violations
	if len(incidents) == 0 && len(resp.Incidents)-len(incidents) > 0 {
		log.V(5).Info("filtered out all incidents based on dep label selector", "filteredOutCount", len(resp.Incidents)-len(incidents))
		return engine.ConditionResponse{
			Matched: resp.Matched,
		}, nil
	}

	cr := engine.ConditionResponse{
		Matched:         resp.Matched,
		TemplateContext: resp.TemplateContext,
		Incidents:       incidents,
	}

	log.V(8).Info("condition response", "ruleID", p.Rule.RuleID, "response", cr, "cap", p.Capability, "conditionInfo", p.ConditionInfo, "client", p.Client)
	if len(resp.Incidents)-len(incidents) > 0 {
		log.V(5).Info("filtered out incidents based on dep label selector", "filteredOutCount", len(resp.Incidents)-len(incidents))
	}
	return cr, nil

}

// matchDepLabelSelector evaluates the dep label selector on incident
func matchDepLabelSelector(s *labels.LabelSelector[*Dep], inc IncidentContext, deps map[uri.URI][]*konveyor.Dep) (bool, error) {
	// always match non dependency URIs or when there are no deps or no dep selector
	if !inc.IsDependencyIncident || s == nil || deps == nil || len(deps) == 0 || inc.FileURI == "" {
		return true, nil
	}
	matched := false
	for _, depList := range deps {
		depList, err := s.MatchList(depList)
		if err != nil {
			return false, err
		}
		for _, d := range depList {
			if strings.HasPrefix(string(inc.FileURI), d.FileURIPrefix) {
				matched = true
			}
		}
	}
	return matched, nil
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

type DependencyConditionCap struct {
	Upperbound string `json:"upperbound,omitempty"`
	Lowerbound string `json:"lowerbound,omitempty"`
	Name       string `json:"name"`
	// NameRegex will be a valid go regex that will be used to
	// search the name of a given dependency.
	// Examples include kubernetes* or jakarta-.*-2.2.
	NameRegex string `json:"name_regex,omitempty"`
}

// TODO where should this go
type DependencyCondition struct {
	DependencyConditionCap

	Client Client
}

func (dc DependencyCondition) Evaluate(ctx context.Context, log logr.Logger, condCtx engine.ConditionContext) (engine.ConditionResponse, error) {
	_, span := tracing.StartNewSpan(ctx, "dep-condition")
	defer span.End()

	resp := engine.ConditionResponse{}
	deps, err := dc.Client.GetDependencies(ctx)
	if err != nil {
		return resp, err
	}
	regex, err := regexp.Compile(dc.NameRegex)
	if err != nil {
		return resp, err
	}
	type matchedDep struct {
		dep *Dep
		uri uri.URI
	}
	matchedDeps := []matchedDep{}
	for u, ds := range deps {
		for _, dep := range ds {
			if dep.Name == dc.Name {
				matchedDeps = append(matchedDeps, matchedDep{dep: dep, uri: u})
				break
			}
			if dc.NameRegex != "" && regex.MatchString(dep.Name) {
				matchedDeps = append(matchedDeps, matchedDep{dep: dep, uri: u})
			}
		}
	}

	if len(matchedDeps) == 0 {
		return resp, nil
	}

	var depLocationResolver DependencyLocationResolver
	depLocationResolver, _ = dc.Client.(DependencyLocationResolver)

	for _, matchedDep := range matchedDeps {
		if matchedDep.dep.Version == "" || (dc.Lowerbound == "" && dc.Upperbound == "") {
			incident := engine.IncidentContext{
				FileURI: matchedDep.uri,
				Variables: map[string]interface{}{
					"name":    matchedDep.dep.Name,
					"version": matchedDep.dep.Version,
					"type":    matchedDep.dep.Type,
				},
			}
			if depLocationResolver != nil {
				// this is a best-effort step and we don't want to block if resolver misbehaves
				timeoutContext, cancelFunc := context.WithTimeout(ctx, time.Second*3)
				location, err := depLocationResolver.GetLocation(timeoutContext, *matchedDep.dep)
				if err == nil {
					incident.LineNumber = &location.StartPosition.Line
					incident.CodeLocation = &location
				} else {
					log.V(7).Error(err, "failed to get location for dependency", "dep", matchedDep.dep.Name)
				}
				cancelFunc()
			}
			resp.Matched = true
			resp.Incidents = append(resp.Incidents, incident)
			// For now, lets leave this TODO to figure out what we should be setting in the context
			resp.TemplateContext = map[string]interface{}{
				"name":    matchedDep.dep.Name,
				"version": matchedDep.dep.Version,
			}
			continue
		}

		depVersion, err := getVersion(matchedDep.dep.Version)
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
		incident := engine.IncidentContext{
			FileURI: matchedDep.uri,
			Variables: map[string]interface{}{
				"name":    matchedDep.dep.Name,
				"version": matchedDep.dep.Version,
			},
		}
		if depLocationResolver != nil {
			// this is a best-effort step and we don't want to block if resolver misbehaves
			timeoutContext, cancelFunc := context.WithTimeout(context.Background(), time.Second*3)
			location, err := depLocationResolver.GetLocation(timeoutContext, *matchedDep.dep)
			if err == nil {
				incident.LineNumber = &location.StartPosition.Line
				incident.CodeLocation = &location
			} else if baseDep, ok := matchedDep.dep.Extras["baseDep"]; ok {
				// Use "parent" baseDep location lookup for indirect dependencies
				location, err = depLocationResolver.GetLocation(timeoutContext, baseDep.(konveyor.Dep))
				if err == nil {
					incident.LineNumber = &location.StartPosition.Line
					incident.CodeLocation = &location
				} else {
					log.V(7).Error(err, "failed to get location for indirect dependency", "dep", matchedDep.dep.Name)
				}
			} else {
				log.V(7).Error(err, "failed to get location for dependency", "dep", matchedDep.dep.Name)
			}
			cancelFunc()
		}
		resp.Incidents = append(resp.Incidents, incident)
		resp.TemplateContext = map[string]interface{}{
			"name":    matchedDep.dep.Name,
			"version": matchedDep.dep.Version,
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
	re := regexp.MustCompile(`v?([0-9]+(?:.[0-9]+)*)`)
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
func ConvertDagItemsToList(items []DepDAGItem) []*Dep {
	deps := []*Dep{}
	for _, i := range items {
		d := i.Dep
		deps = append(deps, &d)
		deps = append(deps, ConvertDagItemsToList(i.AddedDeps)...)
	}
	return deps
}

func deduplicateDependencies(dependencies map[uri.URI][]*Dep) map[uri.URI][]*Dep {
	// Just need this so I can differentiate between dependencies that aren't found
	// and dependencies that are at index 0
	intPtr := func(i int) *int {
		return &i
	}
	deduped := map[uri.URI][]*Dep{}
	for uri, deps := range dependencies {
		deduped[uri] = []*Dep{}
		depSeen := map[string]*int{}
		for _, dep := range deps {
			id := dep.Name + dep.Version + dep.ResolvedIdentifier
			if depSeen[id+"direct"] != nil {
				// We've already seen it and it's direct, nothing to do
				continue
			} else if depSeen[id+"indirect"] != nil {
				if !dep.Indirect {
					// We've seen it as an indirect, need to update the dep in
					// the list to reflect that it's actually a direct dependency
					deduped[uri][*depSeen[id+"indirect"]].Indirect = false
					depSeen[id+"direct"] = depSeen[id+"indirect"]
				} else {
					// Otherwise, we've just already seen it
					continue
				}
			} else {
				// We haven't seen this before and need to update the dedup
				// list and mark that we've seen it
				deduped[uri] = append(deduped[uri], dep)
				if dep.Indirect {
					depSeen[id+"indirect"] = intPtr(len(deduped) - 1)
				} else {
					depSeen[id+"direct"] = intPtr(len(deduped) - 1)
				}
			}
		}
	}
	return deduped
}

func ToProviderCap(r *openapi3.Reflector, log logr.Logger, cond interface{}, name string) (Capability, error) {
	jsonCondition, err := r.Reflector.Reflect(cond)
	if err != nil {
		log.Error(err, "fix it")
		return Capability{}, err
	}
	s := &openapi3.SchemaOrRef{}
	s.FromJSONSchema(jsonschema.SchemaOrBool{
		TypeObject: &jsonCondition,
	})
	return Capability{
		Name:  name,
		Input: *s,
	}, nil

}

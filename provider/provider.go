package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/cbroglie/mustache"
	"github.com/go-logr/logr"
	"github.com/hashicorp/go-version"
	"github.com/konveyor/analyzer-lsp/dependency/dependency"
	depprovider "github.com/konveyor/analyzer-lsp/dependency/provider"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/provider/builtin"
	"github.com/konveyor/analyzer-lsp/provider/golang"
	"github.com/konveyor/analyzer-lsp/provider/java"
	"github.com/konveyor/analyzer-lsp/provider/lib"
	"github.com/konveyor/analyzer-lsp/tracing"
	"go.opentelemetry.io/otel/attribute"
	"gopkg.in/yaml.v2"
)

// For some period of time during POC this will be in tree, in the future we need to write something that can do this w/ external binaries
type Client interface {
	Capabilities() []lib.Capability
	HasCapability(name string) bool

	// Block until initialized
	Init(context.Context, logr.Logger) error

	Evaluate(cap string, conditionInfo []byte) (lib.ProviderEvaluateResponse, error)

	Stop()

	depprovider.DependencyProvider
}

type ProviderCondition struct {
	Client
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
		lib.ProviderContext `yaml:",inline"`
		Capability          map[string]interface{} `yaml:",inline"`
	}{
		ProviderContext: lib.ProviderContext{
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
		incidents = append(incidents, engine.IncidentContext{
			FileURI: inc.FileURI,
			Effort:  inc.Effort,
			Extras:  inc.Extras,
			Links:   p.Rule.Links,
		})
	}

	return engine.ConditionResponse{
		Matched:         resp.Matched,
		TemplateContext: resp.TemplateContext,
		Incidents:       incidents,
	}, nil

}

func templateCondition(condition []byte, ctx map[string]lib.ChainTemplate) ([]byte, error) {
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

// We need some wrapper that can deal with out of tree providers, this will be a call, that will mock it out, but go against in tree.
func GetProviderClient(config lib.Config) (Client, error) {
	switch config.Name {
	case "go":
		return golang.NewGolangProvider(config), nil
	case "java":
		return java.NewJavaProvider(config), nil
	case "builtin":
		return builtin.NewBuiltinProvider(config), nil
	default:
		return nil, fmt.Errorf("unknown and invalid provider client")
	}
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
	deps, err := dc.Client.GetDependencies()
	if err != nil {
		return resp, err
	}
	regex, err := regexp.Compile(dc.NameRegex)
	if err != nil {
		fmt.Printf("regex: %v", regex)
		return resp, err
	}
	matchedDeps := []*dependency.Dep{}
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
				FileURI: matchedDep.Type,
				Extras: map[string]interface{}{
					"name":    matchedDep.Name,
					"version": matchedDep.Version,
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
			constraintPieces = append(constraintPieces, "> "+dc.Lowerbound)
		}
		if dc.Upperbound != "" {
			constraintPieces = append(constraintPieces, "< "+dc.Upperbound)
		}
		constraints, err := version.NewConstraint(strings.Join(constraintPieces, ", "))
		if err != nil {
			return resp, err
		}

		resp.Matched = constraints.Check(depVersion)
		resp.Incidents = append(resp.Incidents, engine.IncidentContext{
			FileURI: matchedDep.Type,
			Extras: map[string]interface{}{
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

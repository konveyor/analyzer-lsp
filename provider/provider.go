package provider

import (
	"bytes"
	"context"
	"fmt"
	"text/template"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/provider/builtin"
	"github.com/konveyor/analyzer-lsp/provider/golang"
	"github.com/konveyor/analyzer-lsp/provider/java"
	"github.com/konveyor/analyzer-lsp/provider/lib"
	"gopkg.in/yaml.v2"
)

// For some period of time during POC this will be in tree, in the future we need to write something that can do this w/ external binaries
type Client interface {
	Capabilities() ([]string, error)

	// Block until initialized
	Init(context.Context, logr.Logger) error

	Evaluate(cap string, conditionInfo []byte) (lib.ProviderEvaluateResponse, error)

	Stop()
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

func (p *ProviderCondition) Evaluate(log logr.Logger, ctx map[string]interface{}) (engine.ConditionResponse, error) {
	serializedInfo, err := yaml.Marshal(map[string]interface{}{p.Capability: p.ConditionInfo})
	if err != nil {
		//TODO(fabianvf)
		panic(err)
	}
	templatedInfo, err := templateCondition(serializedInfo, ctx)
	if err != nil {
		//TODO(fabianvf)
		panic(err)
	}
	resp, err := p.Client.Evaluate(p.Capability, templatedInfo)
	if err != nil {
		// If an error always just return the empty
		return engine.ConditionResponse{}, err
	}

	incidents := []engine.IncidentContext{}
	for _, inc := range resp.Incidents {
		links := []engine.ExternalLinks{}
		for _, link := range inc.Links {
			links = append(links, engine.ExternalLinks{
				URL:   link.URL,
				Title: link.Title,
			})
		}
		// Default to -1 if no effort is assinged.
		// TODO(shawn-hurley evaluate if this is the correct default or if we want the end facing doc to have nil as an option.)
		effort := int(-1)
		if inc.Effort != nil {
			effort = *inc.Effort
		}

		incidents = append(incidents, engine.IncidentContext{
			FileURI: inc.FileURI,
			Effort:  effort,
			Extras:  inc.Extras,
			Links:   links,
		})
	}

	return engine.ConditionResponse{
		Passed:          resp.Passed,
		TemplateContext: resp.TemplateContext,
		Incidents:       incidents,
	}, nil

}

func templateCondition(condition []byte, ctx map[string]interface{}) ([]byte, error) {
	//TODO(fabianvf): this delim thing is a pretty gross hack
	t, err := template.New("condition").Delims("'{{", "}}'").Parse(string(condition))
	if err != nil {
		return nil, err
	}
	buf := &bytes.Buffer{}
	err = t.Execute(buf, ctx)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
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

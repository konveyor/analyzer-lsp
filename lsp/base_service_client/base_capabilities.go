package base

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
)

// Type aliases to make the function definitions shorter

type ctx = context.Context
type resp = provider.ProviderEvaluateResponse
type base = HasLSPServiceClientBase

// Technically not necessary
type NoOpCondition struct{}

// A simple no-op function. Returns a blank response
func EvaluateNoOp[T base](t T, ctx ctx, cap string, info []byte) (resp, error) {
	return resp{}, nil
}

// Generic referenced condition
type ReferencedCondition struct {
	Referenced struct {
		Pattern string `yaml:"pattern"`
	} `yaml:"referenced"`
}

// EvaluateReferenced evaluates references to a given entity based on a query
// pattern. The function uses the provided query pattern to find references to
// the specified entity within the workspace, filters out references in certain
// directories, and returns a list of incident contexts associated with these
// references.
func EvaluateReferenced[T base](t T, ctx ctx, cap string, info []byte) (resp, error) {
	sc := t.GetLSPServiceClientBase()

	var cond ReferencedCondition
	err := yaml.Unmarshal(info, &cond)
	if err != nil {
		return resp{}, fmt.Errorf("error unmarshaling query info")
	}

	query := cond.Referenced.Pattern
	if query == "" {
		return resp{}, fmt.Errorf("unable to get query info")
	}

	symbols := sc.GetAllDeclarations(ctx, sc.BaseConfig.WorkspaceFolders, query)

	incidents := []provider.IncidentContext{}
	incidentsMap := make(map[string]provider.IncidentContext) // Remove duplicates

	for _, s := range symbols {
		references := sc.GetAllReferences(ctx, s.Location.Value.(protocol.Location))

		breakEarly := false
		for _, ref := range references {
			// Look for things that are in the location loaded,
			// Note may need to filter out vendor at some point
			if !strings.Contains(ref.URI, sc.BaseConfig.WorkspaceFolders[0]) {
				continue
			}

			for _, substr := range sc.BaseConfig.DependencyFolders {
				if substr == "" {
					continue
				}

				if strings.Contains(ref.URI, substr) {
					breakEarly = true
					break
				}
			}

			if breakEarly {
				break
			}

			u, err := uri.Parse(ref.URI)
			if err != nil {
				return resp{}, err
			}
			lineNumber := int(ref.Range.Start.Line)
			incident := provider.IncidentContext{
				FileURI:    u,
				LineNumber: &lineNumber,
				Variables: map[string]interface{}{
					"file": ref.URI,
				},
			}
			b, _ := json.Marshal(incident)

			incidentsMap[string(b)] = incident
		}
	}

	for _, incident := range incidentsMap {
		incidents = append(incidents, incident)
	}

	// No results were found.
	if len(incidents) == 0 {
		return resp{Matched: false}, nil
	}
	return resp{
		Matched:   true,
		Incidents: incidents,
	}, nil
}

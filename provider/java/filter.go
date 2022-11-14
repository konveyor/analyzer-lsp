package java

import (
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider/lib"
)

const (
	LINE_NUMBER_EXTRA_KEY = "lineNumber"
	KIND_EXTRA_KEY        = "kind"
	SYMBOL_NAME_KEY       = "name"
	FILE_KEY              = "file"
)

func (p *javaProvider) filterTypeReferences(symbols []protocol.WorkspaceSymbol) ([]lib.IncidentContext, error) {
	incidents := []lib.IncidentContext{}
	for _, symbol := range symbols {
		references := p.GetAllReferences(symbol)

		for _, ref := range references {
			// Look for things that are in the location loaded, //Note may need to filter out vendor at some point
			// if strings.Contains(ref.URI, p.config.Location) {
			incidents = append(incidents, lib.IncidentContext{
				FileURI: ref.URI,
				Extras: map[string]interface{}{
					LINE_NUMBER_EXTRA_KEY: ref.Range.Start.Line,
					// TODO(fabianvf) remove this, Temporary for testing purpses
					KIND_EXTRA_KEY:  symbolKindToString(symbol.Kind),
					SYMBOL_NAME_KEY: symbol.Name,
					FILE_KEY:        ref.URI,
				},
			})
		}
	}
	return incidents, nil
}

// TODO: we will probably want to filter symbols bassed on if in any way the method is being used in the code directly.
// This will need to be part of a "filtration" concept that windup has. Searching partiular subsets of things (just the application, applicatoin + corp libraries and the everything.)
// Today this is just giving everything.
func (p *javaProvider) filterMethodSymbols(symbols []protocol.WorkspaceSymbol) ([]lib.IncidentContext, error) {
	incidents := []lib.IncidentContext{}
	for _, symbol := range symbols {
		// Verify symbol is method.
		if symbol.Kind != protocol.Method {
			continue
		}

		incidents = append(incidents, lib.IncidentContext{
			FileURI: symbol.Location.URI,
			Extras: map[string]interface{}{
				LINE_NUMBER_EXTRA_KEY: symbol.Location.Range.Start.Line,

				KIND_EXTRA_KEY:  symbolKindToString(symbol.Kind),
				SYMBOL_NAME_KEY: symbol.Name,
				FILE_KEY:        symbol.Location.URI,
			},
		})
	}
	return incidents, nil

}

func (p *javaProvider) filterConstructorSymbols(symbols []protocol.WorkspaceSymbol) ([]lib.IncidentContext, error) {

	incidents := []lib.IncidentContext{}
	for _, symbol := range symbols {
		// Verify symbol is Constructor.
		if symbol.Kind != protocol.Constructor {
			continue
		}

		incidents = append(incidents, lib.IncidentContext{
			FileURI: symbol.Location.URI,
			Extras: map[string]interface{}{
				LINE_NUMBER_EXTRA_KEY: symbol.Location.Range.Start.Line,
				KIND_EXTRA_KEY:        symbolKindToString(symbol.Kind),
				SYMBOL_NAME_KEY:       symbol.Name,
				FILE_KEY:              symbol.Location.URI,
			},
		})
	}
	return incidents, nil
}

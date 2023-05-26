package java

import (
	"strings"

	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
)

const (
	LINE_NUMBER_EXTRA_KEY = "lineNumber"
	KIND_EXTRA_KEY        = "kind"
	SYMBOL_NAME_KEY       = "name"
	FILE_KEY              = "file"
)

func (p *javaServiceClient) filterVariableDeclaration(symbols []protocol.WorkspaceSymbol) ([]provider.IncidentContext, error) {
	incidents := []provider.IncidentContext{}
	for _, ref := range symbols {
		incident, err := p.convertToIncidentContext(ref)
		if err != nil {
			return nil, err
		}
		incidents = append(incidents, incident)
	}
	return incidents, nil
}

func (p *javaServiceClient) filterModulesImports(symbols []protocol.WorkspaceSymbol) ([]provider.IncidentContext, error) {
	incidents := []provider.IncidentContext{}
	for _, symbol := range symbols {
		if symbol.Kind != protocol.Module {
			continue
		}
		incident, err := p.convertToIncidentContext(symbol)
		if err != nil {
			return nil, err
		}
		incidents = append(incidents, incident)
	}
	return incidents, nil
}

func (p *javaServiceClient) filterTypesInheritance(symbols []protocol.WorkspaceSymbol) ([]provider.IncidentContext, error) {
	incidents := []provider.IncidentContext{}
	for _, symbol := range symbols {
		incident, err := p.convertToIncidentContext(symbol)
		if err != nil {
			return nil, err
		}
		incidents = append(incidents, incident)
	}

	return incidents, nil
}

func (p *javaServiceClient) filterTypeReferences(symbols []protocol.WorkspaceSymbol) ([]provider.IncidentContext, error) {
	incidents := []provider.IncidentContext{}
	for _, symbol := range symbols {
		references := p.GetAllReferences(symbol)

		for _, ref := range references {
			incident, err := p.convertSymbolRefToIncidentContext(symbol, ref)
			if err != nil {
				return nil, err
			}
			incidents = append(incidents, incident)
		}
	}
	return incidents, nil
}

func (p *javaServiceClient) filterDefault(symbols []protocol.WorkspaceSymbol) ([]provider.IncidentContext, error) {
	incidents := []provider.IncidentContext{}
	for _, symbol := range symbols {
		incident, err := p.convertToIncidentContext(symbol)
		if err != nil {
			return nil, err
		}
		incidents = append(incidents, incident)
	}
	return incidents, nil
}

// TODO: we will probably want to filter symbols bassed on if in any way the method is being used in the code directly.
// This will need to be part of a "filtration" concept that windup has. Searching partiular subsets of things (just the application, applicatoin + corp libraries and the everything.)
// Today this is just giving everything.
func (p *javaServiceClient) filterMethodSymbols(symbols []protocol.WorkspaceSymbol) ([]provider.IncidentContext, error) {
	incidents := []provider.IncidentContext{}
	for _, symbol := range symbols {
		incident, err := p.convertToIncidentContext(symbol)
		if err != nil {
			return nil, err
		}
		incidents = append(incidents, incident)

	}
	return incidents, nil

}

func (p *javaServiceClient) filterConstructorSymbols(symbols []protocol.WorkspaceSymbol) ([]provider.IncidentContext, error) {

	incidents := []provider.IncidentContext{}
	for _, symbol := range symbols {
		references := p.GetAllReferences(symbol)
		for _, ref := range references {
			incident, err := p.convertSymbolRefToIncidentContext(symbol, ref)
			if err != nil {
				return nil, err
			}
			incidents = append(incidents, incident)
		}
	}
	return incidents, nil
}

func (p *javaServiceClient) convertToIncidentContext(symbol protocol.WorkspaceSymbol) (provider.IncidentContext, error) {
	var u uri.URI
	var err error

	// TODO: Can remove when the LSP starts giving files to decompiled binaries
	if strings.HasPrefix(symbol.Location.URI, "jdt") {
		u = uri.URI(symbol.Location.URI)
	} else {
		u, err = uri.Parse(symbol.Location.URI)
		if err != nil {
			return provider.IncidentContext{}, err
		}
	}

	incident := provider.IncidentContext{
		FileURI: u,
		Variables: map[string]interface{}{
			LINE_NUMBER_EXTRA_KEY: symbol.Location.Range.Start.Line,

			KIND_EXTRA_KEY:  symbolKindToString(symbol.Kind),
			SYMBOL_NAME_KEY: symbol.Name,
			FILE_KEY:        symbol.Location.URI,
		},
	}
	if symbol.Location.Range.Start.Line == 0 && symbol.Location.Range.Start.Character == 0 && symbol.Location.Range.End.Line == 0 && symbol.Location.Range.End.Character == 0 {
		return incident, nil
	}
	incident.CodeLocation = &provider.Location{
		StartPosition: provider.Position{
			Line:      symbol.Location.Range.Start.Line,
			Character: symbol.Location.Range.Start.Character,
		},
		EndPosition: provider.Position{
			Line:      symbol.Location.Range.End.Line,
			Character: symbol.Location.Range.End.Character,
		},
	}
	return incident, nil
}

func (p *javaServiceClient) convertSymbolRefToIncidentContext(symbol protocol.WorkspaceSymbol, ref protocol.Location) (provider.IncidentContext, error) {
	var u uri.URI
	var err error

	// TODO: Can remove when the LSP starts giving files to decompiled binaries
	if strings.HasPrefix(symbol.Location.URI, "jdt") {
		u = uri.URI(symbol.Location.URI)
	} else {
		u, err = uri.Parse(ref.URI)
		if err != nil {
			return provider.IncidentContext{}, err
		}
	}
	incident := provider.IncidentContext{
		FileURI: u,
		Variables: map[string]interface{}{

			KIND_EXTRA_KEY:  symbolKindToString(symbol.Kind),
			SYMBOL_NAME_KEY: symbol.Name,
		},
	}
	if ref.Range.Start.Line == 0 && ref.Range.Start.Character == 0 && ref.Range.End.Line == 0 && ref.Range.End.Character == 0 {
		return incident, nil
	}

	incident.CodeLocation = &provider.Location{
		StartPosition: provider.Position{
			Line:      ref.Range.Start.Line,
			Character: ref.Range.Start.Character,
		},
		EndPosition: provider.Position{
			Line:      ref.Range.End.Line,
			Character: ref.Range.End.Character,
		},
	}
	incident.Variables[FILE_KEY] = ref.URI
	incident.Variables[LINE_NUMBER_EXTRA_KEY] = ref.Range.Start.Line

	return incident, nil

}

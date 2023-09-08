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

	var locationURI protocol.DocumentURI
	var locationRange protocol.Range
	switch x := symbol.Location.Value.(type) {
	case protocol.Location:
		locationURI = x.URI
		locationRange = x.Range
	case protocol.PLocationMsg_workspace_symbol:
		locationURI = x.URI
		locationRange = protocol.Range{}
	default:
		locationURI = ""
		locationRange = protocol.Range{}
	}

	if strings.HasPrefix(locationURI, FILE_URI_PREFIX) {
		u = uri.URI(locationURI)
	} else {
		u, err = uri.Parse(locationURI)
		if err != nil {
			return provider.IncidentContext{}, err
		}
	}
	lineNumber := int(locationRange.Start.Line)
	incident := provider.IncidentContext{
		FileURI:    u,
		LineNumber: &lineNumber,
		Variables: map[string]interface{}{

			KIND_EXTRA_KEY:  symbolKindToString(symbol.Kind),
			SYMBOL_NAME_KEY: symbol.Name,
			FILE_KEY:        locationURI,
		},
	}
	if locationRange.Start.Line == 0 && locationRange.Start.Character == 0 && locationRange.End.Line == 0 && locationRange.End.Character == 0 {
		return incident, nil
	}
	incident.CodeLocation = &provider.Location{
		StartPosition: provider.Position{
			Line:      float64(locationRange.Start.Line),
			Character: float64(locationRange.Start.Character),
		},
		EndPosition: provider.Position{
			Line:      float64(locationRange.End.Line),
			Character: float64(locationRange.End.Character),
		},
	}
	return incident, nil
}

func (p *javaServiceClient) convertSymbolRefToIncidentContext(symbol protocol.WorkspaceSymbol, ref protocol.Location) (provider.IncidentContext, error) {
	var u uri.URI
	var err error

	// TODO: Can remove when the LSP starts giving files to decompiled binaries

	var locationURI protocol.DocumentURI
	switch x := symbol.Location.Value.(type) {
	case protocol.Location:
		locationURI = x.URI
	case protocol.PLocationMsg_workspace_symbol:
		locationURI = x.URI
	default:
		locationURI = ""
	}

	if strings.HasPrefix(locationURI, FILE_URI_PREFIX) {
		u = uri.URI(locationURI)
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
			Line:      float64(ref.Range.Start.Line),
			Character: float64(ref.Range.Start.Character),
		},
		EndPosition: provider.Position{
			Line:      float64(ref.Range.End.Line),
			Character: float64(ref.Range.End.Character),
		},
	}
	incident.Variables[FILE_KEY] = ref.URI
	lineNumber := int(ref.Range.Start.Line)
	incident.LineNumber = &lineNumber

	return incident, nil

}

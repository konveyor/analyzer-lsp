package java

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

func (p *javaServiceClient) filterTypeReferences(ctx context.Context, symbols []protocol.WorkspaceSymbol) ([]provider.IncidentContext, error) {
	incidents := []provider.IncidentContext{}
	for _, symbol := range symbols {
		references := p.GetAllReferences(ctx, symbol)

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

func (p *javaServiceClient) filterConstructorSymbols(ctx context.Context, symbols []protocol.WorkspaceSymbol) ([]provider.IncidentContext, error) {

	incidents := []provider.IncidentContext{}
	for _, symbol := range symbols {
		references := p.GetAllReferences(ctx, symbol)
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

	u, err := p.getURI(locationURI)
	if err != nil {
		return provider.IncidentContext{}, err
	}

	lineNumber := int(locationRange.Start.Line) + 1

	incident := provider.IncidentContext{
		FileURI:    u,
		LineNumber: &lineNumber,
		Variables: map[string]interface{}{

			KIND_EXTRA_KEY:  symbolKindToString(symbol.Kind),
			SYMBOL_NAME_KEY: symbol.Name,
			FILE_KEY:        u,
		},
	}

	// based on original URI we got, we can tell if this incident appeared in a dep
	if locationURI != "" && strings.HasPrefix(locationURI, JDT_CLASS_FILE_URI_PREFIX) {
		incident.IsDependencyIncident = true
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
	u, err := p.getURI(ref.URI)
	if err != nil {
		return provider.IncidentContext{}, err
	}

	incident := provider.IncidentContext{
		FileURI: u,
		Variables: map[string]interface{}{
			KIND_EXTRA_KEY:  symbolKindToString(symbol.Kind),
			SYMBOL_NAME_KEY: symbol.Name,
			FILE_KEY:        u,
		},
	}

	// based on original URI we got, we can tell if this incident appeared in a dep
	if strings.HasPrefix(ref.URI, JDT_CLASS_FILE_URI_PREFIX) {
		incident.IsDependencyIncident = true
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
	lineNumber := int(ref.Range.Start.Line) + 1
	incident.LineNumber = &lineNumber

	return incident, nil

}

func (p *javaServiceClient) getURI(refURI string) (uri.URI, error) {
	if !strings.HasPrefix(refURI, JDT_CLASS_FILE_URI_PREFIX) {
		return uri.Parse(refURI)
	}

	u, err := url.Parse(refURI)
	if err != nil {
		return uri.URI(""), err
	}

	// Decompile the jar
	sourceRange, err := strconv.ParseBool(u.Query().Get("source-range"))
	if err != nil {
		// then we got some response that does not make sense or should not be valid
		return uri.URI(""), fmt.Errorf("unable to get konveyor-jdt source range query parameter")
	}
	packageName := u.Query().Get("packageName")

	var jarPath string
	if sourceRange {
		// If there is a source range, we know we know there is a sources jar
		jarName := filepath.Base(u.Path)
		s := strings.TrimSuffix(jarName, ".jar")
		s = fmt.Sprintf("%v-sources.jar", s)
		jarPath = filepath.Join(filepath.Dir(u.Path), s)
	} else {
		jarName := filepath.Base(u.Path)
		jarPath = filepath.Join(filepath.Dir(u.Path), jarName)
	}
	path := filepath.Join(strings.Split(strings.TrimSuffix(packageName, ".class"), ".")...)

	javaFileName := fmt.Sprintf("%s.java", filepath.Base(path))
	if i := strings.Index(javaFileName, "$"); i > 0 {
		javaFileName = fmt.Sprintf("%v.java", javaFileName[0:i])
	}

	javaFileAbsolutePath := filepath.Join(filepath.Dir(jarPath), filepath.Dir(path), javaFileName)

	// attempt to decompile when directory for the expected java file doesn't exist
	// if directory exists, assume .java file is present within, this avoids decompiling every Jar
	if _, err := os.Stat(filepath.Dir(javaFileAbsolutePath)); err != nil {
		cmd := exec.Command("jar", "xf", filepath.Base(jarPath))
		cmd.Dir = filepath.Dir(jarPath)
		err := cmd.Run()
		if err != nil {
			fmt.Printf("\n java error%v", err)
			return "", err
		}
	}

	return uri.New(javaFileAbsolutePath), nil

}

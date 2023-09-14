package java

import (
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
	u, err := p.getURI(symbol.Location.URI)
	if err != nil {
		return provider.IncidentContext{}, err
	}

	lineNumber := int(symbol.Location.Range.Start.Line)
	incident := provider.IncidentContext{
		FileURI:    u,
		LineNumber: &lineNumber,
		Variables: map[string]interface{}{

			KIND_EXTRA_KEY:  symbolKindToString(symbol.Kind),
			SYMBOL_NAME_KEY: symbol.Name,
			FILE_KEY:        u,
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
	lineNumber := int(ref.Range.Start.Line)
	incident.LineNumber = &lineNumber

	return incident, nil

}

func (p *javaServiceClient) getURI(refURI string) (uri.URI, error) {
	if !strings.HasPrefix(refURI, FILE_URI_PREFIX) {
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

	if _, err := os.Stat(javaFileAbsolutePath); err != nil {
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

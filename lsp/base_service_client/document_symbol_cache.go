package base

import (
	"regexp"
	"sync"

	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"go.lsp.dev/uri"
)

// DocumentSymbolCache stores document symbols keyed by URI and exposes helpers
// to retrieve flattened workspace symbols that match a regex.
type DocumentSymbolCache struct {
	mu     sync.RWMutex
	lookup map[uri.URI][]protocol.DocumentSymbol
}

func NewDocumentSymbolCache() *DocumentSymbolCache {
	return &DocumentSymbolCache{
		lookup: make(map[uri.URI][]protocol.DocumentSymbol),
	}
}

func (c *DocumentSymbolCache) Get(u uri.URI) ([]protocol.DocumentSymbol, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	symbols, ok := c.lookup[u]
	return symbols, ok
}

func (c *DocumentSymbolCache) Set(u uri.URI, symbols []protocol.DocumentSymbol) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lookup[u] = symbols
}

func (c *DocumentSymbolCache) Invalidate(u uri.URI) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.lookup, u)
}

func (c *DocumentSymbolCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lookup = make(map[uri.URI][]protocol.DocumentSymbol)
}

// FindWorkspaceSymbols returns workspace symbols whose names match the provided
// regex across all cached documents.
func (c *DocumentSymbolCache) FindWorkspaceSymbols(pattern *regexp.Regexp) []protocol.WorkspaceSymbol {
	if c == nil || pattern == nil {
		return nil
	}

	snapshot := c.snapshot()
	results := make([]protocol.WorkspaceSymbol, 0)

	for u, symbols := range snapshot {
		collectWorkspaceSymbols(u, symbols, pattern, &results, "")
	}

	return results
}

func (c *DocumentSymbolCache) snapshot() map[uri.URI][]protocol.DocumentSymbol {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[uri.URI][]protocol.DocumentSymbol, len(c.lookup))
	for k, v := range c.lookup {
		result[k] = v
	}
	return result
}

func collectWorkspaceSymbols(docURI uri.URI, symbols []protocol.DocumentSymbol, pattern *regexp.Regexp, out *[]protocol.WorkspaceSymbol, containerName string) {
	for _, symbol := range symbols {
		if pattern.MatchString(symbol.Name) {
			*out = append(*out, protocol.WorkspaceSymbol{
				BaseSymbolInformation: protocol.BaseSymbolInformation{
					Name:          symbol.Name,
					Kind:          symbol.Kind,
					Tags:          symbol.Tags,
					ContainerName: containerName,
				},
				Location: protocol.OrPLocation_workspace_symbol{
					Value: protocol.Location{
						URI:   protocol.DocumentURI(docURI),
						Range: preferredRange(symbol),
					},
				},
			})
		}

		if len(symbol.Children) > 0 {
			collectWorkspaceSymbols(docURI, symbol.Children, pattern, out, symbol.Name)
		}
	}
}

func preferredRange(symbol protocol.DocumentSymbol) protocol.Range {
	if symbol.SelectionRange.Start.Line != 0 ||
		symbol.SelectionRange.Start.Character != 0 ||
		symbol.SelectionRange.End.Line != 0 ||
		symbol.SelectionRange.End.Character != 0 {
		return symbol.SelectionRange
	}
	return symbol.Range
}

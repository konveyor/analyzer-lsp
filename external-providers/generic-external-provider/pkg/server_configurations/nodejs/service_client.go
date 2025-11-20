package nodejs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-logr/logr"
	base "github.com/konveyor/analyzer-lsp/lsp/base_service_client"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/swaggest/openapi-go/openapi3"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
)

var (
	// fromKeywordRegex matches "from" as a standalone keyword followed by a quote
	fromKeywordRegex = regexp.MustCompile(`\bfrom\s+["']`)
)

type NodeServiceClientConfig struct {
	base.LSPServiceClientConfig `yaml:",inline"`

	blah int `yaml:",inline"`
}

// Tidy aliases
type serviceClientFn = base.LSPServiceClientFunc[*NodeServiceClient]

type NodeServiceClient struct {
	*base.LSPServiceClientBase
	*base.LSPServiceClientEvaluator[*NodeServiceClient]

	Config NodeServiceClientConfig
}

type NodeServiceClientBuilder struct{}

func (n *NodeServiceClientBuilder) Init(ctx context.Context, log logr.Logger, c provider.InitConfig) (provider.ServiceClient, error) {
	sc := &NodeServiceClient{}

	// Unmarshal the config
	b, _ := yaml.Marshal(c.ProviderSpecificConfig)
	err := yaml.Unmarshal(b, &sc.Config)
	if err != nil {
		return nil, err
	}

	params := protocol.InitializeParams{}

	if c.Location != "" {
		sc.Config.WorkspaceFolders = []string{c.Location}
	}

	if len(sc.Config.WorkspaceFolders) == 0 {
		params.RootURI = ""
	} else {
		params.RootURI = sc.Config.WorkspaceFolders[0]
	}
	// var workspaceFolders []protocol.WorkspaceFolder
	// for _, f := range sc.Config.WorkspaceFolders {
	// 	workspaceFolders = append(workspaceFolders, protocol.WorkspaceFolder{
	// 		URI:  f,
	// 		Name: f,
	// 	})
	// }
	// params.WorkspaceFolders = workspaceFolders

	params.Capabilities = protocol.ClientCapabilities{}

	var InitializationOptions map[string]any
	err = json.Unmarshal([]byte(sc.Config.LspServerInitializationOptions), &InitializationOptions)
	if err != nil {
		// fmt.Printf("Could not unmarshal into map[string]any: %s\n", sc.Config.LspServerInitializationOptions)
		params.InitializationOptions = map[string]any{}
	} else {
		params.InitializationOptions = InitializationOptions
	}

	// Initialize the base client
	scBase, err := base.NewLSPServiceClientBase(
		ctx, log, c,
		base.LogHandler(log),
		params,
	)
	if err != nil {
		return nil, err
	}
	sc.LSPServiceClientBase = scBase

	// Synchronize BaseConfig.WorkspaceFolders with Config.WorkspaceFolders
	// This ensures consistency between the two configurations
	sc.BaseConfig.WorkspaceFolders = sc.Config.WorkspaceFolders

	// DEBUG: Log LSP initialization details
	log.Info("NodeJS LSP initialized",
		"rootURI", params.RootURI,
		"workspaceFolders", sc.Config.WorkspaceFolders,
		"lspServerPath", sc.Config.LspServerPath,
		"lspServerName", sc.Config.LspServerName)

	// Initialize the fancy evaluator (dynamic dispatch ftw)
	eval, err := base.NewLspServiceClientEvaluator[*NodeServiceClient](sc, n.GetGenericServiceClientCapabilities(log))
	if err != nil {
		return nil, err
	}
	sc.LSPServiceClientEvaluator = eval

	return sc, nil
}

func (n *NodeServiceClientBuilder) GetGenericServiceClientCapabilities(log logr.Logger) []base.LSPServiceClientCapability {
	caps := []base.LSPServiceClientCapability{}
	r := openapi3.NewReflector()
	refCap, err := provider.ToProviderCap(r, log, referencedCondition{}, "referenced")
	if err != nil {
		log.Error(err, "unable to get referenced cap")
	} else {
		caps = append(caps, base.LSPServiceClientCapability{
			Capability: refCap,
			Fn:         serviceClientFn((*NodeServiceClient).EvaluateReferenced),
		})
	}
	return caps
}

type resp = provider.ProviderEvaluateResponse

// Example condition
type referencedCondition struct {
	Referenced struct {
		Pattern string `yaml:"pattern"`
	} `yaml:"referenced"`
}

// ImportLocation tracks where a symbol is imported
type ImportLocation struct {
	FileURI  string
	LangID   string
	Position protocol.Position
	Line     string
}

// fileInfo tracks a source file
type fileInfo struct {
	path   string
	langID string
}

// EvaluateReferenced implements nodejs.referenced capability using import-based search.
//
// Algorithm:
// 1. Scans all TypeScript/JavaScript files for import statements containing the pattern
// 2. For each import found, uses LSP textDocument/definition to get the symbol's definition location
// 3. For each definition, uses LSP textDocument/references to find all usage locations
// 4. Returns deduplicated incidents for all references within the workspace
//
// This approach is much faster than workspace/symbol search and correctly handles:
// - Multiline import statements
// - Named imports: import { Card } from "package"
// - Default imports: import Card from "package"
// - Multiple symbols in one import: import { Card, CardBody } from "package"
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - cap: Capability name ("referenced")
//   - info: YAML-encoded referencedCondition with pattern to search
//
// Returns:
//   - ProviderEvaluateResponse with Matched=true and incidents if symbol is found
//   - ProviderEvaluateResponse with Matched=false if symbol is not imported anywhere
//   - Error if processing fails
func (sc *NodeServiceClient) EvaluateReferenced(ctx context.Context, cap string, info []byte) (provider.ProviderEvaluateResponse, error) {
	var cond referencedCondition
	err := yaml.Unmarshal(info, &cond)
	if err != nil {
		return resp{}, fmt.Errorf("error unmarshaling query info")
	}

	query := cond.Referenced.Pattern
	if query == "" {
		return resp{}, fmt.Errorf("unable to get query info")
	}

	// get all ts/js files
	if len(sc.Config.WorkspaceFolders) == 0 {
		return resp{}, fmt.Errorf("no workspace folders configured")
	}
	folder := strings.TrimPrefix(sc.Config.WorkspaceFolders[0], "file://")
	var nodeFiles []fileInfo
	err = filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip node_modules
		if info.IsDir() && info.Name() == "node_modules" {
			return filepath.SkipDir
		}
		if !info.IsDir() {
			ext := filepath.Ext(path)
			if ext == ".ts" || ext == ".tsx" {
				langID := "typescript"
				if ext == ".tsx" {
					langID = "typescriptreact"
				}
				path = "file://" + path
				nodeFiles = append(nodeFiles, fileInfo{path: path, langID: langID})
			}
			if ext == ".js" || ext == ".jsx" {
				langID := "javascript"
				if ext == ".jsx" {
					langID = "javascriptreact"
				}
				path = "file://" + path
				nodeFiles = append(nodeFiles, fileInfo{path: path, langID: langID})
			}
		}

		return nil
	})
	if err != nil {
		return provider.ProviderEvaluateResponse{}, err
	}

	sc.Log.Info("Scanning for import statements",
		"query", query,
		"totalFiles", len(nodeFiles))

	// FAST: Find all import statements containing the pattern (no LSP needed)
	importLocations := sc.findImportStatements(query, nodeFiles)

	if len(importLocations) == 0 {
		sc.Log.Info("No imports found for symbol",
			"query", query)
		return resp{Matched: false}, nil
	}

	sc.Log.Info("Found imports for symbol",
		"query", query,
		"importCount", len(importLocations))

	didOpen := func(uri string, langID string, text []byte) error {
		params := protocol.DidOpenTextDocumentParams{
			TextDocument: protocol.TextDocumentItem{
				URI:        uri,
				LanguageID: langID,
				Version:    0,
				Text:       string(text),
			},
		}
		return sc.Conn.Notify(ctx, "textDocument/didOpen", params)
	}

	didClose := func(uri string) error {
		params := protocol.DidCloseTextDocumentParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: uri,
			},
		}
		return sc.Conn.Notify(ctx, "textDocument/didClose", params)
	}

	var allReferences []protocol.Location
	processedDefinitions := make(map[string]bool) // Avoid processing same definition multiple times
	openedFiles := make(map[string]bool)

	// Process each import location
	for _, importLoc := range importLocations {
		// Open the file if not already open
		if !openedFiles[importLoc.FileURI] {
			trimmedURI := strings.TrimPrefix(importLoc.FileURI, "file://")
			text, err := os.ReadFile(trimmedURI)
			if err != nil {
				sc.Log.V(1).Info("Failed to read file", "file", importLoc.FileURI, "error", err)
				continue
			}

			err = didOpen(importLoc.FileURI, importLoc.LangID, text)
			if err != nil {
				sc.Log.V(1).Info("Failed to open file in LSP", "file", importLoc.FileURI, "error", err)
				continue
			}
			openedFiles[importLoc.FileURI] = true
		}

		// Get definition of the imported symbol using textDocument/definition
		// Use retry logic with exponential backoff to handle LSP indexing delays
		params := protocol.DefinitionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{
					URI: importLoc.FileURI,
				},
				Position: importLoc.Position,
			},
		}

		var definitions []protocol.Location
		var err error

		// Retry up to 3 times with exponential backoff (50ms, 100ms, 200ms)
		// This handles cases where the LSP server needs time to index newly opened files
		maxRetries := 3
		for attempt := 0; attempt < maxRetries; attempt++ {
			err = sc.Conn.Call(ctx, "textDocument/definition", params).Await(ctx, &definitions)
			if err == nil && len(definitions) > 0 {
				break
			}

			if attempt < maxRetries-1 {
				delay := time.Duration(50*(1<<uint(attempt))) * time.Millisecond
				time.Sleep(delay)
			}
		}

		if err != nil {
			sc.Log.V(1).Info("Failed to get definition",
				"file", importLoc.FileURI,
				"position", importLoc.Position,
				"error", err)
			continue
		}

		sc.Log.V(1).Info("Got definitions for import",
			"query", query,
			"file", importLoc.FileURI,
			"line", importLoc.Line,
			"definitionCount", len(definitions))

		// For each definition, get all references
		for _, def := range definitions {
			defKey := fmt.Sprintf("%s:%d:%d", def.URI, def.Range.Start.Line, def.Range.Start.Character)
			if processedDefinitions[defKey] {
				continue // Already processed this definition
			}
			processedDefinitions[defKey] = true

			sc.Log.V(1).Info("Getting references for definition",
				"definitionURI", def.URI)

			// Get all references to this definition
			references := sc.GetAllReferences(ctx, def)
			allReferences = append(allReferences, references...)

			sc.Log.V(1).Info("Got references",
				"definitionURI", def.URI,
				"referenceCount", len(references))
		}
	}

	// Close all opened files
	for fileURI := range openedFiles {
		err = didClose(fileURI)
		if err != nil {
			sc.Log.V(1).Info("Failed to close file", "file", fileURI, "error", err)
		}
	}

	// Filter references to workspace only and convert to incidents
	incidentsMap := make(map[string]provider.IncidentContext)
	for _, ref := range allReferences {
		// Only include references within the workspace
		if len(sc.BaseConfig.WorkspaceFolders) == 0 {
			continue
		}
		if !strings.Contains(ref.URI, sc.BaseConfig.WorkspaceFolders[0]) {
			continue
		}

		// Skip references in dependency folders
		skip := false
		for _, depFolder := range sc.BaseConfig.DependencyFolders {
			if depFolder != "" && strings.Contains(ref.URI, depFolder) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		u, err := uri.Parse(ref.URI)
		if err != nil {
			continue
		}

		lineNumber := int(ref.Range.Start.Line)
		incident := provider.IncidentContext{
			FileURI:    u,
			LineNumber: &lineNumber,
			Variables: map[string]interface{}{
				"file": ref.URI,
			},
			CodeLocation: &provider.Location{
				StartPosition: provider.Position{Line: float64(lineNumber)},
				EndPosition:   provider.Position{Line: float64(lineNumber)},
			},
		}
		b, _ := json.Marshal(incident)
		incidentsMap[string(b)] = incident
	}

	incidents := []provider.IncidentContext{}
	for _, incident := range incidentsMap {
		incidents = append(incidents, incident)
	}

	sc.Log.Info("nodejs.referenced import-based search complete",
		"query", query,
		"importsFound", len(importLocations),
		"definitionsProcessed", len(processedDefinitions),
		"totalReferences", len(allReferences),
		"incidentsFound", len(incidents))

	if len(incidents) == 0 {
		return resp{Matched: false}, nil
	}

	return resp{
		Matched:   true,
		Incidents: incidents,
	}, nil
}

func (sc *NodeServiceClient) EvaluateSymbols(ctx context.Context, symbols []protocol.WorkspaceSymbol) (map[string]provider.IncidentContext, error) {
	incidentsMap := make(map[string]provider.IncidentContext)

	for _, s := range symbols {
		references := sc.GetAllReferences(ctx, s.Location.Value.(protocol.Location))
		breakEarly := false
		for _, ref := range references {
			// Look for things that are in the location loaded,
			// Note may need to filter out vendor at some point
			if len(sc.BaseConfig.WorkspaceFolders) == 0 {
				continue
			}
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
				return nil, err
			}
			lineNumber := int(ref.Range.Start.Line)
			incident := provider.IncidentContext{
				FileURI:    u,
				LineNumber: &lineNumber,
				Variables: map[string]interface{}{
					"file": ref.URI,
				},
				CodeLocation: &provider.Location{
					StartPosition: provider.Position{Line: float64(lineNumber)},
					EndPosition:   provider.Position{Line: float64(lineNumber)},
				},
			}
			b, _ := json.Marshal(incident)

			incidentsMap[string(b)] = incident
		}
	}

	return incidentsMap, nil
}

// findImportStatements searches for import statements containing the given pattern.
//
// This method performs a fast regex-based search without requiring LSP, making it much
// more efficient than workspace/symbol search. It handles both named and default imports,
// and correctly processes multiline imports by normalizing them first.
//
// Supported import patterns:
//   - Named imports: import { Button, Card } from "@patternfly/react-core"
//   - Default imports: import Button from "@patternfly/react-core"
//   - Multiline imports: import {\n  Button,\n  Card\n} from "..."
//
// Parameters:
//   - pattern: The symbol name to search for (e.g., "Button", "Card")
//   - files: List of TypeScript/JavaScript files to search
//
// Returns:
//   - Slice of ImportLocation structs containing file URI, language ID, and position
//     where the symbol appears in the import statement
//
// The returned locations can be used with LSP textDocument/definition to find where
// the symbol is actually defined, enabling efficient reference lookup.
func (sc *NodeServiceClient) findImportStatements(pattern string, files []fileInfo) []ImportLocation {
	// Regex to match: import { Pattern, ... } from "package"
	// Captures both named imports and default imports
	importRegex := regexp.MustCompile(
		`import\s+(?:\{([^}]*)\}|(\w+))\s+from\s+['"]([^'"]+)['"]`,
	)

	var locations []ImportLocation

	for _, file := range files {
		trimmedPath := strings.TrimPrefix(file.path, "file://")
		content, err := os.ReadFile(trimmedPath)
		if err != nil {
			continue
		}

		lines := strings.Split(string(content), "\n")

		// Normalize multiline imports to single line for regex matching
		normalized := sc.normalizeMultilineImports(string(content))

		// Find all imports in the normalized content
		allMatches := importRegex.FindAllStringSubmatchIndex(normalized, -1)

		for _, matchIdx := range allMatches {
			if len(matchIdx) < 4 {
				continue
			}

			var namedImports string
			var defaultImport string

			// Extract named imports (group 1)
			if matchIdx[2] != -1 && matchIdx[3] != -1 {
				namedImports = normalized[matchIdx[2]:matchIdx[3]]
			}

			// Extract default import (group 2)
			if matchIdx[4] != -1 && matchIdx[5] != -1 {
				defaultImport = normalized[matchIdx[4]:matchIdx[5]]
			}

			// Check if pattern appears in named imports or is the default import
			patternFound := false
			if namedImports != "" && strings.Contains(namedImports, pattern) {
				patternFound = true
			} else if defaultImport != "" && defaultImport == pattern {
				patternFound = true
			}

			if !patternFound {
				continue
			}

			// Find the pattern in the original (non-normalized) content
			// We need to search for it starting near the import statement
			importStart := matchIdx[0]

			// Find which line this corresponds to in the original content
			charCount := 0
			for lineNum, line := range lines {
				if charCount <= importStart && charCount+len(line)+1 > importStart {
					// This line (or nearby lines) contains the import
					// Search for the pattern in this and subsequent lines
					for searchLine := lineNum; searchLine < len(lines) && searchLine < lineNum+20; searchLine++ {
						searchContent := lines[searchLine]

						// Look for the pattern as a complete word
						patternPos := strings.Index(searchContent, pattern)
						if patternPos != -1 {
							// Verify it's a complete word (not part of a larger identifier)
							isWordStart := patternPos == 0 || !isIdentifierChar(rune(searchContent[patternPos-1]))
							isWordEnd := patternPos+len(pattern) >= len(searchContent) || !isIdentifierChar(rune(searchContent[patternPos+len(pattern)]))

							if isWordStart && isWordEnd {
								locations = append(locations, ImportLocation{
									FileURI:  file.path,
									LangID:   file.langID,
									Position: protocol.Position{
										Line:      uint32(searchLine),
										Character: uint32(patternPos),
									},
									Line: searchContent,
								})
								break
							}
						}
					}
					break
				}
				charCount += len(line) + 1 // +1 for newline
			}
		}
	}

	return locations
}

// normalizeMultilineImports converts multiline import statements to single lines.
//
// This preprocessing step allows the import regex to match imports that span multiple
// lines, which is common in formatted TypeScript/JavaScript code.
//
// Example transformation:
//   Before: import {\n  Card,\n  CardBody\n} from "..."
//   After:  import { Card, CardBody } from "..."
//
// The method preserves:
//   - String literals (quoted strings are not modified)
//   - Brace depth tracking (handles nested structures)
//   - Semicolons and statement boundaries
//
// Edge cases handled:
//   - The word "import" within larger identifiers (e.g., "myimport") is not matched
//   - Escape sequences in strings are preserved
//   - Both single and double quoted strings are supported
//
// Parameters:
//   - content: Source file content as a string
//
// Returns:
//   - Normalized content with multiline imports converted to single lines
func (sc *NodeServiceClient) normalizeMultilineImports(content string) string {
	var result strings.Builder
	result.Grow(len(content))

	i := 0
	for i < len(content) {
		// Look for "import" keyword
		if i+6 <= len(content) && content[i:i+6] == "import" {
			// Check if this is actually the start of an import statement
			// (not part of a larger word)
			if i > 0 && isIdentifierChar(rune(content[i-1])) {
				result.WriteByte(content[i])
				i++
				continue
			}

			// Start of import statement - find the end
			importStart := i
			braceDepth := 0
			inString := false
			stringChar := byte(0)

			// Copy "import" and continue
			result.WriteString("import")
			i += 6

			// Process until we find the end of the import statement
			for i < len(content) {
				ch := content[i]

				// Handle strings
				if !inString && (ch == '"' || ch == '\'' || ch == '`') {
					inString = true
					stringChar = ch
					result.WriteByte(ch)
					i++
					continue
				} else if inString && ch == stringChar {
					// Count preceding backslashes to determine if quote is escaped
					escapeCount := 0
					for j := i - 1; j >= 0 && content[j] == '\\'; j-- {
						escapeCount++
					}
					// If even number of backslashes (including 0), quote is not escaped
					if escapeCount%2 == 0 {
						inString = false
					}
					result.WriteByte(ch)
					i++
					continue
				} else if inString {
					result.WriteByte(ch)
					i++
					continue
				}

				// Track braces (not in strings)
				if ch == '{' {
					braceDepth++
					result.WriteByte(ch)
					i++
				} else if ch == '}' {
					braceDepth--
					result.WriteByte(ch)
					i++
				} else if ch == '\n' || ch == '\r' {
					// Replace newlines with spaces, but check if this import is complete
					if braceDepth == 0 && i > importStart+6 {
						// Restrict "from" detection to just the current import statement
						snippet := content[importStart:min(i+1, len(content))]
						if len(snippet) > 10 {
							start := len(snippet) - min(50, len(snippet))
							last50 := snippet[start:]
							// Match "from" as a standalone word followed by a quote
							if fromKeywordRegex.MatchString(last50) {
								// Import statement is complete
								result.WriteByte('\n')
								i++
								break
							}
						}
					}
					result.WriteByte(' ')
					i++
				} else if ch == ';' {
					result.WriteByte(ch)
					i++
					// Semicolon ends the import
					break
				} else {
					result.WriteByte(ch)
					i++
				}
			}
		} else {
			result.WriteByte(content[i])
			i++
		}
	}

	return result.String()
}

// isIdentifierChar checks if a character can be part of a JavaScript identifier
func isIdentifierChar(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '$'
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GetDependencies returns an empty dependency map as the nodejs provider
// does not use external dependency providers. This overrides the base
// implementation which would return "dependency provider path not set" error.
func (sc *NodeServiceClient) GetDependencies(ctx context.Context) (map[uri.URI][]*provider.Dep, error) {
	return map[uri.URI][]*provider.Dep{}, nil
}

// GetDependenciesDAG returns an empty dependency DAG as the nodejs provider
// does not use external dependency providers.
func (sc *NodeServiceClient) GetDependenciesDAG(ctx context.Context) (map[uri.URI][]provider.DepDAGItem, error) {
	return map[uri.URI][]provider.DepDAGItem{}, nil
}

// Test helper functions - these expose private functions for unit testing

// FileInfo is exported for testing
type FileInfo struct {
	Path   string
	LangID string
}

// NormalizeMultilineImportsPublic exposes normalizeMultilineImports for testing
func (sc *NodeServiceClient) NormalizeMultilineImportsPublic(content string) string {
	return sc.normalizeMultilineImports(content)
}

// FindImportStatementsPublic exposes findImportStatements for testing
func (sc *NodeServiceClient) FindImportStatementsPublic(pattern string, files []FileInfo) []ImportLocation {
	// Convert FileInfo to fileInfo
	internalFiles := make([]fileInfo, len(files))
	for i, f := range files {
		internalFiles[i] = fileInfo{
			path:   f.Path,
			langID: f.LangID,
		}
	}
	return sc.findImportStatements(pattern, internalFiles)
}

// IsIdentifierCharPublic exposes isIdentifierChar for testing
func (sc *NodeServiceClient) IsIdentifierCharPublic(ch rune) bool {
	return isIdentifierChar(ch)
}

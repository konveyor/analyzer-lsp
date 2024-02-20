package protocol

import (
	"fmt"
)

// Whether the server supports the given method or not. Servers, such as pylsp,
// crash when given a method they do not support.
// FIXME: Implement all requests!
// TODO: Evaluate tradeoffs of turning this into a map?
func (c *ServerCapabilities) Supports(method string) bool {
	switch method {
	// case "$/cancelRequest":
	// case "$/logTrace":
	// case "$/progress":
	// case "$/setTrace":
	// case "callHierarchy/incomingCalls":
	// case "callHierarchy/outgoingCalls":
	// case "client/registerCapability":
	// case "client/unregisterCapability":
	// case "codeAction/resolve":
	// case "codeLens/resolve":
	// case "completionItem/resolve":
	// case "documentLink/resolve":
	// case "exit":
	// case "initialize":
	// case "initialized":
	// case "inlayHint/resolve":
	// case "notebookDocument/didChange":
	// case "notebookDocument/didClose":
	// case "notebookDocument/didOpen":
	// case "notebookDocument/didSave":
	// case "shutdown":
	// case "telemetry/event":
	// case "textDocument/codeAction":
	// case "textDocument/codeLens":
	// case "textDocument/colorPresentation":
	// case "textDocument/completion":
	case "textDocument/declaration":
		t := c.DeclarationProvider
		if t == nil {
			return false
		}

		switch x := t.Value.(type) {
		case DeclarationOptions:
			return true
		case DeclarationRegistrationOptions:
			return true
		case bool:
			return x
		case nil:
			return false
		}
	case "textDocument/definition":
		t := c.DefinitionProvider
		if t == nil {
			return false
		}

		switch x := t.Value.(type) {
		case DefinitionOptions:
			return true
		case bool:
			return x
		case nil:
			return false
		}
	// case "textDocument/diagnostic":
	// case "textDocument/didChange":
	// case "textDocument/didClose":
	// case "textDocument/didOpen":
	// case "textDocument/didSave":
	// case "textDocument/documentColor":
	// case "textDocument/documentHighlight":
	// case "textDocument/documentLink":
	case "textDocument/documentSymbol":
		t := c.DocumentSymbolProvider
		if t == nil {
			return false
		}

		switch x := t.Value.(type) {
		case DocumentSymbolOptions:
			return true
		case bool:
			return x
		case nil:
			return false
		}
	// case "textDocument/foldingRange":
	// case "textDocument/formatting":
	// case "textDocument/hover":
	// case "textDocument/implementation":
	// case "textDocument/inlayHint":
	// case "textDocument/inlineCompletion":
	// case "textDocument/inlineValue":
	// case "textDocument/linkedEditingRange":
	// case "textDocument/moniker":
	// case "textDocument/onTypeFormatting":
	// case "textDocument/prepareCallHierarchy":
	// case "textDocument/prepareRename":
	// case "textDocument/prepareTypeHierarchy":
	// case "textDocument/publishDiagnostics":
	// case "textDocument/rangeFormatting":
	// case "textDocument/rangesFormatting":
	// case "textDocument/references":
	// case "textDocument/rename":
	// case "textDocument/selectionRange":
	// case "textDocument/semanticTokens/full":
	// case "textDocument/semanticTokens/full/delta":
	// case "textDocument/semanticTokens/range":
	// case "textDocument/signatureHelp":
	// case "textDocument/typeDefinition":
	// case "textDocument/willSave":
	// case "textDocument/willSaveWaitUntil":
	// case "typeHierarchy/subtypes":
	// case "typeHierarchy/supertypes":
	// case "window/logMessage":
	// case "window/showDocument":
	// case "window/showMessage":
	// case "window/showMessageRequest":
	// case "window/workDoneProgress/cancel":
	// case "window/workDoneProgress/create":
	// case "workspace/applyEdit":
	// case "workspace/codeLens/refresh":
	// case "workspace/configuration":
	// case "workspace/diagnostic":
	// case "workspace/diagnostic/refresh":
	// case "workspace/didChangeConfiguration":
	// case "workspace/didChangeWatchedFiles":
	// case "workspace/didChangeWorkspaceFolders":
	// case "workspace/didCreateFiles":
	// case "workspace/didDeleteFiles":
	// case "workspace/didRenameFiles":
	// case "workspace/executeCommand":
	// case "workspace/inlayHint/refresh":
	// case "workspace/inlineValue/refresh":
	// case "workspace/semanticTokens/refresh":
	case "workspace/symbol":
		t := c.WorkspaceSymbolProvider
		if t == nil {
			return false
		}

		switch x := t.Value.(type) {
		case WorkspaceSymbolOptions:
			return true
		case bool:
			return x
		case nil:
			return false
		}

		// case "workspace/willCreateFiles":
		// case "workspace/willDeleteFiles":
		// case "workspace/willRenameFiles":
		// case "workspace/workspaceFolders":
		// case "workspaceSymbol/resolve":
	}

	// Default to false for now. May cause headaches until all request checks are
	// implemented unfortunately.
	fmt.Printf("Method `%s` is not supported!\n", method)
	return false
}

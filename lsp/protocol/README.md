# Protocol generator tool

Originally from: https://github.com/golang/tools/tree/master/gopls/internal/lsp/protocol
Commit id: If4c85191760baef916911130ca315773d2adda1f

## How to use

If there is an update to the LSP specification, navigate to the `protocol` directory and run `go generate`. This will generate all the types needed for the analyzer.  You need to add `ExtendedClientCapilities map[string]interface{} `json:"extendedClientCapabilities"`` to XInitializeParams struct. Other than that, you should be good to go.

## Changes

- Created `extensions.go`
  - Need to implement `Supports` method fully.
- Commented out `writeclient()` and `writeserver()` in generate/main.go
- Commented out `"Or_WorkspaceFoldersServerCapabilities_changeNotifications": "string",` in generate/tables.go
- Changed `type DocumentURI string` to `type DocumentURI = string` in generate/output.go
- Need to add `ExtendedClientCapilities map[string]interface{} `json:"extendedClientCapabilities"`` to XInitializeParams struct. Need to figure out why needed. Can remove if not.

Using gopls as the reference implementation to ensure feature parity across lsp servers.

[settings.md](https://github.com/golang/tools/blob/ecbfa885b278478686e8b8efb52535e934c53ec5/gopls/doc/settings.md) is very useful

The `workspace/symbol` method returns all the top-level declarations that match the given query.

References (each one builds off the next):

- `workspace/symbol` calls this [WorkspaceSymbol function](https://github.com/golang/tools/blob/ecbfa885b278478686e8b8efb52535e934c53ec5/gopls/internal/lsp/source/workspace_symbol.go#L54)
- Which calls [collectSymbols](https://github.com/golang/tools/blob/ecbfa885b278478686e8b8efb52535e934c53ec5/gopls/internal/lsp/source/workspace_symbol.go#L299)
- Which calls [snapshot.Symbols](https://github.com/golang/tools/blob/master/gopls/internal/lsp/cache/snapshot.go#L1109) which calls [symbolize](https://github.com/golang/tools/blob/master/gopls/internal/lsp/cache/symbols.go#L21)
- Which calls [symbolizeImpl](https://github.com/golang/tools/blob/ecbfa885b278478686e8b8efb52535e934c53ec5/gopls/internal/lsp/cache/symbols.go#L72), which after parsing the file, only takes the Decls property of the ast.File, [which are the top-level declarations](https://pkg.go.dev/go/ast#File)
  - https://github.com/golang/tools/blob/master/gopls/internal/lsp/cache/parse.go#L59
- https://github.com/golang/tools/blob/master/gopls/internal/lsp/cache/symbols.go#L115
- https://pkg.go.dev/go/ast#File

symbolScope controls which packages are searched for workspace/symbol requests.
Default: `"all"` matches symbols in any loaded package, including dependencies.

symbolMatcher sets the algorithm that is used when finding workspace symbols.
Default: `"FastFuzzy"`.
- https://github.com/golang/tools/blob/ecbfa885b278478686e8b8efb52535e934c53ec5/gopls/internal/lsp/source/workspace_symbol.go#L160
-

symbolStyle controls how symbols are qualified in symbol responses.
Default: `"Dynamic"` uses whichever qualifier results in the highest scoring
match for the given symbol query. Here a "qualifier" is any "/" or "."
delimited suffix of the fully qualified symbol. i.e. "to/pkg.Foo.Field" or
just "Foo.Field".
- https://github.com/golang/tools/blob/ecbfa885b278478686e8b8efb52535e934c53ec5/gopls/internal/lsp/source/workspace_symbol.go#L99


`workspace/didChangeConfiguration` to change settings


https://jedi.readthedocs.io/en/latest/docs/api.html#jedi.Project.search

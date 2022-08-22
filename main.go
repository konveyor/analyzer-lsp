package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"path/filepath"
	"strings"

	"github.com/shawn-hurley/jsonrpc-golang/jsonrpc2"
	"github.com/shawn-hurley/jsonrpc-golang/lsp/protocol"
	"github.com/shawn-hurley/jsonrpc-golang/rules"
)

//TODO(shawn-hurley) - this needs to be passed in as args. Will need to refactor to use cobra
var config = rules.Configuration{
	ProjectLocation: "./examples/golang",
}

//TODO(shawn-hurley) - this package/type name stutters.
var workspaceRules = []rules.Rule{
	{
		ImportRule: &rules.ImportRule{
			GoImportRule: &rules.GoImportRule{
				Import: "pkg/apis/apiextensions/v1beta1.CustomResourceDefinition",
				// TODO(shawn-hurley) - copy the windup ability to intersparse known text here.
				Message: "Use of deprecated and removed API",
			},
		},
	},
}

func main() {
	ctx := context.Background()
	conn, err := net.Dial("tcp", "localhost:37374")
	if err != nil {
		fmt.Printf("error: %v", err)
	}
	rpc := jsonrpc2.NewConn(jsonrpc2.NewHeaderStream(conn, conn))

	go func() {
		err := rpc.Run(ctx)
		if err != nil {
			log.Printf("connection terminated: %v", err)
		}
	}()

	absolutePath, err := filepath.Abs(config.ProjectLocation)
	if err != nil {
		return
	}

	params := &protocol.InitializeParams{
		//TODO(shawn-hurley): add ability to parse path to URI in a real supported way
		RootURI: fmt.Sprintf("file://%v", absolutePath),
		Capabilities: protocol.ClientCapabilities{
			TextDocument: protocol.TextDocumentClientCapabilities{
				DocumentSymbol: &protocol.DocumentSymbolClientCapabilities{
					HierarchicalDocumentSymbolSupport: true,
				},
			},
		},
	}

	var result protocol.InitializeResult
	if err := rpc.Call(ctx, "initialize", params, &result); err != nil {
		fmt.Printf("initialize failed: %v", err)
	}
	if err := rpc.Notify(ctx, "initialized", &protocol.InitializedParams{}); err != nil {
		fmt.Printf("initialized failed: %v", err)
	}
	fmt.Printf("connection initialized: %#v", result.Capabilities.WorkspaceSymbolProvider)

	for _, r := range workspaceRules {
		if r.GoImportRule != nil {
			if loc, found := findReferences(ctx, rpc, absolutePath, r.GoImportRule.Import); found {
				fmt.Printf("\n%v\nlocation: %v:%v", r.GoImportRule.Message, loc.URI, loc.Range.Start.Line)
			}
		}

	}
}

func findReferences(ctx context.Context, rpc *jsonrpc2.Conn, rootDir, query string) (protocol.Location, bool) {
	wsp := &protocol.WorkspaceSymbolParams{
		Query: query,
	}

	var refs []protocol.WorkspaceSymbol
	err := rpc.Call(ctx, "workspace/symbol", wsp, &refs)
	if err != nil {
		fmt.Printf("error: %v", err)
	}

	for _, r := range refs {
		params := &protocol.ReferenceParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{
					URI: r.Location.URI,
				},
				Position: r.Location.Range.Start,
			},
		}

		res := []protocol.Location{}
		err := rpc.Call(ctx, "textDocument/references", params, &res)
		if err != nil {
			fmt.Printf("Error rpc: %v", err)
		}
		for _, result := range res {
			if strings.Contains(result.URI, rootDir) {
				return result, true
			}
		}
	}
	return protocol.Location{}, false
}

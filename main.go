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
)

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

	d, err := filepath.Abs("../test-analyziers-gopls")
	fmt.Printf("repo: %v", d)
	if err != nil {
		return
	}

	params := &protocol.InitializeParams{
		RootURI: fmt.Sprintf("file://%v", d),
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
	wsp := &protocol.WorkspaceSymbolParams{
		Query: "pkg/apis/apiextensions/v1beta1.CustomResourceDefinition",
	}

	var res []protocol.WorkspaceSymbol
	err = rpc.Call(ctx, "workspace/symbol", wsp, &res)
	if err != nil {
		fmt.Printf("error: %v", err)
	}

	refs := []protocol.WorkspaceSymbol{}
	for _, s := range res {
		if s.Kind == protocol.Struct {
			refs = append(refs, s)
		}
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
			if strings.Contains(result.URI, d) {
				fmt.Printf("\nFound references to type - %v:%v\n", result.URI, result.Range.Start.Line)
			}
		}
	}
}

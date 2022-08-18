package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"path/filepath"

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
	fmt.Printf("connection initialized: %#v", result)
	wsp := &protocol.WorkspaceSymbolParams{
		Query: "pkg/apis/apiextensions/v1beta1.CustomResourceDefinition",
	}

	var res []protocol.WorkspaceSymbol
	err = rpc.Call(ctx, "workspace/symbol", wsp, &res)
	if err != nil {
		fmt.Printf("error: %v", err)
	}

	for _, s := range res {
		fmt.Printf("\n%v - %v/%v\n", s.Location.URI, s.Kind, s.Name)
	}
}

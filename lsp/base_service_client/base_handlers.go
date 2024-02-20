package base

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	jsonrpc2 "github.com/konveyor/analyzer-lsp/jsonrpc2_v2"
)

// Default handler always returns jsonrpc2.ErrNotHandled for every request.
type DefaultHandler struct{}

// Returns jsonrpc2.ErrNotHandled for every request.
func (*DefaultHandler) Handle(context.Context, *jsonrpc2.Request) (interface{}, error) {
	return nil, jsonrpc2.ErrNotHandled
}

// Logs the requests received
func LogHandler(log logr.Logger) jsonrpc2.HandlerFunc {
	return func(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
		log.V(5).Info("Request received", "Method", req.Method)
		fmt.Printf(
			"Request received:\n- ID: %v\n- Method: %s\n- Params: %s\n\n",
			req.ID,
			req.Method,
			string(req.Params),
		)
		return nil, jsonrpc2.ErrNotHandled
	}
}

// Executes the Handlers one after the other, back to front stack-like. Returns
// the first response that has error == nil
type ChainHandler struct {
	Handlers []jsonrpc2.Handler
}

// Create a new ChainHandler with auto-flattening
func NewChainHandler(handlers ...jsonrpc2.Handler) *ChainHandler {
	output := ChainHandler{}

	for _, h := range handlers {
		if ch, ok := h.(*ChainHandler); ok {
			output.Handlers = append(output.Handlers, ch.Handlers...)
		} else {
			output.Handlers = append(output.Handlers, h)
		}
	}

	return &output
}

// Executes the Handlers one after the other, back to front stack-like. Returns
// the first response that has error == nil
func (ch *ChainHandler) Handle(ctx context.Context, req *jsonrpc2.Request) (result interface{}, err error) {
	for i := len(ch.Handlers) - 1; i >= 0; i-- {
		result, err = ch.Handlers[i].Handle(ctx, req)
		if err == nil {
			return result, nil
		}
	}

	return nil, jsonrpc2.ErrNotHandled
}

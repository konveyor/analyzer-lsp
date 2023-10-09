// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package jsonrpc2 is a minimal implementation of the JSON RPC 2 spec.
// https://www.jsonrpc.org/specification
// It is intended to be compatible with other implementations at the wire level.
package jsonrpc2

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/go-logr/logr"
)

// Conn is a JSON RPC 2 client server connection.
// Conn is bidirectional; it does not have a designated server or client end.
type Conn struct {
	seq       int64 // must only be accessed using atomic operations
	handlers  []Handler
	stream    Stream
	pendingMu sync.Mutex // protects the pending map
	pending   map[ID]chan *WireResponse
	logger    logr.Logger
}

// NewErrorf builds a Error struct for the supplied message and code.
// If args is not empty, message and args will be passed to Sprintf.
func NewErrorf(code int64, format string, args ...interface{}) *Error {
	return &Error{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}
}

// NewConn creates a new connection object around the supplied stream.
// You must call Run for the connection to be active.
func NewConn(s Stream, log logr.Logger) *Conn {
	conn := &Conn{
		handlers: []Handler{defaultHandler{}},
		stream:   s,
		pending:  make(map[ID]chan *WireResponse),
		logger:   log,
	}
	return conn
}

// AddHandler adds a new handler to the set the connection will invoke.
// Handlers are invoked in the reverse order of how they were added, this
// allows the most recent addition to be the first one to attempt to handle a
// message.
func (c *Conn) AddHandler(handler Handler) {
	// prepend the new handlers so we use them first
	c.handlers = append([]Handler{handler}, c.handlers...)
}

// Notify is called to send a notification request over the connection.
// It will return as soon as the notification has been sent, as no response is
// possible.
func (c *Conn) Notify(ctx context.Context, method string, params interface{}) (err error) {
	jsonParams, err := marshalToRaw(params)
	if err != nil {
		return fmt.Errorf("marshalling notify parameters: %v", err)
	}
	request := &WireRequest{
		Method: method,
		Params: jsonParams,
	}
	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshalling notify request: %v", err)
	}
	for _, h := range c.handlers {
		ctx = h.Request(ctx, c, Send, request)
	}
	defer func() {
		for _, h := range c.handlers {
			h.Done(ctx, err)
		}
	}()

	n, err := c.stream.Write(ctx, data)
	for _, h := range c.handlers {
		ctx = h.Wrote(ctx, n)
	}
	return err
}

type RPCUnmarshalError struct {
	Json string
	Err  error
}

func (e *RPCUnmarshalError) Error() string {
	return fmt.Sprintf("tried to unmarshal: %v\ngot error: %v", e.Json, e.Err)
}

// Call sends a request over the connection and then waits for a response.
// If the response is not an error, it will be decoded into result.
// result must be of a type you an pass to json.Unmarshal.
func (c *Conn) Call(ctx context.Context, method string, params, result interface{}) (err error) {
	// generate a new request identifier
	id := ID{Number: atomic.AddInt64(&c.seq, 1)}
	jsonParams, err := marshalToRaw(params)
	if err != nil {
		return fmt.Errorf("marshalling call parameters: %v", err)
	}
	request := &WireRequest{
		ID:     &id,
		Method: method,
		Params: jsonParams,
	}
	// marshal the request now it is complete
	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshalling call request: %v", err)
	}
	for _, h := range c.handlers {
		ctx = h.Request(ctx, c, Send, request)
	}
	// we have to add ourselves to the pending map before we send, otherwise we
	// are racing the response
	rchan := make(chan *WireResponse)
	c.pendingMu.Lock()
	c.pending[id] = rchan
	c.pendingMu.Unlock()
	defer func() {
		// clean up the pending response handler on the way out
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		for _, h := range c.handlers {
			h.Done(ctx, err)
		}
	}()

	// now we are ready to send
	n, err := c.stream.Write(ctx, data)
	for _, h := range c.handlers {
		ctx = h.Wrote(ctx, n)
	}
	if err != nil {
		// sending failed, we will never get a response, so don't leave it pending
		return err
	}
	// now wait for the response
	select {
	case response := <-rchan:
		for _, h := range c.handlers {
			ctx = h.Response(ctx, c, Receive, response)
		}
		// is it an error response?
		if response.Error != nil {
			return response.Error
		}
		if result == nil || response.Result == nil {
			return nil
		}

		if err := json.Unmarshal(*response.Result, result); err != nil {
			return &RPCUnmarshalError{string(*response.Result), err}
		}
		return nil
	case <-ctx.Done():
		// allow the handler to propagate the cancel
		cancelled := false
		for _, h := range c.handlers {
			if h.Cancel(ctx, c, id, cancelled) {
				cancelled = true
			}
		}
		return ctx.Err()
	}
}

// combined has all the fields of both Request and Response.
// We can decode this and then work out which it is.
type combined struct {
	VersionTag VersionTag       `json:"jsonrpc"`
	ID         *ID              `json:"id,omitempty"`
	Method     string           `json:"method"`
	Params     *json.RawMessage `json:"params,omitempty"`
	Result     *json.RawMessage `json:"result,omitempty"`
	Error      *Error           `json:"error,omitempty"`
}

// Run blocks until the connection is terminated, and returns any error that
// caused the termination.
// It must be called exactly once for each Conn.
// It returns only when the reader is closed or there is an error in the stream.
func (c *Conn) Run(runCtx context.Context) error {
	// we need to make the next request "lock" in an unlocked state to allow
	// the first incoming request to proceed. All later requests are unlocked
	// by the preceding request going to parallel mode.
	c.logger.V(5).Info("starting to run rpc connection")
	nextRequest := make(chan struct{})
	close(nextRequest)
	for {
		// get the data for a message
		data, _, err := c.stream.Read(runCtx)
		if err != nil {
			// the stream failed, we cannot continue
			return err
		}
		// read a combined message
		msg := &combined{}
		if err := json.Unmarshal(data, msg); err != nil {
			// a badly formed message arrived, log it and continue
			// we trust the stream to have isolated the error to just this message
			for _, h := range c.handlers {
				h.Error(runCtx, fmt.Errorf("unmarshal failed: %v", err))
			}
			continue
		}
		// work out which kind of message we have
		switch {
		case msg.ID != nil:
			// we have a response, get the pending entry from the map
			c.pendingMu.Lock()
			rchan, ok := c.pending[*msg.ID]
			if rchan != nil {
				delete(c.pending, *msg.ID)
			}
			c.pendingMu.Unlock()
			// and send the reply to the channel
			response := &WireResponse{
				Result: msg.Result,
				Error:  msg.Error,
				ID:     msg.ID,
			}

			// yaml-language-server sends back a request with an ID
			if ok {
				rchan <- response
				close(rchan)
			}
		default:
			for _, h := range c.handlers {
				h.Error(runCtx, fmt.Errorf("message not a call, notify or response, ignoring"))
			}
		}
	}
}

func marshalToRaw(obj interface{}) (*json.RawMessage, error) {
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	raw := json.RawMessage(data)
	return &raw, nil
}

package base

import (
	"context"
	"math"
	"sync"
	"time"

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
		log.V(5).Info("Request received", "Method", req.Method, "Id", req.ID, "params", req.Params)
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

type BackoffHandler struct {
	failedRequests   map[requestKey]*backoffTimer
	failedRequestsMu sync.Mutex
	logger           logr.Logger
}

func NewBackoffHandler(log logr.Logger) jsonrpc2.Handler {
	return &BackoffHandler{
		failedRequests:   make(map[requestKey]*backoffTimer),
		failedRequestsMu: sync.Mutex{},
		logger:           log,
	}
}

type requestKey struct {
	method string
	params string
}

type backoffTimer struct {
	retries           float64
	lastAttemptedTime *time.Time
	lastDurationTime  time.Duration
}

func (b *backoffTimer) BackoffRequest() time.Duration {
	if b.lastAttemptedTime == nil {
		t := time.Now()
		b.lastAttemptedTime = &t
		b.lastDurationTime = time.Duration(0)
		return b.lastDurationTime
	}

	// if backoff exists but has been more than a minute passed the
	// the last back off, then reset the backoff.
	if time.Now().After(b.lastAttemptedTime.Add(b.lastDurationTime).Add(time.Minute * 1)) {
		b.retries = 0
		t := time.Now()
		b.lastAttemptedTime = &t
		b.lastDurationTime = time.Duration(0)
		return b.lastDurationTime
	}

	// calculated back off
	b.lastDurationTime = time.Second * time.Duration((math.Pow(2, b.retries)))

	// Cap back off at 5 min.
	if b.lastDurationTime >= (time.Minute * 5) {
		b.lastDurationTime = time.Minute * 5
	}

	b.retries = b.retries + 1
	return b.lastDurationTime
}

// Log is invoked for all messages flowing through a Conn.
// direction indicates if the message being received or sent
// id is the message id, if not set it was a notification
// elapsed is the time between a call being seen and the response, and is
// negative for anything that is not a response.
// method is the method name specified in the message
// payload is the parameters for a call or notification, and the result for a
// response
// Request is called near the start of processing any request.
func (b *BackoffHandler) Handle(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	//handle Back off
	requestKey := requestKey{
		method: req.Method,
		params: string(req.Params),
	}
	b.failedRequestsMu.Lock()
	backOff, ok := b.failedRequests[requestKey]
	if !ok {
		backOff = &backoffTimer{
			retries:           0,
			lastAttemptedTime: nil,
			lastDurationTime:  0,
		}
		b.failedRequests[requestKey] = backOff
	}
	b.failedRequestsMu.Unlock()

	d := backOff.BackoffRequest()
	b.logger.V(9).Info("starting backing off request", "method", req.Method, "duration", d)
	time.Sleep(d)
	b.logger.V(9).Info("stopping backing off request", "method", req.Method)
	return nil, jsonrpc2.ErrNotHandled
}

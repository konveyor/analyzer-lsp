package jsonrpc2

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/go-logr/logr"
)

type BackoffHandler struct {
	failedRequests   map[requestKey]*backoffTimer
	failedRequestsMu sync.Mutex
	logger           logr.Logger
}

func NewBackoffHandler(log logr.Logger) *BackoffHandler {
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

var _ Handler = &BackoffHandler{}

// Cancel is invoked for cancelled outgoing requests.
// It is okay to use the connection to send notifications, but the context will
// be in the cancelled state, so you must do it with the background context
// instead.
// If Cancel returns true all subsequent handlers will be invoked with
// cancelled set to true, and should not attempt to cancel the message.
func (b *BackoffHandler) Cancel(ctx context.Context, conn *Conn, id ID, cancelled bool) bool {
	return cancelled
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
func (b *BackoffHandler) Request(ctx context.Context, conn *Conn, direction Direction, r *WireRequest) context.Context {
	//handle Back off
	requestKey := requestKey{
		method: r.Method,
		params: string(*r.Params),
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
	b.logger.V(9).Info("starting backing off request", "method", r.Method, "duration", d)
	time.Sleep(d)
	b.logger.V(9).Info("stopping backing off request", "method", r.Method)
	return context.WithValue(ctx, "back-off-timer", requestKey)
}

// Response is called near the start of processing any response.
func (b *BackoffHandler) Response(ctx context.Context, conn *Conn, direction Direction, r *WireResponse) context.Context {
	return ctx
}

// Done is called when any request is fully processed.
// For calls, this means the response has also been processed, for notifies
// this is as soon as the message has been written to the stream.
// If err is set, it implies the request failed.
func (b *BackoffHandler) Done(ctx context.Context, err error) {
	requestKey, ok := ctx.Value("back-off-timer").(requestKey)
	if !ok {
		return
	}

	b.failedRequestsMu.Lock()
	// handle clean up in back off if we need to .
	if err == nil {
		b.logger.V(7).Info("deleting request key")
		delete(b.failedRequests, requestKey)
	}
	b.failedRequestsMu.Unlock()
}

// Read is called with a count each time some data is read from the stream.
// The read calls are delayed until after the data has been interpreted so
// that it can be attributed to a request/response.
func (b *BackoffHandler) Read(ctx context.Context, bytes int64) context.Context {
	return ctx
}

// Wrote is called each time some data is written to the stream.
func (b *BackoffHandler) Wrote(ctx context.Context, bytes int64) context.Context {
	return ctx
}

// Error is called with errors that cannot be delivered through the normal
// mechanisms, for instance a failure to process a notify cannot be delivered
// back to the other party.
func (b *BackoffHandler) Error(ctx context.Context, err error) {
}

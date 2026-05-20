package java

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"
	jsonrpc2 "github.com/konveyor/analyzer-lsp/jsonrpc2_v2"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
)

func TestHandlerWorkDoneProgressCreate(t *testing.T) {
	log := testr.New(t)
	ready := make(chan struct{})
	h := newJDTLSProgressHandler(log, ready)

	// window/workDoneProgress/create is a request (has ID) that must return nil, nil
	req, err := jsonrpc2.NewCall(jsonrpc2.StringID("1"), "window/workDoneProgress/create",
		protocol.WorkDoneProgressCreateParams{Token: "test-token"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := h.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result, got: %v", result)
	}
}

func TestHandlerProgressBeginAndEnd(t *testing.T) {
	log := testr.New(t)
	ready := make(chan struct{})
	h := newJDTLSProgressHandler(log, ready)

	ctx := context.Background()

	// Send a begin notification
	beginReq, _ := jsonrpc2.NewNotification("$/progress", protocol.ProgressParams{
		Token: "import-token",
		Value: protocol.WorkDoneProgressBegin{
			Kind:  "begin",
			Title: "Importing Maven project(s)",
		},
	})
	h.Handle(ctx, beginReq)

	// Channel should NOT be closed yet
	select {
	case <-ready:
		t.Fatal("workspaceReady should not be closed after begin")
	default:
	}

	// Send an end notification
	endReq, _ := jsonrpc2.NewNotification("$/progress", protocol.ProgressParams{
		Token: "import-token",
		Value: protocol.WorkDoneProgressEnd{
			Kind:    "end",
			Message: "Finished",
		},
	})
	h.Handle(ctx, endReq)

	// Channel should now be closed
	select {
	case <-ready:
		// success
	case <-time.After(time.Second):
		t.Fatal("workspaceReady should be closed after all progress ends")
	}
}

func TestHandlerMultipleProgressTokens(t *testing.T) {
	log := testr.New(t)
	ready := make(chan struct{})
	h := newJDTLSProgressHandler(log, ready)

	ctx := context.Background()

	// Begin two concurrent progress operations
	begin1, _ := jsonrpc2.NewNotification("$/progress", protocol.ProgressParams{
		Token: "token-1",
		Value: protocol.WorkDoneProgressBegin{Kind: "begin", Title: "Importing Maven project(s)"},
	})
	begin2, _ := jsonrpc2.NewNotification("$/progress", protocol.ProgressParams{
		Token: "token-2",
		Value: protocol.WorkDoneProgressBegin{Kind: "begin", Title: "Resolving classpath"},
	})
	h.Handle(ctx, begin1)
	h.Handle(ctx, begin2)

	// End first token — should NOT signal ready yet
	end1, _ := jsonrpc2.NewNotification("$/progress", protocol.ProgressParams{
		Token: "token-1",
		Value: protocol.WorkDoneProgressEnd{Kind: "end", Message: "Done"},
	})
	h.Handle(ctx, end1)

	select {
	case <-ready:
		t.Fatal("workspaceReady should not be closed while token-2 is still active")
	default:
	}

	// End second token — should signal ready
	end2, _ := jsonrpc2.NewNotification("$/progress", protocol.ProgressParams{
		Token: "token-2",
		Value: protocol.WorkDoneProgressEnd{Kind: "end", Message: "Done"},
	})
	h.Handle(ctx, end2)

	select {
	case <-ready:
		// success
	case <-time.After(time.Second):
		t.Fatal("workspaceReady should be closed after all progress ends")
	}
}

func TestHandlerLanguageStatusReady(t *testing.T) {
	log := testr.New(t)
	ready := make(chan struct{})
	h := newJDTLSProgressHandler(log, ready)

	req, _ := jsonrpc2.NewNotification("language/status", languageStatusParams{
		Type:    "Started",
		Message: "ServiceReady",
	})
	h.Handle(context.Background(), req)

	select {
	case <-ready:
		// success
	case <-time.After(time.Second):
		t.Fatal("workspaceReady should be closed after language/status Started")
	}
}

func TestHandlerLanguageStatusNotReady(t *testing.T) {
	log := testr.New(t)
	ready := make(chan struct{})
	h := newJDTLSProgressHandler(log, ready)

	// "Message" type should NOT signal readiness
	req, _ := jsonrpc2.NewNotification("language/status", languageStatusParams{
		Type:    "Message",
		Message: "Importing project...",
	})
	h.Handle(context.Background(), req)

	select {
	case <-ready:
		t.Fatal("workspaceReady should not be closed for non-Started status")
	default:
		// expected
	}
}

func TestHandlerUnknownMethodReturnsNotHandled(t *testing.T) {
	log := testr.New(t)
	ready := make(chan struct{})
	h := newJDTLSProgressHandler(log, ready)

	req, _ := jsonrpc2.NewNotification("textDocument/publishDiagnostics", nil)
	_, err := h.Handle(context.Background(), req)
	if err != jsonrpc2.ErrNotHandled {
		t.Fatalf("expected ErrNotHandled for unknown method, got: %v", err)
	}
}

func TestHandlerProgressReport(t *testing.T) {
	log := testr.New(t)
	ready := make(chan struct{})
	h := newJDTLSProgressHandler(log, ready)

	ctx := context.Background()

	// Begin
	begin, _ := jsonrpc2.NewNotification("$/progress", protocol.ProgressParams{
		Token: "token",
		Value: protocol.WorkDoneProgressBegin{Kind: "begin", Title: "Importing"},
	})
	h.Handle(ctx, begin)

	// Report — should not crash or signal ready
	report, _ := jsonrpc2.NewNotification("$/progress", protocol.ProgressParams{
		Token: "token",
		Value: protocol.WorkDoneProgressReport{Kind: "report", Message: "50%", Percentage: 50},
	})
	_, err := h.Handle(ctx, report)
	if err != nil {
		t.Fatalf("expected nil error for report, got: %v", err)
	}

	select {
	case <-ready:
		t.Fatal("workspaceReady should not be closed after report")
	default:
	}
}

func TestHandlerSignalReadyIdempotent(t *testing.T) {
	log := testr.New(t)
	ready := make(chan struct{})
	h := newJDTLSProgressHandler(log, ready)

	// Calling signalReady multiple times should not panic
	h.signalReady()
	h.signalReady()
	h.signalReady()

	select {
	case <-ready:
		// success
	default:
		t.Fatal("channel should be closed after signalReady")
	}
}

func TestHandlerPreClosedChannel(t *testing.T) {
	// Simulates the no-build-tool case where channel is pre-closed
	log := testr.New(t)
	ready := make(chan struct{})
	close(ready)
	h := newJDTLSProgressHandler(log, ready)

	// Progress events should not panic even with pre-closed channel
	begin, _ := jsonrpc2.NewNotification("$/progress", protocol.ProgressParams{
		Token: "token",
		Value: protocol.WorkDoneProgressBegin{Kind: "begin", Title: "Importing"},
	})
	_, err := h.Handle(context.Background(), begin)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	end, _ := jsonrpc2.NewNotification("$/progress", protocol.ProgressParams{
		Token: "token",
		Value: protocol.WorkDoneProgressEnd{Kind: "end", Message: "Done"},
	})
	// This calls signalReady on an already-closed channel — should not panic
	_, err = h.Handle(context.Background(), end)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

package java

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	jsonrpc2 "github.com/konveyor/analyzer-lsp/jsonrpc2_v2"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
)

// languageStatusParams represents JDTLS-specific language/status notification.
// This is not part of the LSP standard but is sent by Eclipse JDTLS to indicate
// workspace readiness.
type languageStatusParams struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// progressValue is used to determine the kind of a progress notification value.
// The $/progress notification Value can be WorkDoneProgressBegin, WorkDoneProgressReport,
// or WorkDoneProgressEnd, distinguished by the "kind" field.
type progressValue struct {
	Kind string `json:"kind"`
}

// jdtlsProgressHandler handles JDTLS progress notifications to track workspace
// initialization status. It handles three LSP methods:
//   - window/workDoneProgress/create: acknowledges progress token creation
//   - $/progress: tracks begin/end of workspace import progress
//   - language/status: secondary readiness signal from JDTLS
//
// When all active progress operations complete (or language/status signals "Started"),
// the workspaceReady channel is closed, unblocking Prepare().
type jdtlsProgressHandler struct {
	log            logr.Logger
	workspaceReady chan struct{}
	closeOnce      sync.Once

	mu             sync.Mutex
	activeProgress map[string]string // token (stringified) -> title from WorkDoneProgressBegin
	sawProgress    bool              // true once at least one WorkDoneProgressBegin has been seen
}

func newJDTLSProgressHandler(log logr.Logger, workspaceReady chan struct{}) *jdtlsProgressHandler {
	return &jdtlsProgressHandler{
		log:            log,
		workspaceReady: workspaceReady,
		activeProgress: make(map[string]string),
	}
}

// signalReady closes the workspaceReady channel exactly once.
// It is safe to call even if the channel was already closed externally
// (e.g., the no-build-tool case where the channel is pre-closed).
func (h *jdtlsProgressHandler) signalReady() {
	h.closeOnce.Do(func() {
		select {
		case <-h.workspaceReady:
			// Channel is already closed (e.g., pre-closed for no-build-tool case)
		default:
			close(h.workspaceReady)
		}
	})
}

func (h *jdtlsProgressHandler) Handle(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	switch req.Method {
	case "window/workDoneProgress/create":
		return h.handleWorkDoneProgressCreate(req)
	case "$/progress":
		return h.handleProgress(req)
	case "language/status":
		return h.handleLanguageStatus(req)
	default:
		return nil, jsonrpc2.ErrNotHandled
	}
}

// handleWorkDoneProgressCreate acknowledges a progress token creation request from JDTLS.
// JDTLS sends this as a request (with an ID) that requires a response.
// Returning nil acknowledges the token so JDTLS will send $/progress notifications for it.
func (h *jdtlsProgressHandler) handleWorkDoneProgressCreate(req *jsonrpc2.Request) (interface{}, error) {
	if req.Params != nil {
		var params protocol.WorkDoneProgressCreateParams
		if err := json.Unmarshal(req.Params, &params); err == nil {
			h.log.V(5).Info("acknowledged progress token creation", "token", params.Token)
		}
	}
	// Return nil result with nil error to send a successful response
	return nil, nil
}

// handleProgress processes $/progress notifications to track workspace import status.
// It tracks active progress operations by their tokens. When all active progress
// operations have completed (all begins have matching ends), it signals readiness.
func (h *jdtlsProgressHandler) handleProgress(req *jsonrpc2.Request) (interface{}, error) {
	if req.Params == nil {
		return nil, nil
	}

	var params protocol.ProgressParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		h.log.V(5).Error(err, "failed to parse $/progress params")
		return nil, nil
	}

	// Re-marshal the Value to inspect the kind field
	valueBytes, err := json.Marshal(params.Value)
	if err != nil {
		h.log.V(5).Error(err, "failed to marshal progress value")
		return nil, nil
	}

	var pv progressValue
	if err := json.Unmarshal(valueBytes, &pv); err != nil {
		h.log.V(5).Error(err, "failed to parse progress value kind")
		return nil, nil
	}

	tokenKey := fmt.Sprintf("%v", params.Token)

	switch pv.Kind {
	case "begin":
		var begin protocol.WorkDoneProgressBegin
		if err := json.Unmarshal(valueBytes, &begin); err != nil {
			h.log.V(5).Error(err, "failed to parse WorkDoneProgressBegin")
			return nil, nil
		}
		h.mu.Lock()
		h.activeProgress[tokenKey] = begin.Title
		h.sawProgress = true
		h.mu.Unlock()
		h.log.Info("JDTLS progress started", "title", begin.Title, "token", tokenKey)

	case "report":
		var report protocol.WorkDoneProgressReport
		if err := json.Unmarshal(valueBytes, &report); err == nil {
			h.mu.Lock()
			title := h.activeProgress[tokenKey]
			h.mu.Unlock()
			h.log.V(5).Info("JDTLS progress update", "title", title, "message", report.Message, "percentage", report.Percentage)
		}

	case "end":
		var end protocol.WorkDoneProgressEnd
		if err := json.Unmarshal(valueBytes, &end); err == nil {
			h.mu.Lock()
			title := h.activeProgress[tokenKey]
			delete(h.activeProgress, tokenKey)
			sawProgress := h.sawProgress
			remaining := len(h.activeProgress)
			h.mu.Unlock()

			h.log.Info("JDTLS progress ended", "title", title, "message", end.Message, "remainingActive", remaining)

			if sawProgress && remaining == 0 {
				h.log.Info("all JDTLS initialization progress complete, workspace is ready")
				h.signalReady()
			}
		}
	}

	return nil, nil
}

// handleLanguageStatus processes JDTLS-specific language/status notifications.
// When JDTLS sends type "Started", the workspace is ready.
// This serves as a secondary/fallback readiness signal alongside $/progress.
func (h *jdtlsProgressHandler) handleLanguageStatus(req *jsonrpc2.Request) (interface{}, error) {
	if req.Params == nil {
		return nil, nil
	}

	var params languageStatusParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		h.log.V(5).Error(err, "failed to parse language/status params")
		return nil, nil
	}

	h.log.V(3).Info("JDTLS language status", "type", params.Type, "message", params.Message)

	if params.Type == "Started" {
		h.log.Info("JDTLS reported ServiceReady via language/status")
		h.signalReady()
	}

	return nil, nil
}

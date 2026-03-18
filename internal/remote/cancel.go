package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// ---------------------------------------------------------------------------
// RequestTracker — agent-side cancellation registry
// ---------------------------------------------------------------------------

// RequestTracker keeps track of in-flight requests so they can be cancelled.
// The agent creates one tracker at startup and registers every streaming
// request. When a "cancel" message arrives, it calls Cancel(id) to abort
// the corresponding operation.
type RequestTracker struct {
	mu      sync.Mutex
	pending map[int]context.CancelFunc
}

// NewRequestTracker returns an empty tracker.
func NewRequestTracker() *RequestTracker {
	return &RequestTracker{pending: make(map[int]context.CancelFunc)}
}

// Start registers a new in-flight request and returns a derived context
// that will be cancelled when Cancel(id) is called or the parent context
// is done. The caller MUST call the returned cleanup func when the request
// finishes (whether normally or via cancellation) to avoid leaking entries.
func (rt *RequestTracker) Start(parent context.Context, id int) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	rt.mu.Lock()
	rt.pending[id] = cancel
	rt.mu.Unlock()

	cleanup := func() {
		rt.mu.Lock()
		delete(rt.pending, id)
		rt.mu.Unlock()
		cancel() // no-op if already cancelled, but releases resources
	}
	return ctx, cleanup
}

// Cancel cancels the in-flight request with the given id.
// Returns true if the request was found and cancelled, false if no such
// request is tracked (it may have already completed).
func (rt *RequestTracker) Cancel(id int) bool {
	rt.mu.Lock()
	cancel, ok := rt.pending[id]
	if ok {
		delete(rt.pending, id)
	}
	rt.mu.Unlock()

	if ok {
		cancel()
	}
	return ok
}

// Len returns the number of currently tracked requests.
func (rt *RequestTracker) Len() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return len(rt.pending)
}

// CancelAll cancels every in-flight request. Used during shutdown.
func (rt *RequestTracker) CancelAll() {
	rt.mu.Lock()
	pending := rt.pending
	rt.pending = make(map[int]context.CancelFunc)
	rt.mu.Unlock()

	for _, cancel := range pending {
		cancel()
	}
}

// ---------------------------------------------------------------------------
// Error helpers — constructing error envelopes
// ---------------------------------------------------------------------------

// NewErrorEnvelope builds an Envelope with an error response for the given
// request id.
func NewErrorEnvelope(id int, code int, message string) Envelope {
	return Envelope{
		ID:   id,
		Done: true,
		Error: &ProtoError{
			Code:    code,
			Message: message,
		},
	}
}

// NewCancelledEnvelope is a convenience for building a cancellation error.
func NewCancelledEnvelope(id int) Envelope {
	return NewErrorEnvelope(id, ErrCodeCancelled, "request cancelled")
}

// NewNotFoundEnvelope is a convenience for building a not-found error.
func NewNotFoundEnvelope(id int, path string) Envelope {
	return NewErrorEnvelope(id, ErrCodeNotFound, fmt.Sprintf("not found: %s", path))
}

// NewPermissionDeniedEnvelope is a convenience for building a permission error.
func NewPermissionDeniedEnvelope(id int, path string) Envelope {
	return NewErrorEnvelope(id, ErrCodePermissionDenied, fmt.Sprintf("permission denied: %s", path))
}

// NewInternalErrorEnvelope is a convenience for building an internal error.
func NewInternalErrorEnvelope(id int, err error) Envelope {
	return NewErrorEnvelope(id, ErrCodeInternal, err.Error())
}

// ---------------------------------------------------------------------------
// Response helpers — client-side error extraction
// ---------------------------------------------------------------------------

// responseShape is used by ParseResponse to extract the error and done fields.
type responseShape struct {
	ID     int             `json:"id"`
	Method string          `json:"method"`
	Result json.RawMessage `json:"result"`
	Error  *ProtoError     `json:"error"`
	Done   bool            `json:"done"`
}

// ParsedResponse holds the decoded fields of an agent response.
type ParsedResponse struct {
	ID     int
	Method string
	Result json.RawMessage // nil when absent
	Done   bool
}

// ParseResponse extracts the id, method, result, and done flag from a raw
// response message. If the message carries a ProtoError, it is returned as
// a Go error (of type *ProtoError, so callers can type-assert for the code).
func ParseResponse(raw json.RawMessage) (ParsedResponse, error) {
	var shape responseShape
	if err := json.Unmarshal(raw, &shape); err != nil {
		return ParsedResponse{}, fmt.Errorf("remote: parse response: %w", err)
	}
	if shape.Error != nil {
		return ParsedResponse{
			ID:   shape.ID,
			Done: shape.Done,
		}, shape.Error
	}
	return ParsedResponse{
		ID:     shape.ID,
		Method: shape.Method,
		Result: shape.Result,
		Done:   shape.Done,
	}, nil
}

// IsProtoError checks whether err is a *ProtoError and, if so, returns it.
func IsProtoError(err error) (*ProtoError, bool) {
	pe, ok := err.(*ProtoError)
	return pe, ok
}

// IsCancelled returns true if the error is a ProtoError with code ErrCodeCancelled.
func IsCancelled(err error) bool {
	pe, ok := IsProtoError(err)
	return ok && pe.Code == ErrCodeCancelled
}

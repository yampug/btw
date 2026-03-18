package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
)

// HandlerFunc is the signature for a method handler on the agent side.
// It receives the request ID, raw params, the encoder to write responses,
// and a context that is cancelled if the client sends a cancel request.
type HandlerFunc func(ctx context.Context, id int, params json.RawMessage, enc *Encoder) error

// AgentServer reads JSON-Lines requests from a Decoder and dispatches them
// to registered handlers. It is the core loop of the btw-agent process.
//
// AgentServer is designed to be usable both from cmd/btw-agent/main.go and
// from in-process tests (connected via io.Pipe).
type AgentServer struct {
	handlers map[string]HandlerFunc
	tracker  *RequestTracker
	logger   *log.Logger
}

// NewAgentServer creates a new server with the built-in ping handler
// registered. Additional handlers can be added with Handle().
func NewAgentServer(logger *log.Logger) *AgentServer {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	s := &AgentServer{
		handlers: make(map[string]HandlerFunc),
		tracker:  NewRequestTracker(),
		logger:   logger,
	}

	// Built-in: ping → pong
	s.Handle(MethodPing, s.handlePing)

	return s
}

// Handle registers a handler for the given method name.
func (s *AgentServer) Handle(method string, h HandlerFunc) {
	s.handlers[method] = h
}

// Serve runs the main read-dispatch loop. It reads from dec, dispatches to
// handlers, and writes responses to enc. It returns nil on clean EOF and
// an error for unexpected failures.
func (s *AgentServer) Serve(ctx context.Context, dec *Decoder, enc *Encoder) error {
	for {
		// Check parent context first.
		select {
		case <-ctx.Done():
			s.tracker.CancelAll()
			return ctx.Err()
		default:
		}

		raw, err := dec.Receive()
		if err == io.EOF {
			s.logger.Println("EOF received, shutting down")
			s.tracker.CancelAll()
			return nil
		}
		if err != nil {
			s.logger.Printf("read error: %v", err)
			return fmt.Errorf("agent: read: %w", err)
		}

		method, id, params, err := ParseRequest(raw)
		if err != nil {
			s.logger.Printf("bad request: %v", err)
			// Send an error response. Use id=0 since we couldn't parse it.
			_ = enc.Send(NewErrorEnvelope(0, ErrCodeInternal, fmt.Sprintf("bad request: %v", err)))
			continue
		}

		s.logger.Printf("request: method=%s id=%d", method, id)

		// Handle cancel specially — it targets another request.
		if method == MethodCancel {
			s.handleCancel(id, params, enc)
			continue
		}

		handler, ok := s.handlers[method]
		if !ok {
			s.logger.Printf("unknown method: %s", method)
			_ = enc.Send(NewErrorEnvelope(id, ErrCodeInternal, fmt.Sprintf("unknown method: %s", method)))
			continue
		}

		// Run the handler with a tracked, cancellable context.
		reqCtx, cleanup := s.tracker.Start(ctx, id)
		go func() {
			defer cleanup()
			if err := handler(reqCtx, id, params, enc); err != nil {
				if reqCtx.Err() != nil {
					// Cancelled — send cancelled envelope.
					_ = enc.Send(NewCancelledEnvelope(id))
				} else {
					s.logger.Printf("handler error: method=%s id=%d err=%v", method, id, err)
					_ = enc.Send(NewInternalErrorEnvelope(id, err))
				}
			}
		}()
	}
}

// handlePing responds to a ping with a pong.
func (s *AgentServer) handlePing(ctx context.Context, id int, params json.RawMessage, enc *Encoder) error {
	return enc.Send(Envelope{
		Method: MethodPong,
		ID:     id,
		Result: PongResult{},
		Done:   true,
	})
}

// handleCancel processes a cancel request.
func (s *AgentServer) handleCancel(id int, params json.RawMessage, enc *Encoder) {
	var cp CancelParams
	if err := json.Unmarshal(params, &cp); err != nil {
		s.logger.Printf("bad cancel params: %v", err)
		_ = enc.Send(NewErrorEnvelope(id, ErrCodeInternal, "bad cancel params"))
		return
	}

	found := s.tracker.Cancel(cp.CancelID)
	s.logger.Printf("cancel: target_id=%d found=%v", cp.CancelID, found)
}

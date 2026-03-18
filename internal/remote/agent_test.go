package remote

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

// agentTestHarness wires up a client and agent connected by io.Pipe.
// The test communicates with the agent through clientEnc/clientDec.
// Call stop() when done — it closes the client→agent pipe (sending EOF)
// and waits for the serve loop to exit.
type agentTestHarness struct {
	clientEnc *Encoder
	clientDec *Decoder
	server    *AgentServer
	serveErr  error

	clientWriter io.WriteCloser // close to send EOF to agent
	agentWriter  io.WriteCloser // close to unblock client reads
	wg           sync.WaitGroup
}

func newAgentTestHarness(t *testing.T) *agentTestHarness {
	t.Helper()

	// client writes → agent reads
	cr, cw := io.Pipe()
	// agent writes → client reads
	ar, aw := io.Pipe()

	h := &agentTestHarness{
		clientEnc:    NewEncoder(cw),
		clientDec:    NewDecoder(ar),
		server:       NewAgentServer(nil),
		clientWriter: cw,
		agentWriter:  aw,
	}

	agentDec := NewDecoder(cr)
	agentEnc := NewEncoder(aw)

	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		h.serveErr = h.server.Serve(context.Background(), agentDec, agentEnc)
	}()

	return h
}

// stop closes the client→agent pipe, causing the agent to receive EOF,
// then waits for the serve goroutine to exit.
func (h *agentTestHarness) stop() {
	h.clientWriter.Close()
	h.wg.Wait()
	h.agentWriter.Close()
}

// ---------------------------------------------------------------------------
// Ping / Pong
// ---------------------------------------------------------------------------

func TestAgentServer_Ping(t *testing.T) {
	h := newAgentTestHarness(t)
	defer h.stop()

	// Send ping.
	if err := h.clientEnc.Send(Envelope{Method: MethodPing, ID: 1}); err != nil {
		t.Fatal(err)
	}

	// Read pong.
	raw, err := h.clientDec.Receive()
	if err != nil {
		t.Fatal(err)
	}

	resp, err := ParseResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != 1 {
		t.Errorf("id = %d, want 1", resp.ID)
	}
	if !resp.Done {
		t.Error("pong should have done=true")
	}
	if resp.Method != MethodPong {
		t.Errorf("method = %q, want %q", resp.Method, MethodPong)
	}
}

// ---------------------------------------------------------------------------
// EOF → clean exit
// ---------------------------------------------------------------------------

func TestAgentServer_EOF(t *testing.T) {
	h := newAgentTestHarness(t)

	// Close client writer immediately → agent gets EOF.
	h.clientWriter.Close()
	h.wg.Wait()
	h.agentWriter.Close()

	if h.serveErr != nil {
		t.Errorf("expected nil error on EOF, got %v", h.serveErr)
	}
}

// ---------------------------------------------------------------------------
// Invalid JSON → error response, no crash
// ---------------------------------------------------------------------------

func TestAgentServer_InvalidJSON(t *testing.T) {
	h := newAgentTestHarness(t)
	defer h.stop()

	// Write raw invalid JSON directly to the pipe, bypassing the Encoder
	// (which would reject it during json.Marshal).
	_, err := h.clientWriter.Write([]byte("{invalid json}\n"))
	if err != nil {
		t.Fatal(err)
	}

	// Agent should respond with an error envelope.
	raw, err := h.clientDec.Receive()
	if err != nil {
		t.Fatal(err)
	}

	_, parseErr := ParseResponse(raw)
	if parseErr == nil {
		t.Fatal("expected error response")
	}
	pe, ok := IsProtoError(parseErr)
	if !ok {
		t.Fatalf("expected *ProtoError, got %T: %v", parseErr, parseErr)
	}
	if pe.Code != ErrCodeInternal {
		t.Errorf("code = %d, want %d", pe.Code, ErrCodeInternal)
	}

	// Server should still be alive — send a ping to confirm.
	if err := h.clientEnc.Send(Envelope{Method: MethodPing, ID: 2}); err != nil {
		t.Fatal(err)
	}
	raw, err = h.clientDec.Receive()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := ParseResponse(raw)
	if err != nil {
		t.Fatalf("ping after bad json: %v", err)
	}
	if resp.Method != MethodPong {
		t.Errorf("expected pong, got %q", resp.Method)
	}
}

// ---------------------------------------------------------------------------
// Unknown method → error response
// ---------------------------------------------------------------------------

func TestAgentServer_UnknownMethod(t *testing.T) {
	h := newAgentTestHarness(t)
	defer h.stop()

	// Send request with unknown method.
	if err := h.clientEnc.Send(Envelope{Method: "nonexistent", ID: 10}); err != nil {
		t.Fatal(err)
	}

	raw, err := h.clientDec.Receive()
	if err != nil {
		t.Fatal(err)
	}

	_, parseErr := ParseResponse(raw)
	if parseErr == nil {
		t.Fatal("expected error for unknown method")
	}
	pe, ok := IsProtoError(parseErr)
	if !ok {
		t.Fatalf("expected *ProtoError, got %T", parseErr)
	}
	if pe.Code != ErrCodeInternal {
		t.Errorf("code = %d, want %d", pe.Code, ErrCodeInternal)
	}
}

// ---------------------------------------------------------------------------
// Custom handler registration
// ---------------------------------------------------------------------------

func TestAgentServer_CustomHandler(t *testing.T) {
	h := newAgentTestHarness(t)
	defer h.stop()

	// Register a custom "echo" handler.
	h.server.Handle("echo", func(ctx context.Context, id int, params json.RawMessage, enc *Encoder) error {
		return enc.Send(Envelope{
			Method: "echo_reply",
			ID:     id,
			Result: json.RawMessage(params),
			Done:   true,
		})
	})

	// Send echo request.
	if err := h.clientEnc.Send(Envelope{
		Method: "echo",
		ID:     3,
		Params: json.RawMessage(`{"hello":"world"}`),
	}); err != nil {
		t.Fatal(err)
	}

	raw, err := h.clientDec.Receive()
	if err != nil {
		t.Fatal(err)
	}

	resp, err := ParseResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != 3 {
		t.Errorf("id = %d", resp.ID)
	}
	if resp.Method != "echo_reply" {
		t.Errorf("method = %q", resp.Method)
	}
	if !resp.Done {
		t.Error("done should be true")
	}
}

// ---------------------------------------------------------------------------
// Handler error → internal error response
// ---------------------------------------------------------------------------

func TestAgentServer_HandlerError(t *testing.T) {
	h := newAgentTestHarness(t)
	defer h.stop()

	h.server.Handle("fail", func(ctx context.Context, id int, params json.RawMessage, enc *Encoder) error {
		return errors.New("something went wrong")
	})

	if err := h.clientEnc.Send(Envelope{Method: "fail", ID: 4}); err != nil {
		t.Fatal(err)
	}

	raw, err := h.clientDec.Receive()
	if err != nil {
		t.Fatal(err)
	}

	_, parseErr := ParseResponse(raw)
	if parseErr == nil {
		t.Fatal("expected error response")
	}
	pe, ok := IsProtoError(parseErr)
	if !ok {
		t.Fatalf("expected *ProtoError, got %T", parseErr)
	}
	if pe.Code != ErrCodeInternal {
		t.Errorf("code = %d, want %d", pe.Code, ErrCodeInternal)
	}
	if pe.Message != "something went wrong" {
		t.Errorf("message = %q", pe.Message)
	}
}

// ---------------------------------------------------------------------------
// Streaming handler with cancellation
// ---------------------------------------------------------------------------

func TestAgentServer_StreamingWithCancel(t *testing.T) {
	h := newAgentTestHarness(t)
	defer h.stop()

	// A handler that streams until cancelled.
	streamStarted := make(chan struct{})
	h.server.Handle("slow_stream", func(ctx context.Context, id int, params json.RawMessage, enc *Encoder) error {
		close(streamStarted)
		i := 0
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(10 * time.Millisecond):
				i++
				if err := enc.Send(Envelope{
					Method: "slow_stream_entry",
					ID:     id,
					Result: map[string]int{"n": i},
				}); err != nil {
					return err
				}
			}
		}
	})

	// Start the stream.
	if err := h.clientEnc.Send(Envelope{Method: "slow_stream", ID: 10}); err != nil {
		t.Fatal(err)
	}

	// Wait for handler to start.
	<-streamStarted

	// Read a few entries.
	for i := 0; i < 3; i++ {
		raw, err := h.clientDec.Receive()
		if err != nil {
			t.Fatalf("receive entry %d: %v", i, err)
		}
		resp, err := ParseResponse(raw)
		if err != nil {
			t.Fatalf("parse entry %d: %v", i, err)
		}
		if resp.Method != "slow_stream_entry" {
			t.Errorf("entry %d method = %q", i, resp.Method)
		}
	}

	// Send cancel.
	if err := h.clientEnc.Send(Envelope{
		Method: MethodCancel,
		ID:     20,
		Params: CancelParams{CancelID: 10},
	}); err != nil {
		t.Fatal(err)
	}

	// The handler should stop. Read until we get the cancelled error or
	// the stream stops (within a timeout).
	gotCancelled := false
	deadline := time.After(2 * time.Second)
	for !gotCancelled {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for cancelled response")
		default:
		}

		raw, err := h.clientDec.Receive()
		if err != nil {
			break
		}
		_, parseErr := ParseResponse(raw)
		if parseErr != nil {
			if IsCancelled(parseErr) {
				gotCancelled = true
			}
		}
	}

	if !gotCancelled {
		t.Error("expected a cancelled error response")
	}
}

// ---------------------------------------------------------------------------
// Multiple pings
// ---------------------------------------------------------------------------

func TestAgentServer_MultiplePings(t *testing.T) {
	h := newAgentTestHarness(t)
	defer h.stop()

	// Send 10 pings.
	for i := 1; i <= 10; i++ {
		if err := h.clientEnc.Send(Envelope{Method: MethodPing, ID: i}); err != nil {
			t.Fatal(err)
		}
	}

	// Read 10 pongs.
	seen := make(map[int]bool)
	for i := 0; i < 10; i++ {
		raw, err := h.clientDec.Receive()
		if err != nil {
			t.Fatalf("receive %d: %v", i, err)
		}
		resp, err := ParseResponse(raw)
		if err != nil {
			t.Fatalf("parse %d: %v", i, err)
		}
		if resp.Method != MethodPong {
			t.Errorf("expected pong, got %q", resp.Method)
		}
		seen[resp.ID] = true
	}

	if len(seen) != 10 {
		t.Errorf("expected 10 unique pong IDs, got %d", len(seen))
	}
}

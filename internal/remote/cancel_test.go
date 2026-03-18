package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// RequestTracker — basic operations
// ---------------------------------------------------------------------------

func TestRequestTracker_StartAndCleanup(t *testing.T) {
	rt := NewRequestTracker()

	ctx, cleanup := rt.Start(context.Background(), 1)
	if rt.Len() != 1 {
		t.Errorf("len = %d, want 1", rt.Len())
	}

	// Context should not be cancelled yet.
	select {
	case <-ctx.Done():
		t.Fatal("context should not be done")
	default:
	}

	cleanup()
	if rt.Len() != 0 {
		t.Errorf("len after cleanup = %d, want 0", rt.Len())
	}
}

func TestRequestTracker_Cancel(t *testing.T) {
	rt := NewRequestTracker()

	ctx, cleanup := rt.Start(context.Background(), 5)
	defer cleanup()

	ok := rt.Cancel(5)
	if !ok {
		t.Error("cancel should return true for tracked request")
	}

	select {
	case <-ctx.Done():
		// good — context was cancelled
	case <-time.After(100 * time.Millisecond):
		t.Fatal("context should be done after Cancel")
	}
}

func TestRequestTracker_CancelUnknownID(t *testing.T) {
	rt := NewRequestTracker()
	ok := rt.Cancel(999)
	if ok {
		t.Error("cancel should return false for unknown id")
	}
}

func TestRequestTracker_CancelAll(t *testing.T) {
	rt := NewRequestTracker()
	ctxs := make([]context.Context, 5)
	for i := range ctxs {
		ctx, _ := rt.Start(context.Background(), i)
		ctxs[i] = ctx
	}

	if rt.Len() != 5 {
		t.Fatalf("len = %d, want 5", rt.Len())
	}

	rt.CancelAll()

	if rt.Len() != 0 {
		t.Errorf("len after CancelAll = %d, want 0", rt.Len())
	}

	for i, ctx := range ctxs {
		select {
		case <-ctx.Done():
		default:
			t.Errorf("context %d should be cancelled", i)
		}
	}
}

func TestRequestTracker_DoubleCleanup(t *testing.T) {
	rt := NewRequestTracker()
	_, cleanup := rt.Start(context.Background(), 1)
	cleanup()
	cleanup() // should not panic
	if rt.Len() != 0 {
		t.Errorf("len = %d", rt.Len())
	}
}

func TestRequestTracker_CancelThenCleanup(t *testing.T) {
	rt := NewRequestTracker()
	_, cleanup := rt.Start(context.Background(), 1)
	rt.Cancel(1)
	cleanup() // should not panic even though already cancelled + removed
	if rt.Len() != 0 {
		t.Errorf("len = %d", rt.Len())
	}
}

func TestRequestTracker_ParentContextCancellation(t *testing.T) {
	rt := NewRequestTracker()
	parent, parentCancel := context.WithCancel(context.Background())

	ctx, cleanup := rt.Start(parent, 1)
	defer cleanup()

	parentCancel()

	select {
	case <-ctx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("child context should be done when parent is cancelled")
	}
}

func TestRequestTracker_ConcurrentAccess(t *testing.T) {
	rt := NewRequestTracker()
	var wg sync.WaitGroup

	// Concurrently start and cancel many requests.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, cleanup := rt.Start(context.Background(), id)
			// Randomly cancel or cleanup.
			if id%2 == 0 {
				rt.Cancel(id)
			}
			cleanup()
		}(i)
	}

	wg.Wait()
	if rt.Len() != 0 {
		t.Errorf("len after concurrent ops = %d, want 0", rt.Len())
	}
}

// ---------------------------------------------------------------------------
// Error envelope constructors
// ---------------------------------------------------------------------------

func TestNewErrorEnvelope(t *testing.T) {
	env := NewErrorEnvelope(7, ErrCodeNotFound, "file not found")
	if env.ID != 7 {
		t.Errorf("id = %d", env.ID)
	}
	if !env.Done {
		t.Error("done should be true")
	}
	if env.Error == nil {
		t.Fatal("error should not be nil")
	}
	if env.Error.Code != ErrCodeNotFound {
		t.Errorf("code = %d", env.Error.Code)
	}
	if env.Error.Message != "file not found" {
		t.Errorf("message = %q", env.Error.Message)
	}
}

func TestNewCancelledEnvelope(t *testing.T) {
	env := NewCancelledEnvelope(3)
	if env.Error.Code != ErrCodeCancelled {
		t.Errorf("code = %d, want %d", env.Error.Code, ErrCodeCancelled)
	}
}

func TestNewNotFoundEnvelope(t *testing.T) {
	env := NewNotFoundEnvelope(1, "/missing/path")
	if env.Error.Code != ErrCodeNotFound {
		t.Errorf("code = %d", env.Error.Code)
	}
	if env.Error.Message != "not found: /missing/path" {
		t.Errorf("message = %q", env.Error.Message)
	}
}

func TestNewPermissionDeniedEnvelope(t *testing.T) {
	env := NewPermissionDeniedEnvelope(2, "/root/secret")
	if env.Error.Code != ErrCodePermissionDenied {
		t.Errorf("code = %d", env.Error.Code)
	}
}

func TestNewInternalErrorEnvelope(t *testing.T) {
	env := NewInternalErrorEnvelope(4, io.ErrUnexpectedEOF)
	if env.Error.Code != ErrCodeInternal {
		t.Errorf("code = %d", env.Error.Code)
	}
	if env.Error.Message != "unexpected EOF" {
		t.Errorf("message = %q", env.Error.Message)
	}
}

// ---------------------------------------------------------------------------
// ParseResponse — success
// ---------------------------------------------------------------------------

func TestParseResponse_Success(t *testing.T) {
	env := Envelope{
		Method: MethodWalkEntry,
		ID:     1,
		Result: FileEntryResult{Path: "/tmp/a.go", Name: "a.go"},
		Done:   false,
	}
	data, _ := json.Marshal(env)

	resp, err := ParseResponse(json.RawMessage(data))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.ID != 1 {
		t.Errorf("id = %d", resp.ID)
	}
	if resp.Method != MethodWalkEntry {
		t.Errorf("method = %q", resp.Method)
	}
	if resp.Done {
		t.Error("done should be false")
	}
	if resp.Result == nil {
		t.Error("result should not be nil")
	}
}

func TestParseResponse_DoneMessage(t *testing.T) {
	env := Envelope{
		Method: MethodWalkEntry,
		ID:     1,
		Done:   true,
	}
	data, _ := json.Marshal(env)

	resp, err := ParseResponse(json.RawMessage(data))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !resp.Done {
		t.Error("done should be true")
	}
}

// ---------------------------------------------------------------------------
// ParseResponse — error
// ---------------------------------------------------------------------------

func TestParseResponse_Error(t *testing.T) {
	env := NewCancelledEnvelope(5)
	data, _ := json.Marshal(env)

	_, err := ParseResponse(json.RawMessage(data))
	if err == nil {
		t.Fatal("expected error")
	}

	pe, ok := IsProtoError(err)
	if !ok {
		t.Fatalf("expected *ProtoError, got %T", err)
	}
	if pe.Code != ErrCodeCancelled {
		t.Errorf("code = %d, want %d", pe.Code, ErrCodeCancelled)
	}
}

func TestParseResponse_NotFoundError(t *testing.T) {
	env := NewNotFoundEnvelope(2, "/missing")
	data, _ := json.Marshal(env)

	_, err := ParseResponse(json.RawMessage(data))
	if err == nil {
		t.Fatal("expected error")
	}

	pe, ok := IsProtoError(err)
	if !ok {
		t.Fatal("expected *ProtoError")
	}
	if pe.Code != ErrCodeNotFound {
		t.Errorf("code = %d", pe.Code)
	}
}

func TestParseResponse_MalformedJSON(t *testing.T) {
	_, err := ParseResponse(json.RawMessage(`{garbage`))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

// ---------------------------------------------------------------------------
// IsCancelled / IsProtoError helpers
// ---------------------------------------------------------------------------

func TestIsCancelled_True(t *testing.T) {
	err := &ProtoError{Code: ErrCodeCancelled, Message: "cancelled"}
	if !IsCancelled(err) {
		t.Error("should be cancelled")
	}
}

func TestIsCancelled_False_DifferentCode(t *testing.T) {
	err := &ProtoError{Code: ErrCodeNotFound, Message: "not found"}
	if IsCancelled(err) {
		t.Error("should not be cancelled")
	}
}

func TestIsCancelled_False_NotProtoError(t *testing.T) {
	err := io.EOF
	if IsCancelled(err) {
		t.Error("io.EOF should not be cancelled")
	}
}

func TestIsProtoError_Nil(t *testing.T) {
	_, ok := IsProtoError(nil)
	if ok {
		t.Error("nil should not be a ProtoError")
	}
}

// ---------------------------------------------------------------------------
// Simulated walk with cancellation — end-to-end over codec
// ---------------------------------------------------------------------------

func TestSimulatedWalkWithCancellation(t *testing.T) {
	// Simulate: client sends walk request, agent streams entries, client
	// sends cancel, agent stops and sends a cancelled done.
	//
	// We use two goroutines connected by pipes to simulate the two sides.

	clientToAgent := &bytes.Buffer{}
	agentToClient := &bytes.Buffer{}

	// We'll assemble the conversation sequentially for determinism:
	// 1. Client sends walk request
	// 2. Agent sends a few walk_entry results
	// 3. Client sends cancel
	// 4. Agent sends cancelled done

	clientEnc := NewEncoder(clientToAgent)
	agentEnc := NewEncoder(agentToClient)

	// Step 1: Client sends walk.
	walkReq := Envelope{Method: MethodWalk, ID: 1, Params: WalkParams{Root: "/project"}}
	if err := clientEnc.Send(walkReq); err != nil {
		t.Fatal(err)
	}

	// Agent side: set up tracker, parse request, start context.
	rt := NewRequestTracker()
	agentDec := NewDecoder(clientToAgent)

	raw, err := agentDec.Receive()
	if err != nil {
		t.Fatal(err)
	}
	method, id, _, err := ParseRequest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if method != MethodWalk || id != 1 {
		t.Fatalf("method=%q id=%d", method, id)
	}

	ctx, cleanup := rt.Start(context.Background(), id)
	defer cleanup()

	// Step 2: Agent streams 3 entries.
	var sentCount int32
	for i := 0; i < 3; i++ {
		select {
		case <-ctx.Done():
			break
		default:
		}
		entry := Envelope{
			Method: MethodWalkEntry,
			ID:     id,
			Result: FileEntryResult{Path: "/project/file" + string(rune('a'+i)) + ".go"},
		}
		if err := agentEnc.Send(entry); err != nil {
			t.Fatal(err)
		}
		atomic.AddInt32(&sentCount, 1)
	}

	// Step 3: Client sends cancel (write to a fresh buffer since the old one was consumed).
	cancelBuf := &bytes.Buffer{}
	cancelEnc := NewEncoder(cancelBuf)
	cancelReq := Envelope{Method: MethodCancel, ID: 2, Params: CancelParams{CancelID: 1}}
	if err := cancelEnc.Send(cancelReq); err != nil {
		t.Fatal(err)
	}

	// Agent reads and processes cancel.
	cancelDec := NewDecoder(cancelBuf)
	rawCancel, err := cancelDec.Receive()
	if err != nil {
		t.Fatal(err)
	}
	cancelMethod, _, cancelParams, err := ParseRequest(rawCancel)
	if err != nil {
		t.Fatal(err)
	}
	if cancelMethod != MethodCancel {
		t.Fatalf("expected cancel, got %q", cancelMethod)
	}
	var cp CancelParams
	if err := json.Unmarshal(cancelParams, &cp); err != nil {
		t.Fatal(err)
	}

	found := rt.Cancel(cp.CancelID)
	if !found {
		t.Error("expected cancel to find request 1")
	}

	// Context should now be done.
	select {
	case <-ctx.Done():
	default:
		t.Fatal("context should be cancelled")
	}

	// Step 4: Agent sends cancelled done message.
	doneEnv := NewCancelledEnvelope(id)
	if err := agentEnc.Send(doneEnv); err != nil {
		t.Fatal(err)
	}

	// Client reads all messages from agent.
	clientDec := NewDecoder(agentToClient)
	var received []ParsedResponse
	for {
		raw, err := clientDec.Receive()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		resp, parseErr := ParseResponse(raw)
		if parseErr != nil {
			// Error responses are still valid — store them.
			pe, ok := IsProtoError(parseErr)
			if !ok {
				t.Fatalf("unexpected error: %v", parseErr)
			}
			received = append(received, ParsedResponse{ID: resp.ID, Done: true})
			if !IsCancelled(pe) {
				t.Errorf("expected cancelled error, got code %d", pe.Code)
			}
			continue
		}
		received = append(received, resp)
	}

	// We should have 3 entries + 1 cancelled done = 4 messages.
	if len(received) != 4 {
		t.Fatalf("received %d messages, want 4", len(received))
	}

	// First 3 should NOT be done.
	for i := 0; i < 3; i++ {
		if received[i].Done {
			t.Errorf("message %d should not be done", i)
		}
	}

	// Last should be done.
	if !received[3].Done {
		t.Error("last message should be done (cancelled)")
	}
}

// ---------------------------------------------------------------------------
// Error envelope round-trip through encoder/decoder
// ---------------------------------------------------------------------------

func TestErrorEnvelope_Codec_RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)

	// Send an error envelope.
	errEnv := NewNotFoundEnvelope(42, "/does/not/exist")
	if err := enc.Send(errEnv); err != nil {
		t.Fatal(err)
	}

	dec := NewDecoder(&buf)
	raw, err := dec.Receive()
	if err != nil {
		t.Fatal(err)
	}

	_, parseErr := ParseResponse(raw)
	if parseErr == nil {
		t.Fatal("expected error from ParseResponse")
	}

	pe, ok := IsProtoError(parseErr)
	if !ok {
		t.Fatalf("expected *ProtoError, got %T", parseErr)
	}
	if pe.Code != ErrCodeNotFound {
		t.Errorf("code = %d, want %d", pe.Code, ErrCodeNotFound)
	}
	if pe.Message != "not found: /does/not/exist" {
		t.Errorf("message = %q", pe.Message)
	}
}

package remote

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Encoder.Send + Decoder.Receive round-trip
// ---------------------------------------------------------------------------

func TestCodec_RoundTrip_Envelope(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	dec := NewDecoder(&buf)

	sent := Envelope{
		Method: MethodWalk,
		ID:     1,
		Params: WalkParams{
			Root:          "/home/user/project",
			IncludeHidden: true,
			MaxDepth:      50,
		},
	}

	if err := enc.Send(sent); err != nil {
		t.Fatalf("send: %v", err)
	}

	raw, err := dec.Receive()
	if err != nil {
		t.Fatalf("receive: %v", err)
	}

	var got Envelope
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Method != MethodWalk {
		t.Errorf("method = %q, want %q", got.Method, MethodWalk)
	}
	if got.ID != 1 {
		t.Errorf("id = %d, want 1", got.ID)
	}
}

func TestCodec_RoundTrip_FileEntryResult(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	dec := NewDecoder(&buf)

	sent := Envelope{
		Method: MethodWalkEntry,
		ID:     1,
		Result: FileEntryResult{
			Path:    "/home/user/project/main.go",
			RelPath: "main.go",
			Name:    "main.go",
			Ext:     ".go",
			Size:    5086,
			ModTime: time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC),
		},
	}

	if err := enc.Send(sent); err != nil {
		t.Fatalf("send: %v", err)
	}

	raw, err := dec.Receive()
	if err != nil {
		t.Fatalf("receive: %v", err)
	}

	var got Envelope
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Method != MethodWalkEntry {
		t.Errorf("method = %q, want %q", got.Method, MethodWalkEntry)
	}
	// Result comes back as map; verify it round-trips via re-marshal.
	resultBytes, _ := json.Marshal(got.Result)
	var fe FileEntryResult
	if err := json.Unmarshal(resultBytes, &fe); err != nil {
		t.Fatalf("re-unmarshal result: %v", err)
	}
	if fe.Path != "/home/user/project/main.go" {
		t.Errorf("path = %q", fe.Path)
	}
	if fe.Size != 5086 {
		t.Errorf("size = %d, want 5086", fe.Size)
	}
}

func TestCodec_RoundTrip_GrepMatchResult(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	dec := NewDecoder(&buf)

	sent := Envelope{
		Method: MethodGrepMatch,
		ID:     2,
		Result: GrepMatchResult{
			FilePath: "/home/user/project/main.go",
			RelPath:  "main.go",
			FileName: "main.go",
			Line:     42,
			Column:   5,
			Content:  "func main() {",
			MatchRanges: []MatchRange{
				{Start: 5, End: 9},
			},
		},
	}

	if err := enc.Send(sent); err != nil {
		t.Fatalf("send: %v", err)
	}

	raw, err := dec.Receive()
	if err != nil {
		t.Fatalf("receive: %v", err)
	}

	var got Envelope
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	resultBytes, _ := json.Marshal(got.Result)
	var gm GrepMatchResult
	if err := json.Unmarshal(resultBytes, &gm); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if gm.Line != 42 {
		t.Errorf("line = %d, want 42", gm.Line)
	}
}

// ---------------------------------------------------------------------------
// Multiple messages in sequence
// ---------------------------------------------------------------------------

func TestCodec_MultipleMessages(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	dec := NewDecoder(&buf)

	for i := 0; i < 100; i++ {
		env := Envelope{Method: MethodPing, ID: i}
		if err := enc.Send(env); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}

	for i := 0; i < 100; i++ {
		raw, err := dec.Receive()
		if err != nil {
			t.Fatalf("receive %d: %v", i, err)
		}
		var got Envelope
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("unmarshal %d: %v", i, err)
		}
		if got.ID != i {
			t.Errorf("msg %d: id = %d", i, got.ID)
		}
	}
}

// ---------------------------------------------------------------------------
// EOF handling
// ---------------------------------------------------------------------------

func TestDecoder_EOF_EmptyReader(t *testing.T) {
	dec := NewDecoder(strings.NewReader(""))
	_, err := dec.Receive()
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestDecoder_EOF_AfterLastMessage(t *testing.T) {
	input := `{"method":"ping","id":1}` + "\n"
	dec := NewDecoder(strings.NewReader(input))

	raw, err := dec.Receive()
	if err != nil {
		t.Fatalf("first receive: %v", err)
	}
	var got Envelope
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Method != MethodPing {
		t.Errorf("method = %q", got.Method)
	}

	_, err = dec.Receive()
	if err != io.EOF {
		t.Errorf("expected io.EOF after last message, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Malformed JSON
// ---------------------------------------------------------------------------

func TestDecoder_MalformedJSON(t *testing.T) {
	// The decoder returns the raw bytes; it's up to the caller to unmarshal.
	// So the decoder itself should NOT error — it just returns the raw line.
	input := `{this is not json}` + "\n"
	dec := NewDecoder(strings.NewReader(input))

	raw, err := dec.Receive()
	if err != nil {
		t.Fatalf("receive: %v", err)
	}

	// But unmarshalling should fail.
	var env Envelope
	if err := json.Unmarshal(raw, &env); err == nil {
		t.Error("expected unmarshal error for malformed JSON")
	}
}

// ---------------------------------------------------------------------------
// Very large payloads (>1 MB)
// ---------------------------------------------------------------------------

func TestCodec_LargePayload(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	dec := NewDecoder(&buf)

	// Build a payload with a ~1.5 MB content string.
	bigContent := strings.Repeat("x", 1_500_000)
	sent := Envelope{
		Method: MethodGrepMatch,
		ID:     99,
		Result: GrepMatchResult{
			FilePath: "/big/file.go",
			Content:  bigContent,
		},
	}

	if err := enc.Send(sent); err != nil {
		t.Fatalf("send large payload: %v", err)
	}

	raw, err := dec.Receive()
	if err != nil {
		t.Fatalf("receive large payload: %v", err)
	}

	var got Envelope
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal large payload: %v", err)
	}

	resultBytes, _ := json.Marshal(got.Result)
	var gm GrepMatchResult
	if err := json.Unmarshal(resultBytes, &gm); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if len(gm.Content) != 1_500_000 {
		t.Errorf("content len = %d, want 1_500_000", len(gm.Content))
	}
}

// ---------------------------------------------------------------------------
// Partial reads / buffering (simulate a slow reader with small chunks)
// ---------------------------------------------------------------------------

type slowReader struct {
	data   []byte
	offset int
	chunk  int // bytes per Read call
}

func (s *slowReader) Read(p []byte) (int, error) {
	if s.offset >= len(s.data) {
		return 0, io.EOF
	}
	n := s.chunk
	if n > len(p) {
		n = len(p)
	}
	if s.offset+n > len(s.data) {
		n = len(s.data) - s.offset
	}
	copy(p, s.data[s.offset:s.offset+n])
	s.offset += n
	return n, nil
}

func TestDecoder_PartialReads(t *testing.T) {
	msg1 := `{"method":"ping","id":1}` + "\n"
	msg2 := `{"method":"pong","id":1}` + "\n"
	fullData := msg1 + msg2

	// Deliver 3 bytes at a time to stress the scanner's buffering.
	reader := &slowReader{data: []byte(fullData), chunk: 3}
	dec := NewDecoder(reader)

	raw1, err := dec.Receive()
	if err != nil {
		t.Fatalf("receive 1: %v", err)
	}
	var env1 Envelope
	if err := json.Unmarshal(raw1, &env1); err != nil {
		t.Fatal(err)
	}
	if env1.Method != MethodPing {
		t.Errorf("msg 1 method = %q, want %q", env1.Method, MethodPing)
	}

	raw2, err := dec.Receive()
	if err != nil {
		t.Fatalf("receive 2: %v", err)
	}
	var env2 Envelope
	if err := json.Unmarshal(raw2, &env2); err != nil {
		t.Fatal(err)
	}
	if env2.Method != MethodPong {
		t.Errorf("msg 2 method = %q, want %q", env2.Method, MethodPong)
	}

	// Should be EOF now.
	_, err = dec.Receive()
	if err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Blank lines are skipped
// ---------------------------------------------------------------------------

func TestDecoder_SkipsBlankLines(t *testing.T) {
	input := "\n\n" + `{"method":"ping","id":1}` + "\n\n"
	dec := NewDecoder(strings.NewReader(input))

	raw, err := dec.Receive()
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if env.Method != MethodPing {
		t.Errorf("method = %q, want %q", env.Method, MethodPing)
	}
}

// ---------------------------------------------------------------------------
// ParseRequest
// ---------------------------------------------------------------------------

func TestParseRequest_ValidWalk(t *testing.T) {
	raw := json.RawMessage(`{"method":"walk","id":5,"params":{"root":"/tmp","include_hidden":true}}`)
	method, id, params, err := ParseRequest(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if method != MethodWalk {
		t.Errorf("method = %q, want %q", method, MethodWalk)
	}
	if id != 5 {
		t.Errorf("id = %d, want 5", id)
	}

	var wp WalkParams
	if err := json.Unmarshal(params, &wp); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if wp.Root != "/tmp" {
		t.Errorf("root = %q, want /tmp", wp.Root)
	}
	if !wp.IncludeHidden {
		t.Error("include_hidden should be true")
	}
}

func TestParseRequest_Ping(t *testing.T) {
	raw := json.RawMessage(`{"method":"ping","id":1}`)
	method, id, params, err := ParseRequest(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if method != MethodPing {
		t.Errorf("method = %q", method)
	}
	if id != 1 {
		t.Errorf("id = %d", id)
	}
	if params != nil {
		t.Errorf("params should be nil for ping, got %s", string(params))
	}
}

func TestParseRequest_Cancel(t *testing.T) {
	raw := json.RawMessage(`{"method":"cancel","id":10,"params":{"cancel_id":3}}`)
	method, id, params, err := ParseRequest(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if method != MethodCancel {
		t.Errorf("method = %q", method)
	}
	if id != 10 {
		t.Errorf("id = %d", id)
	}

	var cp CancelParams
	if err := json.Unmarshal(params, &cp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cp.CancelID != 3 {
		t.Errorf("cancel_id = %d, want 3", cp.CancelID)
	}
}

func TestParseRequest_MissingMethod(t *testing.T) {
	raw := json.RawMessage(`{"id":1}`)
	_, _, _, err := ParseRequest(raw)
	if err == nil {
		t.Fatal("expected error for missing method")
	}
}

func TestParseRequest_MalformedJSON(t *testing.T) {
	raw := json.RawMessage(`{not valid}`)
	_, _, _, err := ParseRequest(raw)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestParseRequest_NoParams(t *testing.T) {
	raw := json.RawMessage(`{"method":"ping","id":42}`)
	method, id, params, err := ParseRequest(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if method != "ping" {
		t.Errorf("method = %q", method)
	}
	if id != 42 {
		t.Errorf("id = %d", id)
	}
	if params != nil {
		t.Errorf("params should be nil, got %s", string(params))
	}
}

// ---------------------------------------------------------------------------
// End-to-end: Encoder → pipe → Decoder → ParseRequest
// ---------------------------------------------------------------------------

func TestCodec_EndToEnd_WithParseRequest(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)

	// Send a walk request.
	env := Envelope{
		Method: MethodWalk,
		ID:     7,
		Params: WalkParams{Root: "/srv/app", MaxDepth: 20},
	}
	if err := enc.Send(env); err != nil {
		t.Fatal(err)
	}

	// Send a cancel.
	cancel := Envelope{
		Method: MethodCancel,
		ID:     8,
		Params: CancelParams{CancelID: 7},
	}
	if err := enc.Send(cancel); err != nil {
		t.Fatal(err)
	}

	// Decode and parse both.
	dec := NewDecoder(&buf)

	// First message: walk
	raw1, err := dec.Receive()
	if err != nil {
		t.Fatal(err)
	}
	method1, id1, params1, err := ParseRequest(raw1)
	if err != nil {
		t.Fatal(err)
	}
	if method1 != MethodWalk || id1 != 7 {
		t.Errorf("msg1: method=%q id=%d", method1, id1)
	}
	var wp WalkParams
	if err := json.Unmarshal(params1, &wp); err != nil {
		t.Fatal(err)
	}
	if wp.Root != "/srv/app" || wp.MaxDepth != 20 {
		t.Errorf("walk params: root=%q max_depth=%d", wp.Root, wp.MaxDepth)
	}

	// Second message: cancel
	raw2, err := dec.Receive()
	if err != nil {
		t.Fatal(err)
	}
	method2, id2, params2, err := ParseRequest(raw2)
	if err != nil {
		t.Fatal(err)
	}
	if method2 != MethodCancel || id2 != 8 {
		t.Errorf("msg2: method=%q id=%d", method2, id2)
	}
	var cp CancelParams
	if err := json.Unmarshal(params2, &cp); err != nil {
		t.Fatal(err)
	}
	if cp.CancelID != 7 {
		t.Errorf("cancel_id = %d, want 7", cp.CancelID)
	}

	// EOF
	_, err = dec.Receive()
	if err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
}

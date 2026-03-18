package remote

import (
	"encoding/json"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Envelope
// ---------------------------------------------------------------------------

func TestEnvelope_MarshalRequest(t *testing.T) {
	env := Envelope{
		Method: MethodWalk,
		ID:     1,
		Params: WalkParams{
			Root:          "/home/user/project",
			IncludeHidden: true,
			MaxDepth:      50,
		},
	}

	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Envelope
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Method != MethodWalk {
		t.Errorf("method = %q, want %q", got.Method, MethodWalk)
	}
	if got.ID != 1 {
		t.Errorf("id = %d, want 1", got.ID)
	}
	if got.Done {
		t.Error("done should be false for a request")
	}
	if got.Error != nil {
		t.Errorf("error should be nil, got %v", got.Error)
	}
}

func TestEnvelope_MarshalStreamResponse(t *testing.T) {
	env := Envelope{
		Method: MethodWalkEntry,
		ID:     1,
		Result: FileEntryResult{
			Path:    "/home/user/project/main.go",
			RelPath: "main.go",
			Name:    "main.go",
			Ext:     ".go",
			Size:    1234,
			ModTime: time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		},
		Done: false,
	}

	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Envelope
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Method != MethodWalkEntry {
		t.Errorf("method = %q, want %q", got.Method, MethodWalkEntry)
	}
	if got.Done {
		t.Error("done should be false for a streamed entry")
	}
}

func TestEnvelope_MarshalDone(t *testing.T) {
	env := Envelope{
		Method: MethodWalkEntry,
		ID:     1,
		Done:   true,
	}

	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Envelope
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !got.Done {
		t.Error("done should be true")
	}
	if got.Result != nil {
		t.Error("result should be nil on a done-only message")
	}
}

func TestEnvelope_MarshalError(t *testing.T) {
	env := Envelope{
		Method: MethodWalk,
		ID:     3,
		Error: &ProtoError{
			Code:    ErrCodeNotFound,
			Message: "path /nonexistent does not exist",
		},
		Done: true,
	}

	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Envelope
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Error == nil {
		t.Fatal("expected error, got nil")
	}
	if got.Error.Code != ErrCodeNotFound {
		t.Errorf("error code = %d, want %d", got.Error.Code, ErrCodeNotFound)
	}
	if got.Error.Message != "path /nonexistent does not exist" {
		t.Errorf("error message = %q", got.Error.Message)
	}
}

// ---------------------------------------------------------------------------
// ProtoError
// ---------------------------------------------------------------------------

func TestProtoError_ImplementsError(t *testing.T) {
	var err error = &ProtoError{Code: ErrCodeInternal, Message: "something broke"}
	if err.Error() != "something broke" {
		t.Errorf("Error() = %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Request params — round-trip
// ---------------------------------------------------------------------------

func TestWalkParams_RoundTrip(t *testing.T) {
	orig := WalkParams{
		Root:           "/home/user/project",
		IncludeHidden:  true,
		FollowSymlinks: false,
		MaxDepth:       42,
		IgnorePatterns: []string{"*.log", "tmp/"},
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got WalkParams
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Root != orig.Root {
		t.Errorf("root = %q, want %q", got.Root, orig.Root)
	}
	if got.IncludeHidden != orig.IncludeHidden {
		t.Errorf("include_hidden = %v, want %v", got.IncludeHidden, orig.IncludeHidden)
	}
	if got.MaxDepth != orig.MaxDepth {
		t.Errorf("max_depth = %d, want %d", got.MaxDepth, orig.MaxDepth)
	}
	if len(got.IgnorePatterns) != 2 {
		t.Errorf("ignore_patterns len = %d, want 2", len(got.IgnorePatterns))
	}
}

func TestGrepParams_RoundTrip(t *testing.T) {
	orig := GrepParams{
		Root:          "/srv/app",
		Query:         "func main",
		MaxResults:    100,
		IncludeHidden: false,
		ProjectOnly:   true,
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got GrepParams
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Query != orig.Query {
		t.Errorf("query = %q, want %q", got.Query, orig.Query)
	}
	if got.ProjectOnly != orig.ProjectOnly {
		t.Errorf("project_only = %v, want %v", got.ProjectOnly, orig.ProjectOnly)
	}
}

func TestSymbolsParams_RoundTrip(t *testing.T) {
	orig := SymbolsParams{Root: "/srv/app", KindFilter: "func"}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got SymbolsParams
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.KindFilter != "func" {
		t.Errorf("kind_filter = %q, want %q", got.KindFilter, "func")
	}
}

func TestReadIgnoreParams_RoundTrip(t *testing.T) {
	orig := ReadIgnoreParams{Root: "/home/user/project"}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got ReadIgnoreParams
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Root != orig.Root {
		t.Errorf("root = %q, want %q", got.Root, orig.Root)
	}
}

func TestDetectRootParams_RoundTrip(t *testing.T) {
	orig := DetectRootParams{StartDir: "/home/user/project/src"}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got DetectRootParams
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.StartDir != orig.StartDir {
		t.Errorf("start_dir = %q, want %q", got.StartDir, orig.StartDir)
	}
}

func TestCancelParams_RoundTrip(t *testing.T) {
	orig := CancelParams{CancelID: 7}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got CancelParams
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.CancelID != 7 {
		t.Errorf("cancel_id = %d, want 7", got.CancelID)
	}
}

func TestPingParams_RoundTrip(t *testing.T) {
	orig := PingParams{}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got PingParams
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	// No fields to check, just ensure it doesn't fail.
	_ = got
}

// ---------------------------------------------------------------------------
// Response results — round-trip
// ---------------------------------------------------------------------------

func TestFileEntryResult_RoundTrip(t *testing.T) {
	orig := FileEntryResult{
		Path:    "/home/user/project/internal/search/index.go",
		RelPath: "internal/search/index.go",
		Name:    "index.go",
		Ext:     ".go",
		Size:    12439,
		ModTime: time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC),
		IsDir:   false,
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got FileEntryResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Path != orig.Path {
		t.Errorf("path = %q, want %q", got.Path, orig.Path)
	}
	if got.Size != orig.Size {
		t.Errorf("size = %d, want %d", got.Size, orig.Size)
	}
	if !got.ModTime.Equal(orig.ModTime) {
		t.Errorf("mod_time = %v, want %v", got.ModTime, orig.ModTime)
	}
}

func TestGrepMatchResult_RoundTrip(t *testing.T) {
	orig := GrepMatchResult{
		FilePath: "/home/user/project/main.go",
		RelPath:  "main.go",
		FileName: "main.go",
		Line:     42,
		Column:   5,
		Content:  "func main() {",
		MatchRanges: []MatchRange{
			{Start: 5, End: 9},
		},
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got GrepMatchResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Line != 42 {
		t.Errorf("line = %d, want 42", got.Line)
	}
	if len(got.MatchRanges) != 1 {
		t.Fatalf("match_ranges len = %d, want 1", len(got.MatchRanges))
	}
	if got.MatchRanges[0].Start != 5 || got.MatchRanges[0].End != 9 {
		t.Errorf("match range = %+v, want {5 9}", got.MatchRanges[0])
	}
}

func TestSymbolEntryResult_RoundTrip(t *testing.T) {
	orig := SymbolEntryResult{
		Name:      "NewIndex",
		Signature: "func NewIndex() *Index",
		Kind:      "func",
		FilePath:  "/home/user/project/internal/search/index.go",
		RelPath:   "internal/search/index.go",
		FileName:  "index.go",
		Line:      68,
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got SymbolEntryResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "NewIndex" {
		t.Errorf("name = %q, want %q", got.Name, "NewIndex")
	}
	if got.Kind != "func" {
		t.Errorf("kind = %q, want %q", got.Kind, "func")
	}
}

func TestReadIgnoreResult_RoundTrip(t *testing.T) {
	orig := ReadIgnoreResult{
		Patterns: []string{"*.log", "tmp/", "node_modules/"},
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got ReadIgnoreResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Patterns) != 3 {
		t.Errorf("patterns len = %d, want 3", len(got.Patterns))
	}
}

func TestDetectRootResult_RoundTrip(t *testing.T) {
	orig := DetectRootResult{Root: "/home/user/project"}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got DetectRootResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Root != orig.Root {
		t.Errorf("root = %q, want %q", got.Root, orig.Root)
	}
}

func TestPongResult_RoundTrip(t *testing.T) {
	orig := PongResult{}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got PongResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	_ = got
}

// ---------------------------------------------------------------------------
// JSON field name verification
// ---------------------------------------------------------------------------

func TestEnvelope_JSONFieldNames(t *testing.T) {
	env := Envelope{
		Method: MethodPing,
		ID:     99,
		Done:   true,
		Error:  &ProtoError{Code: 1, Message: "test"},
	}
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}

	requiredKeys := []string{"method", "id", "done", "error"}
	for _, key := range requiredKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q in marshalled envelope", key)
		}
	}
}

func TestWalkParams_OmitsZeroValues(t *testing.T) {
	params := WalkParams{Root: "/tmp"}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}

	// These should be omitted because they are zero/false.
	omitted := []string{"include_hidden", "follow_symlinks", "max_depth", "ignore_patterns"}
	for _, key := range omitted {
		if _, ok := raw[key]; ok {
			t.Errorf("key %q should be omitted for zero value, but was present", key)
		}
	}
}

// ---------------------------------------------------------------------------
// Method constants
// ---------------------------------------------------------------------------

func TestMethodConstants_AreDistinct(t *testing.T) {
	methods := []string{
		MethodWalk, MethodWalkEntry,
		MethodGrep, MethodGrepMatch,
		MethodSymbols, MethodSymbolEntry,
		MethodReadIgnore, MethodDetectRoot,
		MethodCancel,
		MethodPing, MethodPong,
	}

	seen := make(map[string]bool)
	for _, m := range methods {
		if m == "" {
			t.Error("method constant is empty")
		}
		if seen[m] {
			t.Errorf("duplicate method constant: %q", m)
		}
		seen[m] = true
	}
}

// ---------------------------------------------------------------------------
// Error codes
// ---------------------------------------------------------------------------

func TestErrorCodes_AreDistinct(t *testing.T) {
	codes := []int{ErrCodeNotFound, ErrCodePermissionDenied, ErrCodeCancelled, ErrCodeInternal}
	seen := make(map[int]bool)
	for _, c := range codes {
		if c == 0 {
			t.Error("error code is zero")
		}
		if seen[c] {
			t.Errorf("duplicate error code: %d", c)
		}
		seen[c] = true
	}
}

package remote

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// ---------------------------------------------------------------------------
// detect_root
// ---------------------------------------------------------------------------

// setupDetectRootFixture creates a directory tree that simulates a Go project:
//
//	root/
//	  go.mod
//	  src/
//	    pkg/
//	      deep/
func setupDetectRootFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	os.MkdirAll(filepath.Join(root, "src", "pkg", "deep"), 0o755)
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n"), 0o644)

	return root
}

func TestHandleDetectRoot_FindsGoModRoot(t *testing.T) {
	root := setupDetectRootFixture(t)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodDetectRoot, HandleDetectRoot)
	defer h.stop()

	// Call from a deep subdirectory.
	startDir := filepath.Join(root, "src", "pkg", "deep")

	if err := h.clientEnc.Send(Envelope{
		Method: MethodDetectRoot,
		ID:     1,
		Params: DetectRootParams{StartDir: startDir},
	}); err != nil {
		t.Fatal(err)
	}

	raw, err := h.clientDec.Receive()
	if err != nil {
		t.Fatal(err)
	}

	resp, parseErr := ParseResponse(raw)
	if parseErr != nil {
		t.Fatalf("unexpected error: %v", parseErr)
	}
	if !resp.Done {
		t.Error("expected done=true for one-shot response")
	}

	resultBytes, _ := json.Marshal(resp.Result)
	var result DetectRootResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		t.Fatal(err)
	}

	if result.Root != root {
		t.Errorf("root = %q, want %q", result.Root, root)
	}
}

func TestHandleDetectRoot_FallsBackToStartDir(t *testing.T) {
	// A directory without any markers.
	noMarker := t.TempDir()

	h := newAgentTestHarness(t)
	h.server.Handle(MethodDetectRoot, HandleDetectRoot)
	defer h.stop()

	if err := h.clientEnc.Send(Envelope{
		Method: MethodDetectRoot,
		ID:     1,
		Params: DetectRootParams{StartDir: noMarker},
	}); err != nil {
		t.Fatal(err)
	}

	raw, err := h.clientDec.Receive()
	if err != nil {
		t.Fatal(err)
	}

	resp, parseErr := ParseResponse(raw)
	if parseErr != nil {
		t.Fatal(parseErr)
	}

	resultBytes, _ := json.Marshal(resp.Result)
	var result DetectRootResult
	json.Unmarshal(resultBytes, &result)

	// Should fall back to the start dir itself.
	if result.Root != noMarker {
		t.Errorf("root = %q, want %q", result.Root, noMarker)
	}
}

func TestHandleDetectRoot_NotFoundStartDir(t *testing.T) {
	h := newAgentTestHarness(t)
	h.server.Handle(MethodDetectRoot, HandleDetectRoot)
	defer h.stop()

	if err := h.clientEnc.Send(Envelope{
		Method: MethodDetectRoot,
		ID:     1,
		Params: DetectRootParams{StartDir: "/nonexistent/dir"},
	}); err != nil {
		t.Fatal(err)
	}

	raw, err := h.clientDec.Receive()
	if err != nil {
		t.Fatal(err)
	}

	_, parseErr := ParseResponse(raw)
	pe, ok := IsProtoError(parseErr)
	if !ok {
		t.Fatal("expected ProtoError")
	}
	if pe.Code != ErrCodeNotFound {
		t.Errorf("code = %d, want %d", pe.Code, ErrCodeNotFound)
	}
}

func TestHandleDetectRoot_EmptyStartDir(t *testing.T) {
	h := newAgentTestHarness(t)
	h.server.Handle(MethodDetectRoot, HandleDetectRoot)
	defer h.stop()

	if err := h.clientEnc.Send(Envelope{
		Method: MethodDetectRoot,
		ID:     1,
		Params: DetectRootParams{StartDir: ""},
	}); err != nil {
		t.Fatal(err)
	}

	raw, err := h.clientDec.Receive()
	if err != nil {
		t.Fatal(err)
	}

	_, parseErr := ParseResponse(raw)
	if parseErr == nil {
		t.Fatal("expected error for empty start_dir")
	}
}

func TestHandleDetectRoot_GitRepo(t *testing.T) {
	root := t.TempDir()
	// Create a .git directory (simulating a git repo).
	os.MkdirAll(filepath.Join(root, ".git"), 0o755)
	os.MkdirAll(filepath.Join(root, "src", "deep"), 0o755)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodDetectRoot, HandleDetectRoot)
	defer h.stop()

	if err := h.clientEnc.Send(Envelope{
		Method: MethodDetectRoot,
		ID:     1,
		Params: DetectRootParams{StartDir: filepath.Join(root, "src", "deep")},
	}); err != nil {
		t.Fatal(err)
	}

	raw, err := h.clientDec.Receive()
	if err != nil {
		t.Fatal(err)
	}

	resp, parseErr := ParseResponse(raw)
	if parseErr != nil {
		t.Fatal(parseErr)
	}

	resultBytes, _ := json.Marshal(resp.Result)
	var result DetectRootResult
	json.Unmarshal(resultBytes, &result)

	if result.Root != root {
		t.Errorf("root = %q, want %q", result.Root, root)
	}
}

// ---------------------------------------------------------------------------
// read_ignore
// ---------------------------------------------------------------------------

func setupReadIgnoreFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	gitignore := "# comment\nnode_modules/\n*.log\nbuild/\n\n"
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte(gitignore), 0o644)

	bmignore := "tmp/\n.cache/\n"
	os.WriteFile(filepath.Join(root, ".bmignore"), []byte(bmignore), 0o644)

	return root
}

func TestHandleReadIgnore_ReturnsPatterns(t *testing.T) {
	root := setupReadIgnoreFixture(t)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodReadIgnore, HandleReadIgnore)
	defer h.stop()

	if err := h.clientEnc.Send(Envelope{
		Method: MethodReadIgnore,
		ID:     1,
		Params: ReadIgnoreParams{Root: root},
	}); err != nil {
		t.Fatal(err)
	}

	raw, err := h.clientDec.Receive()
	if err != nil {
		t.Fatal(err)
	}

	resp, parseErr := ParseResponse(raw)
	if parseErr != nil {
		t.Fatalf("unexpected error: %v", parseErr)
	}
	if !resp.Done {
		t.Error("expected done=true")
	}

	resultBytes, _ := json.Marshal(resp.Result)
	var result ReadIgnoreResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		t.Fatal(err)
	}

	// Should contain patterns from both .gitignore and .bmignore.
	sort.Strings(result.Patterns)
	expected := []string{"*.log", ".cache/", "build/", "node_modules/", "tmp/"}
	sort.Strings(expected)

	if len(result.Patterns) != len(expected) {
		t.Fatalf("patterns = %v, want %v", result.Patterns, expected)
	}

	for i := range expected {
		if result.Patterns[i] != expected[i] {
			t.Errorf("pattern %d: got %q, want %q", i, result.Patterns[i], expected[i])
		}
	}
}

func TestHandleReadIgnore_NoIgnoreFiles(t *testing.T) {
	root := t.TempDir() // empty, no ignore files

	h := newAgentTestHarness(t)
	h.server.Handle(MethodReadIgnore, HandleReadIgnore)
	defer h.stop()

	if err := h.clientEnc.Send(Envelope{
		Method: MethodReadIgnore,
		ID:     1,
		Params: ReadIgnoreParams{Root: root},
	}); err != nil {
		t.Fatal(err)
	}

	raw, err := h.clientDec.Receive()
	if err != nil {
		t.Fatal(err)
	}

	resp, parseErr := ParseResponse(raw)
	if parseErr != nil {
		t.Fatal(parseErr)
	}

	resultBytes, _ := json.Marshal(resp.Result)
	var result ReadIgnoreResult
	json.Unmarshal(resultBytes, &result)

	if len(result.Patterns) != 0 {
		t.Errorf("expected no patterns, got %v", result.Patterns)
	}
}

func TestHandleReadIgnore_GitignoreOnly(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte("vendor/\n"), 0o644)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodReadIgnore, HandleReadIgnore)
	defer h.stop()

	if err := h.clientEnc.Send(Envelope{
		Method: MethodReadIgnore,
		ID:     1,
		Params: ReadIgnoreParams{Root: root},
	}); err != nil {
		t.Fatal(err)
	}

	raw, err := h.clientDec.Receive()
	if err != nil {
		t.Fatal(err)
	}

	resp, _ := ParseResponse(raw)
	resultBytes, _ := json.Marshal(resp.Result)
	var result ReadIgnoreResult
	json.Unmarshal(resultBytes, &result)

	if len(result.Patterns) != 1 || result.Patterns[0] != "vendor/" {
		t.Errorf("patterns = %v, want [vendor/]", result.Patterns)
	}
}

func TestHandleReadIgnore_NotFoundRoot(t *testing.T) {
	h := newAgentTestHarness(t)
	h.server.Handle(MethodReadIgnore, HandleReadIgnore)
	defer h.stop()

	if err := h.clientEnc.Send(Envelope{
		Method: MethodReadIgnore,
		ID:     1,
		Params: ReadIgnoreParams{Root: "/nonexistent"},
	}); err != nil {
		t.Fatal(err)
	}

	raw, err := h.clientDec.Receive()
	if err != nil {
		t.Fatal(err)
	}

	_, parseErr := ParseResponse(raw)
	pe, ok := IsProtoError(parseErr)
	if !ok {
		t.Fatal("expected ProtoError")
	}
	if pe.Code != ErrCodeNotFound {
		t.Errorf("code = %d", pe.Code)
	}
}

func TestHandleReadIgnore_CommentsAndBlanksSkipped(t *testing.T) {
	root := t.TempDir()
	content := "# This is a comment\n\n*.tmp\n\n# Another comment\n*.bak\n"
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte(content), 0o644)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodReadIgnore, HandleReadIgnore)
	defer h.stop()

	if err := h.clientEnc.Send(Envelope{
		Method: MethodReadIgnore,
		ID:     1,
		Params: ReadIgnoreParams{Root: root},
	}); err != nil {
		t.Fatal(err)
	}

	raw, err := h.clientDec.Receive()
	if err != nil {
		t.Fatal(err)
	}

	resp, _ := ParseResponse(raw)
	resultBytes, _ := json.Marshal(resp.Result)
	var result ReadIgnoreResult
	json.Unmarshal(resultBytes, &result)

	if len(result.Patterns) != 2 {
		t.Errorf("expected 2 patterns (comments skipped), got %v", result.Patterns)
	}
}

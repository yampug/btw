package remote

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setupGrepFixture creates a temporary directory tree for grep testing:
//
//	root/
//	  main.go        — contains "func main()"
//	  util.go        — contains "func helper()" and "const Version"
//	  README.md      — contains "main documentation"
//	  .hidden.go     — contains "func secret()"
//	  binary.png     — a fake binary file
//	  vendor/
//	    dep.go       — contains "func vendorFunc()"
func setupGrepFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	os.MkdirAll(filepath.Join(root, "vendor"), 0o755)

	files := map[string]string{
		"main.go":      "package main\n\nfunc main() {\n\tfmt.Println(\"hello world\")\n}\n",
		"util.go":      "package main\n\nfunc helper() string {\n\treturn \"help\"\n}\n\nconst Version = \"1.0\"\n",
		"README.md":    "# Main Documentation\n\nThis is the main project.\n",
		".hidden.go":   "package main\n\nfunc secret() {}\n",
		"vendor/dep.go": "package dep\n\nfunc vendorFunc() {}\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Write a fake binary file.
	binaryData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A} // PNG header
	if err := os.WriteFile(filepath.Join(root, "binary.png"), binaryData, 0o644); err != nil {
		t.Fatal(err)
	}

	return root
}

// collectGrepResults sends a grep request and collects all streamed matches.
func collectGrepResults(t *testing.T, h *agentTestHarness, params GrepParams) []GrepMatchResult {
	t.Helper()

	if err := h.clientEnc.Send(Envelope{
		Method: MethodGrep,
		ID:     1,
		Params: params,
	}); err != nil {
		t.Fatal(err)
	}

	var results []GrepMatchResult
	for {
		raw, err := h.clientDec.Receive()
		if err != nil {
			t.Fatalf("receive: %v", err)
		}

		resp, parseErr := ParseResponse(raw)
		if parseErr != nil {
			t.Fatalf("unexpected error response: %v", parseErr)
		}

		if resp.Done {
			break
		}

		resultBytes, err := json.Marshal(resp.Result)
		if err != nil {
			t.Fatal(err)
		}
		var gm GrepMatchResult
		if err := json.Unmarshal(resultBytes, &gm); err != nil {
			t.Fatal(err)
		}
		results = append(results, gm)
	}

	return results
}

// ---------------------------------------------------------------------------
// Literal query returns matching results
// ---------------------------------------------------------------------------

func TestHandleGrep_LiteralQuery(t *testing.T) {
	root := setupGrepFixture(t)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodGrep, HandleGrep)
	defer h.stop()

	results := collectGrepResults(t, h, GrepParams{
		Root:  root,
		Query: "func main",
	})

	if len(results) == 0 {
		t.Fatal("expected at least one match for 'func main'")
	}

	found := false
	for _, r := range results {
		if r.FileName == "main.go" && r.Line == 3 {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected match in main.go:3, got: %+v", results)
	}
}

// ---------------------------------------------------------------------------
// Regex query returns matching results
// ---------------------------------------------------------------------------

func TestHandleGrep_RegexQuery(t *testing.T) {
	root := setupGrepFixture(t)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodGrep, HandleGrep)
	defer h.stop()

	// Regex: match any func definition.
	results := collectGrepResults(t, h, GrepParams{
		Root:  root,
		Query: "/func \\w+\\(",
	})

	if len(results) < 2 {
		t.Fatalf("expected at least 2 regex matches, got %d", len(results))
	}

	// Should find func main() and func helper().
	names := make(map[string]bool)
	for _, r := range results {
		names[r.FileName] = true
	}
	if !names["main.go"] {
		t.Error("expected match in main.go")
	}
	if !names["util.go"] {
		t.Error("expected match in util.go")
	}
}

// ---------------------------------------------------------------------------
// Binary files are skipped
// ---------------------------------------------------------------------------

func TestHandleGrep_BinaryFilesSkipped(t *testing.T) {
	root := setupGrepFixture(t)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodGrep, HandleGrep)
	defer h.stop()

	// Search for something that might match binary content.
	results := collectGrepResults(t, h, GrepParams{
		Root:  root,
		Query: "PNG",
	})

	for _, r := range results {
		if r.FileName == "binary.png" {
			t.Error("binary.png should be skipped")
		}
	}
}

// ---------------------------------------------------------------------------
// max_results caps output
// ---------------------------------------------------------------------------

func TestHandleGrep_MaxResults(t *testing.T) {
	root := setupGrepFixture(t)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodGrep, HandleGrep)
	defer h.stop()

	// "main" appears in multiple files (main.go, README.md).
	results := collectGrepResults(t, h, GrepParams{
		Root:       root,
		Query:      "main",
		MaxResults: 1,
	})

	if len(results) != 1 {
		t.Errorf("expected exactly 1 result with max_results=1, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// Cancellation stops the grep
// ---------------------------------------------------------------------------

func TestHandleGrep_CancelStopsGrep(t *testing.T) {
	// Create a large fixture to give time for cancellation.
	root := t.TempDir()
	for i := 0; i < 50; i++ {
		content := "package main\nfunc main() {}\n"
		for j := 0; j < 100; j++ {
			content += "// line padding for grep to scan\n"
		}
		os.WriteFile(
			filepath.Join(root, "file"+string(rune('a'+i/26))+string(rune('a'+i%26))+".go"),
			[]byte(content),
			0o644,
		)
	}

	h := newAgentTestHarness(t)
	h.server.Handle(MethodGrep, HandleGrep)
	defer h.stop()

	// Start grep with high max_results so it doesn't stop naturally.
	if err := h.clientEnc.Send(Envelope{
		Method: MethodGrep,
		ID:     1,
		Params: GrepParams{Root: root, Query: "main", MaxResults: 10000},
	}); err != nil {
		t.Fatal(err)
	}

	// Read a couple of results.
	for i := 0; i < 2; i++ {
		raw, err := h.clientDec.Receive()
		if err != nil {
			t.Fatalf("receive %d: %v", i, err)
		}
		_, parseErr := ParseResponse(raw)
		if parseErr != nil {
			// Already got an error (maybe cancelled from indexing), that's ok too.
			return
		}
	}

	// Send cancel.
	cancelStart := time.Now()
	if err := h.clientEnc.Send(Envelope{
		Method: MethodCancel,
		ID:     2,
		Params: CancelParams{CancelID: 1},
	}); err != nil {
		t.Fatal(err)
	}

	// Drain until done or cancelled.
	gotDone := false
	for {
		raw, err := h.clientDec.Receive()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		resp, parseErr := ParseResponse(raw)
		if parseErr != nil {
			if IsCancelled(parseErr) {
				gotDone = true
				break
			}
			break
		}
		if resp.Done {
			gotDone = true
			break
		}
	}

	elapsed := time.Since(cancelStart)
	if !gotDone {
		t.Error("grep stream did not terminate after cancel")
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("cancel took %v, expected < 500ms", elapsed)
	}
	t.Logf("cancel latency: %v", elapsed)
}

// ---------------------------------------------------------------------------
// project_only excludes vendor
// ---------------------------------------------------------------------------

func TestHandleGrep_ProjectOnlyExcludesVendor(t *testing.T) {
	root := setupGrepFixture(t)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodGrep, HandleGrep)
	defer h.stop()

	results := collectGrepResults(t, h, GrepParams{
		Root:        root,
		Query:       "vendorFunc",
		ProjectOnly: true,
	})

	for _, r := range results {
		if r.FileName == "dep.go" {
			t.Error("vendor/dep.go should be excluded when project_only=true")
		}
	}
}

// ---------------------------------------------------------------------------
// Hidden files excluded by default
// ---------------------------------------------------------------------------

func TestHandleGrep_HiddenExcluded(t *testing.T) {
	root := setupGrepFixture(t)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodGrep, HandleGrep)
	defer h.stop()

	results := collectGrepResults(t, h, GrepParams{
		Root:  root,
		Query: "secret",
	})

	for _, r := range results {
		if r.FileName == ".hidden.go" {
			t.Error(".hidden.go should be excluded by default")
		}
	}
}

// ---------------------------------------------------------------------------
// Hidden files included when requested
// ---------------------------------------------------------------------------

func TestHandleGrep_HiddenIncluded(t *testing.T) {
	root := setupGrepFixture(t)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodGrep, HandleGrep)
	defer h.stop()

	results := collectGrepResults(t, h, GrepParams{
		Root:          root,
		Query:         "secret",
		IncludeHidden: true,
	})

	found := false
	for _, r := range results {
		if r.FileName == ".hidden.go" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected .hidden.go match when include_hidden=true")
	}
}

// ---------------------------------------------------------------------------
// Match ranges are populated
// ---------------------------------------------------------------------------

func TestHandleGrep_MatchRanges(t *testing.T) {
	root := setupGrepFixture(t)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodGrep, HandleGrep)
	defer h.stop()

	results := collectGrepResults(t, h, GrepParams{
		Root:  root,
		Query: "func main",
	})

	for _, r := range results {
		if r.FileName == "main.go" && r.Line == 3 {
			if len(r.MatchRanges) == 0 {
				t.Error("expected match ranges to be populated")
			}
			return
		}
	}
	t.Error("did not find main.go:3 result to check match ranges")
}

// ---------------------------------------------------------------------------
// Error cases
// ---------------------------------------------------------------------------

func TestHandleGrep_NotFoundRoot(t *testing.T) {
	h := newAgentTestHarness(t)
	h.server.Handle(MethodGrep, HandleGrep)
	defer h.stop()

	if err := h.clientEnc.Send(Envelope{
		Method: MethodGrep,
		ID:     1,
		Params: GrepParams{Root: "/nonexistent", Query: "test"},
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

func TestHandleGrep_EmptyQuery(t *testing.T) {
	root := setupGrepFixture(t)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodGrep, HandleGrep)
	defer h.stop()

	if err := h.clientEnc.Send(Envelope{
		Method: MethodGrep,
		ID:     1,
		Params: GrepParams{Root: root, Query: ""},
	}); err != nil {
		t.Fatal(err)
	}

	raw, err := h.clientDec.Receive()
	if err != nil {
		t.Fatal(err)
	}
	_, parseErr := ParseResponse(raw)
	if parseErr == nil {
		t.Fatal("expected error for empty query")
	}
}

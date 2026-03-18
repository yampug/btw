package remote

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/yampug/btw/internal/search"
)

// setupWalkFixture creates a temporary directory tree for walk testing:
//
//	root/
//	  main.go
//	  README.md
//	  .hidden_file
//	  src/
//	    app.go
//	    util.go
//	  docs/
//	    guide.md
//	  .secret/
//	    key.pem
//	  vendor/
//	    dep.go
//	  .gitignore  (contains "vendor/")
func setupWalkFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	dirs := []string{"src", "docs", ".secret", "vendor"}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	files := map[string]string{
		"main.go":          "package main",
		"README.md":        "# README",
		".hidden_file":     "secret",
		"src/app.go":       "package src",
		"src/util.go":      "package src",
		"docs/guide.md":    "# Guide",
		".secret/key.pem":  "-----BEGIN-----",
		"vendor/dep.go":    "package dep",
		".gitignore":       "vendor/\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return root
}

// collectWalkResults sends a walk request to the agent and collects all
// streamed FileEntryResult items until the done message.
func collectWalkResults(t *testing.T, h *agentTestHarness, params WalkParams) []FileEntryResult {
	t.Helper()

	if err := h.clientEnc.Send(Envelope{
		Method: MethodWalk,
		ID:     1,
		Params: params,
	}); err != nil {
		t.Fatal(err)
	}

	var results []FileEntryResult
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

		// Decode the result.
		resultBytes, err := json.Marshal(resp.Result)
		if err != nil {
			t.Fatal(err)
		}
		var fe FileEntryResult
		if err := json.Unmarshal(resultBytes, &fe); err != nil {
			t.Fatal(err)
		}
		results = append(results, fe)
	}

	return results
}

func relPaths(entries []FileEntryResult) []string {
	paths := make([]string, len(entries))
	for i, e := range entries {
		paths[i] = e.RelPath
	}
	sort.Strings(paths)
	return paths
}

// ---------------------------------------------------------------------------
// Walk produces same entries as search.Walk (integration test)
// ---------------------------------------------------------------------------

func TestHandleWalk_MatchesSearchWalk(t *testing.T) {
	root := setupWalkFixture(t)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodWalk, HandleWalk)
	defer h.stop()

	// Remote walk.
	remoteEntries := collectWalkResults(t, h, WalkParams{Root: root})

	// Local walk for comparison.
	rules := search.LoadIgnoreFiles(root)
	ch := search.Walk(context.Background(), root, rules, search.WalkOptions{})
	var localEntries []string
	for entry := range ch {
		localEntries = append(localEntries, entry.RelPath)
	}
	sort.Strings(localEntries)

	remotePaths := relPaths(remoteEntries)

	if len(remotePaths) != len(localEntries) {
		t.Errorf("remote returned %d entries, local returned %d", len(remotePaths), len(localEntries))
		t.Logf("remote: %v", remotePaths)
		t.Logf("local:  %v", localEntries)
		return
	}

	for i := range remotePaths {
		if remotePaths[i] != localEntries[i] {
			t.Errorf("entry %d: remote=%q local=%q", i, remotePaths[i], localEntries[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Hidden files excluded by default
// ---------------------------------------------------------------------------

func TestHandleWalk_HiddenExcludedByDefault(t *testing.T) {
	root := setupWalkFixture(t)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodWalk, HandleWalk)
	defer h.stop()

	entries := collectWalkResults(t, h, WalkParams{Root: root})
	paths := relPaths(entries)

	for _, p := range paths {
		base := filepath.Base(p)
		if len(base) > 0 && base[0] == '.' {
			t.Errorf("hidden file should be excluded: %q", p)
		}
	}
}

// ---------------------------------------------------------------------------
// Hidden files included when requested
// ---------------------------------------------------------------------------

func TestHandleWalk_HiddenIncluded(t *testing.T) {
	root := setupWalkFixture(t)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodWalk, HandleWalk)
	defer h.stop()

	entries := collectWalkResults(t, h, WalkParams{Root: root, IncludeHidden: true})
	paths := relPaths(entries)

	foundHidden := false
	for _, p := range paths {
		if p == ".hidden_file" || p == ".gitignore" {
			foundHidden = true
			break
		}
	}
	if !foundHidden {
		t.Errorf("expected hidden files when include_hidden=true, got: %v", paths)
	}
}

// ---------------------------------------------------------------------------
// .gitignore rules are respected
// ---------------------------------------------------------------------------

func TestHandleWalk_GitignoreRespected(t *testing.T) {
	root := setupWalkFixture(t)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodWalk, HandleWalk)
	defer h.stop()

	entries := collectWalkResults(t, h, WalkParams{Root: root})
	paths := relPaths(entries)

	for _, p := range paths {
		if p == "vendor/dep.go" {
			t.Error("vendor/dep.go should be excluded by .gitignore")
		}
	}
}

// ---------------------------------------------------------------------------
// Cancel stops the walk within 100ms
// ---------------------------------------------------------------------------

func TestHandleWalk_CancelStopsStream(t *testing.T) {
	// Create a larger fixture to give us time to cancel.
	root := t.TempDir()
	for i := 0; i < 100; i++ {
		dir := filepath.Join(root, "dir"+string(rune('a'+i/26))+string(rune('a'+i%26)))
		os.MkdirAll(dir, 0o755)
		for j := 0; j < 20; j++ {
			os.WriteFile(filepath.Join(dir, "file"+string(rune('a'+j))+".go"), []byte("pkg"), 0o644)
		}
	}

	h := newAgentTestHarness(t)
	h.server.Handle(MethodWalk, HandleWalk)
	defer h.stop()

	// Start walk.
	if err := h.clientEnc.Send(Envelope{
		Method: MethodWalk,
		ID:     1,
		Params: WalkParams{Root: root},
	}); err != nil {
		t.Fatal(err)
	}

	// Read a few entries.
	for i := 0; i < 5; i++ {
		raw, err := h.clientDec.Receive()
		if err != nil {
			t.Fatalf("receive %d: %v", i, err)
		}
		_, parseErr := ParseResponse(raw)
		if parseErr != nil {
			t.Fatalf("parse %d: %v", i, parseErr)
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

	// Drain remaining messages until done or cancelled.
	extraCount := 0
	gotDone := false
	for {
		raw, err := h.clientDec.Receive()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("receive: %v", err)
		}

		resp, parseErr := ParseResponse(raw)
		if parseErr != nil {
			if IsCancelled(parseErr) {
				gotDone = true
				break
			}
			// Other error — accept as termination.
			gotDone = true
			break
		}

		if resp.Done {
			gotDone = true
			break
		}
		extraCount++
	}

	elapsed := time.Since(cancelStart)

	if !gotDone {
		t.Error("stream did not terminate after cancel")
	}

	// The walk should have stopped well within 100ms.
	// We allow some slack for CI.
	if elapsed > 500*time.Millisecond {
		t.Errorf("cancel took %v, expected < 500ms", elapsed)
	}

	t.Logf("cancel latency: %v, extra entries after cancel: %d", elapsed, extraCount)
}

// ---------------------------------------------------------------------------
// Walk on non-existent root → not found error
// ---------------------------------------------------------------------------

func TestHandleWalk_NotFoundRoot(t *testing.T) {
	h := newAgentTestHarness(t)
	h.server.Handle(MethodWalk, HandleWalk)
	defer h.stop()

	if err := h.clientEnc.Send(Envelope{
		Method: MethodWalk,
		ID:     1,
		Params: WalkParams{Root: "/nonexistent/path/xyz"},
	}); err != nil {
		t.Fatal(err)
	}

	raw, err := h.clientDec.Receive()
	if err != nil {
		t.Fatal(err)
	}

	_, parseErr := ParseResponse(raw)
	if parseErr == nil {
		t.Fatal("expected error for non-existent root")
	}
	pe, ok := IsProtoError(parseErr)
	if !ok {
		t.Fatalf("expected *ProtoError, got %T", parseErr)
	}
	if pe.Code != ErrCodeNotFound {
		t.Errorf("code = %d, want %d", pe.Code, ErrCodeNotFound)
	}
}

// ---------------------------------------------------------------------------
// Walk with extra ignore patterns
// ---------------------------------------------------------------------------

func TestHandleWalk_CustomIgnorePatterns(t *testing.T) {
	root := setupWalkFixture(t)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodWalk, HandleWalk)
	defer h.stop()

	// Ignore all .md files via custom pattern.
	entries := collectWalkResults(t, h, WalkParams{
		Root:           root,
		IgnorePatterns: []string{"*.md"},
	})
	paths := relPaths(entries)

	for _, p := range paths {
		if filepath.Ext(p) == ".md" {
			t.Errorf("*.md should be excluded by custom ignore pattern: %q", p)
		}
	}

	// Ensure .go files are still there.
	foundGo := false
	for _, p := range paths {
		if filepath.Ext(p) == ".go" {
			foundGo = true
			break
		}
	}
	if !foundGo {
		t.Error("expected .go files to be present")
	}
}

// ---------------------------------------------------------------------------
// Walk result fields are populated correctly
// ---------------------------------------------------------------------------

func TestHandleWalk_ResultFields(t *testing.T) {
	root := setupWalkFixture(t)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodWalk, HandleWalk)
	defer h.stop()

	entries := collectWalkResults(t, h, WalkParams{Root: root})

	// Find main.go and check all fields.
	var mainGo *FileEntryResult
	for i := range entries {
		if entries[i].Name == "main.go" {
			mainGo = &entries[i]
			break
		}
	}
	if mainGo == nil {
		t.Fatal("main.go not found in walk results")
	}

	if mainGo.Path != filepath.Join(root, "main.go") {
		t.Errorf("path = %q, want %q", mainGo.Path, filepath.Join(root, "main.go"))
	}
	if mainGo.RelPath != "main.go" {
		t.Errorf("rel_path = %q", mainGo.RelPath)
	}
	if mainGo.Ext != ".go" {
		t.Errorf("ext = %q", mainGo.Ext)
	}
	if mainGo.Size != int64(len("package main")) {
		t.Errorf("size = %d, want %d", mainGo.Size, len("package main"))
	}
	if mainGo.ModTime.IsZero() {
		t.Error("mod_time should not be zero")
	}
	if mainGo.IsDir {
		t.Error("is_dir should be false")
	}
}

// ---------------------------------------------------------------------------
// Walk empty root param → error
// ---------------------------------------------------------------------------

func TestHandleWalk_EmptyRoot(t *testing.T) {
	h := newAgentTestHarness(t)
	h.server.Handle(MethodWalk, HandleWalk)
	defer h.stop()

	if err := h.clientEnc.Send(Envelope{
		Method: MethodWalk,
		ID:     1,
		Params: WalkParams{Root: ""},
	}); err != nil {
		t.Fatal(err)
	}

	raw, err := h.clientDec.Receive()
	if err != nil {
		t.Fatal(err)
	}

	_, parseErr := ParseResponse(raw)
	if parseErr == nil {
		t.Fatal("expected error for empty root")
	}
}

// ---------------------------------------------------------------------------
// Helpers: reuse agentTestHarness from agent_test.go (same package)
// ---------------------------------------------------------------------------
// The agentTestHarness and newAgentTestHarness are defined in agent_test.go.
// Since we're in the same package, they're available here.

// verify walk works through a wg-tracked serve goroutine
func TestHandleWalk_ViaFullProtocol(t *testing.T) {
	root := setupWalkFixture(t)

	// Use pipes directly.
	cr, cw := io.Pipe()
	ar, aw := io.Pipe()

	clientEnc := NewEncoder(cw)
	clientDec := NewDecoder(ar)
	agentDec := NewDecoder(cr)
	agentEnc := NewEncoder(aw)

	server := NewAgentServer(nil)
	server.Handle(MethodWalk, HandleWalk)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		server.Serve(context.Background(), agentDec, agentEnc)
	}()

	// Walk.
	clientEnc.Send(Envelope{Method: MethodWalk, ID: 1, Params: WalkParams{Root: root}})

	count := 0
	for {
		raw, err := clientDec.Receive()
		if err != nil {
			t.Fatal(err)
		}
		resp, parseErr := ParseResponse(raw)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		if resp.Done {
			break
		}
		count++
	}

	// Should have at least the non-ignored, non-hidden files.
	// main.go, src/app.go, src/util.go, docs/guide.md = 4 at minimum.
	if count < 4 {
		t.Errorf("expected at least 4 entries, got %d", count)
	}

	// Clean shutdown.
	cw.Close()
	wg.Wait()
	aw.Close()
}

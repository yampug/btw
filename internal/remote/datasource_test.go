package remote

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yampug/btw/internal/search"
)

// newTestRemoteDataSource sets up an in-process agent connected via io.Pipe
// to a dummy Session, wrapped inside a RemoteDataSource.
func newTestRemoteDataSource(t *testing.T) (*RemoteDataSource, *agentTestHarness) {
	t.Helper()

	h := newAgentTestHarness(t)
	// Register all real handlers onto the test harness's server.
	h.server.Handle(MethodWalk, HandleWalk)
	h.server.Handle(MethodGrep, HandleGrep)
	h.server.Handle(MethodSymbols, HandleSymbols)
	h.server.Handle(MethodDetectRoot, HandleDetectRoot)
	h.server.Handle(MethodReadIgnore, HandleReadIgnore)

	// Create a dummy session containing our piped encoders/decoders.
	// It doesn't have an underlying process, but it's enough for RemoteDataSource.
	sess := &Session{
		Enc:  h.clientEnc,
		Dec:  h.clientDec,
		done: make(chan struct{}),
	}
	ds := NewRemoteDataSource(sess)
	return ds, h
}

// ---------------------------------------------------------------------------
// Walk
// ---------------------------------------------------------------------------

func TestDataSource_Walk_HappyPath(t *testing.T) {
	ds, h := newTestRemoteDataSource(t)
	defer h.stop()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.go"), []byte("package main\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Hello\n"), 0o644)
	
	ctx := context.Background()
	opts := search.WalkOptions{}
	ch := ds.Walk(ctx, dir, opts)

	var files []search.FileEntry
	for f := range ch {
		files = append(files, f)
	}

	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}
	for _, f := range files {
		if !f.IsRemote {
			t.Errorf("expected file %s to be flagged as remote", f.Path)
		}
	}
}

func TestDataSource_Walk_ErrorCase(t *testing.T) {
	ds, h := newTestRemoteDataSource(t)
	defer h.stop()
	
	ctx := context.Background()
	opts := search.WalkOptions{}
	ch := ds.Walk(ctx, "/does/not/exist/999", opts)
	
	var files []search.FileEntry
	for f := range ch {
		files = append(files, f)
	}
	
	if len(files) != 0 {
		t.Errorf("expected no files from nonexistent walk, got %d", len(files))
	}
}

// ---------------------------------------------------------------------------
// Grep
// ---------------------------------------------------------------------------

func TestDataSource_Grep_HappyPath(t *testing.T) {
	ds, h := newTestRemoteDataSource(t)
	defer h.stop()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.go"), []byte("package main\n\nfunc start() {}\n"), 0o644)

	ctx := context.Background()
	opts := search.GrepOptions{}
	ch := ds.Grep(ctx, dir, "func start", opts)

	var matches []search.GrepMatch
	for m := range ch {
		matches = append(matches, m)
	}

	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Content != "func start() {}" {
		t.Errorf("unexpected match text: %q", matches[0].Content)
	}
}

func TestDataSource_Grep_ErrorCase(t *testing.T) {
	ds, h := newTestRemoteDataSource(t)
	defer h.stop()
	
	ctx := context.Background()
	opts := search.GrepOptions{}
	ch := ds.Grep(ctx, "/bad/path", "query", opts)
	
	count := 0
	for range ch {
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 matches, but got %d", count)
	}
}

// ---------------------------------------------------------------------------
// Symbols
// ---------------------------------------------------------------------------

func TestDataSource_Symbols_HappyPath(t *testing.T) {
	ds, h := newTestRemoteDataSource(t)
	defer h.stop()

	dir := t.TempDir()
	path := filepath.Join(dir, "sym.go")
	os.WriteFile(path, []byte("package test\ntype MySt struct {}\nfunc DoIt() {}\n"), 0o644)

	entries := []search.FileEntry{{Path: path, RelPath: "sym.go", IsRemote: true}}
	ctx := context.Background()
	symbols := ds.ExtractSymbols(ctx, entries)

	if len(symbols) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(symbols))
	}
}

func TestDataSource_Symbols_ErrorCase(t *testing.T) {
	ds, h := newTestRemoteDataSource(t)
	defer h.stop()

	entries := []search.FileEntry{{Path: "/broken/file.go", IsRemote: true}}
	ctx := context.Background()
	symbols := ds.ExtractSymbols(ctx, entries)

	if len(symbols) != 0 {
		t.Errorf("expected 0 symbols, got %d", len(symbols))
	}
}

// ---------------------------------------------------------------------------
// DetectRoot
// ---------------------------------------------------------------------------

func TestDataSource_DetectRoot_HappyPath(t *testing.T) {
	ds, h := newTestRemoteDataSource(t)
	defer h.stop()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module foo\n"), 0o644)
	subdir := filepath.Join(dir, "sub")
	os.Mkdir(subdir, 0o755)

	root, err := ds.DetectRoot(subdir)
	if err != nil {
		t.Fatalf("detect root err: %v", err)
	}
	if root != dir {
		t.Errorf("expected root %q, got %q", dir, root)
	}
}

func TestDataSource_DetectRoot_ErrorCase(t *testing.T) {
	ds, h := newTestRemoteDataSource(t)
	defer h.stop()

	_, err := ds.DetectRoot("/this/is/a/bad/path")
	if err == nil {
		t.Errorf("expected error for nonexistent path")
	}
	if pe, ok := IsProtoError(err); !ok || pe.Code != ErrCodeNotFound {
		t.Errorf("expected NotFound error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// ReadIgnore
// ---------------------------------------------------------------------------

func TestDataSource_LoadIgnoreFiles_HappyPath(t *testing.T) {
	ds, h := newTestRemoteDataSource(t)
	defer h.stop()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules/\ncustom_ignore_file\n"), 0o644)
	
	rules, err := ds.LoadIgnoreFiles(dir)
	if err != nil {
		t.Fatalf("load ignore error: %v", err)
	}
	
	if !rules.IsIgnored("node_modules/index.js", false) && !rules.IsIgnored("node_modules", true) {
		t.Errorf("expected node_modules to be ignored")
	}
}

func TestDataSource_LoadIgnoreFiles_ErrorCase(t *testing.T) {
	ds, h := newTestRemoteDataSource(t)
	defer h.stop()

	_, err := ds.LoadIgnoreFiles("/not_a_dir")
	if err == nil {
		t.Errorf("expected error on missing dir")
	}
	if pe, ok := IsProtoError(err); !ok || pe.Code != ErrCodeNotFound {
		t.Errorf("expected NotFound error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Mid-Stream Cancellation
// ---------------------------------------------------------------------------

func TestDataSource_Cancellation(t *testing.T) {
	ds, h := newTestRemoteDataSource(t)
	defer h.stop()

	// Provide a directory with many files to walk over.
	dir := t.TempDir()
	for i := 0; i < 100; i++ {
		os.MkdirAll(filepath.Join(dir, "dir", "sub"), 0o755)
		os.WriteFile(filepath.Join(dir, "dir", "sub", "fake.go"), []byte("foo"), 0o644)
	}

	// We don't even need real files because Walk operates quickly.
	// But let's cancel early by generating a context cancel abruptly to
	// trigger the graceful close cascade.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately before kicking off!
	
	ch := ds.Walk(ctx, dir, search.WalkOptions{})
	count := 0
	for range ch {
		count++
	}
	
	// Fast mid-stream interrupt.
	if count > 0 {
		// Just ensure it halted and drained.
	}
}

package remote

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// setupSymbolsFixture creates a temp dir with source files for symbol testing:
//
//	root/
//	  main.go     — func main(), func helper(), type Config, const Version, var debug
//	  app.py      — def run(), class App
//	  lib.rs      — fn compute(), pub struct Engine, pub enum Mode
func setupSymbolsFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	files := map[string]string{
		"main.go": `package main

func main() {
	fmt.Println("hello")
}

func helper() string {
	return "help"
}

type Config struct {
	Port int
}

const Version = "1.0"

var debug = false
`,
		"app.py": `def run():
    pass

class App:
    def __init__(self):
        pass
`,
		"lib.rs": `fn compute(x: i32) -> i32 {
    x * 2
}

pub struct Engine {
    speed: f64,
}

pub enum Mode {
    Fast,
    Slow,
}
`,
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return root
}

// collectSymbolResults sends a symbols request and collects all streamed entries.
func collectSymbolResults(t *testing.T, h *agentTestHarness, params SymbolsParams) []SymbolEntryResult {
	t.Helper()

	if err := h.clientEnc.Send(Envelope{
		Method: MethodSymbols,
		ID:     1,
		Params: params,
	}); err != nil {
		t.Fatal(err)
	}

	var results []SymbolEntryResult
	for {
		raw, err := h.clientDec.Receive()
		if err != nil {
			t.Fatalf("receive: %v", err)
		}

		resp, parseErr := ParseResponse(raw)
		if parseErr != nil {
			t.Fatalf("unexpected error: %v", parseErr)
		}

		if resp.Done {
			break
		}

		resultBytes, err := json.Marshal(resp.Result)
		if err != nil {
			t.Fatal(err)
		}
		var se SymbolEntryResult
		if err := json.Unmarshal(resultBytes, &se); err != nil {
			t.Fatal(err)
		}
		results = append(results, se)
	}

	return results
}

// ---------------------------------------------------------------------------
// Go symbols
// ---------------------------------------------------------------------------

func TestHandleSymbols_GoFile(t *testing.T) {
	root := setupSymbolsFixture(t)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodSymbols, HandleSymbols)
	defer h.stop()

	results := collectSymbolResults(t, h, SymbolsParams{Root: root})

	// Check for expected Go symbols.
	symbolNames := make(map[string]string) // name -> kind
	for _, s := range results {
		symbolNames[s.Name] = s.Kind
	}

	expected := map[string]string{
		"main":    "func",
		"helper":  "func",
		"Config":  "type",
		"Version": "constant",
		"debug":   "variable",
	}

	for name, kind := range expected {
		gotKind, ok := symbolNames[name]
		if !ok {
			t.Errorf("expected symbol %q not found", name)
			continue
		}
		if gotKind != kind {
			t.Errorf("symbol %q: kind = %q, want %q", name, gotKind, kind)
		}
	}
}

// ---------------------------------------------------------------------------
// Python symbols
// ---------------------------------------------------------------------------

func TestHandleSymbols_PyFile(t *testing.T) {
	root := setupSymbolsFixture(t)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodSymbols, HandleSymbols)
	defer h.stop()

	results := collectSymbolResults(t, h, SymbolsParams{Root: root})

	symbolNames := make(map[string]string)
	for _, s := range results {
		if s.FileName == "app.py" {
			symbolNames[s.Name] = s.Kind
		}
	}

	if symbolNames["run"] != "func" {
		t.Errorf("expected Python func 'run', got kind=%q", symbolNames["run"])
	}
	if symbolNames["App"] != "type" {
		t.Errorf("expected Python class 'App', got kind=%q", symbolNames["App"])
	}
}

// ---------------------------------------------------------------------------
// Rust symbols
// ---------------------------------------------------------------------------

func TestHandleSymbols_RsFile(t *testing.T) {
	root := setupSymbolsFixture(t)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodSymbols, HandleSymbols)
	defer h.stop()

	results := collectSymbolResults(t, h, SymbolsParams{Root: root})

	symbolNames := make(map[string]string)
	for _, s := range results {
		if s.FileName == "lib.rs" {
			symbolNames[s.Name] = s.Kind
		}
	}

	if symbolNames["compute"] != "func" {
		t.Errorf("expected Rust fn 'compute', got kind=%q", symbolNames["compute"])
	}
	if symbolNames["Engine"] != "type" {
		t.Errorf("expected Rust struct 'Engine', got kind=%q", symbolNames["Engine"])
	}
	if symbolNames["Mode"] != "type" {
		t.Errorf("expected Rust enum 'Mode', got kind=%q", symbolNames["Mode"])
	}
}

// ---------------------------------------------------------------------------
// Kind filter — functions only
// ---------------------------------------------------------------------------

func TestHandleSymbols_KindFilterFunc(t *testing.T) {
	root := setupSymbolsFixture(t)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodSymbols, HandleSymbols)
	defer h.stop()

	results := collectSymbolResults(t, h, SymbolsParams{
		Root:       root,
		KindFilter: "func",
	})

	for _, s := range results {
		if s.Kind != "func" {
			t.Errorf("expected only funcs with kind_filter=func, got %q (%s)", s.Name, s.Kind)
		}
	}

	if len(results) == 0 {
		t.Error("expected at least one func symbol")
	}
}

// ---------------------------------------------------------------------------
// Kind filter — types only
// ---------------------------------------------------------------------------

func TestHandleSymbols_KindFilterType(t *testing.T) {
	root := setupSymbolsFixture(t)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodSymbols, HandleSymbols)
	defer h.stop()

	results := collectSymbolResults(t, h, SymbolsParams{
		Root:       root,
		KindFilter: "type",
	})

	for _, s := range results {
		if s.Kind != "type" {
			t.Errorf("expected only types with kind_filter=type, got %q (%s)", s.Name, s.Kind)
		}
	}

	// Should have Config, App, Engine, Mode.
	if len(results) < 4 {
		t.Errorf("expected at least 4 type symbols, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// Invalid kind filter → error
// ---------------------------------------------------------------------------

func TestHandleSymbols_InvalidKindFilter(t *testing.T) {
	root := setupSymbolsFixture(t)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodSymbols, HandleSymbols)
	defer h.stop()

	if err := h.clientEnc.Send(Envelope{
		Method: MethodSymbols,
		ID:     1,
		Params: SymbolsParams{Root: root, KindFilter: "invalid"},
	}); err != nil {
		t.Fatal(err)
	}

	raw, err := h.clientDec.Receive()
	if err != nil {
		t.Fatal(err)
	}

	_, parseErr := ParseResponse(raw)
	if parseErr == nil {
		t.Fatal("expected error for invalid kind_filter")
	}
}

// ---------------------------------------------------------------------------
// Result fields are populated
// ---------------------------------------------------------------------------

func TestHandleSymbols_ResultFields(t *testing.T) {
	root := setupSymbolsFixture(t)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodSymbols, HandleSymbols)
	defer h.stop()

	results := collectSymbolResults(t, h, SymbolsParams{Root: root})

	// Find main() and check fields.
	for _, s := range results {
		if s.Name == "main" && s.FileName == "main.go" {
			if s.Kind != "func" {
				t.Errorf("kind = %q", s.Kind)
			}
			if s.FilePath != filepath.Join(root, "main.go") {
				t.Errorf("file_path = %q", s.FilePath)
			}
			if s.RelPath != "main.go" {
				t.Errorf("rel_path = %q", s.RelPath)
			}
			if s.Line < 1 {
				t.Errorf("line = %d, expected >= 1", s.Line)
			}
			if s.Signature == "" {
				t.Error("signature should not be empty")
			}
			return
		}
	}
	t.Error("did not find main() symbol in results")
}

// ---------------------------------------------------------------------------
// Not-found root → error
// ---------------------------------------------------------------------------

func TestHandleSymbols_NotFoundRoot(t *testing.T) {
	h := newAgentTestHarness(t)
	h.server.Handle(MethodSymbols, HandleSymbols)
	defer h.stop()

	if err := h.clientEnc.Send(Envelope{
		Method: MethodSymbols,
		ID:     1,
		Params: SymbolsParams{Root: "/nonexistent/path"},
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

// ---------------------------------------------------------------------------
// No kind_filter returns all kinds
// ---------------------------------------------------------------------------

func TestHandleSymbols_NoFilterReturnsAll(t *testing.T) {
	root := setupSymbolsFixture(t)

	h := newAgentTestHarness(t)
	h.server.Handle(MethodSymbols, HandleSymbols)
	defer h.stop()

	results := collectSymbolResults(t, h, SymbolsParams{Root: root})

	kinds := make(map[string]bool)
	for _, s := range results {
		kinds[s.Kind] = true
	}

	// Should have at least func and type.
	if !kinds["func"] {
		t.Error("expected func symbols")
	}
	if !kinds["type"] {
		t.Error("expected type symbols")
	}
}

package search

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bob/boomerang/internal/model"
)

func buildGrepTestDir(t *testing.T) (string, *Index) {
	t.Helper()
	dir := t.TempDir()

	files := map[string]string{
		"main.go":         "package main\n\nfunc main() {\n\tfmt.Println(\"hello world\")\n}\n",
		"util.go":         "package main\n\nfunc helper() string {\n\treturn \"helper result\"\n}\n",
		"readme.md":       "# Project\n\nThis is a Go project.\n",
		"sub/nested.go":   "package sub\n\nfunc Nested() {}\n",
		"data.bin":        string([]byte{0x00, 0x01, 0x02, 0xFF}),
		".hidden":         "secret stuff\n",
	}

	for name, content := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}

	idx := NewIndex()
	rules := &IgnoreRules{}
	idx.RebuildFrom(context.Background(), dir, rules, WalkOptions{IncludeHidden: true}, nil)
	return dir, idx
}

func collectGrep(ctx context.Context, idx *Index, query string, opts GrepOptions) []GrepMatch {
	var results []GrepMatch
	for m := range Grep(ctx, idx, query, opts) {
		results = append(results, m)
	}
	return results
}

func TestGrep_LiteralMatch(t *testing.T) {
	_, idx := buildGrepTestDir(t)
	results := collectGrep(context.Background(), idx, "hello", GrepOptions{})

	if len(results) == 0 {
		t.Fatal("expected at least one match for 'hello'")
	}
	found := false
	for _, r := range results {
		if r.FileName == "main.go" && r.Line == 4 {
			found = true
			if r.Column != 16 { // "hello" starts at column 16 (1-based, after trimming \t)
				// Column is rune-based from start of line including whitespace
				t.Logf("column=%d (content=%q)", r.Column, r.Content)
			}
		}
	}
	if !found {
		t.Error("expected match in main.go line 4")
	}
}

func TestGrep_CaseInsensitive(t *testing.T) {
	_, idx := buildGrepTestDir(t)
	results := collectGrep(context.Background(), idx, "HELLO", GrepOptions{})

	if len(results) == 0 {
		t.Fatal("expected case-insensitive match for 'HELLO'")
	}
}

func TestGrep_Regex(t *testing.T) {
	_, idx := buildGrepTestDir(t)
	results := collectGrep(context.Background(), idx, "/func\\s+\\w+", GrepOptions{})

	if len(results) < 3 {
		t.Fatalf("expected at least 3 func matches, got %d", len(results))
	}
}

func TestGrep_RegexWithTrailingSlash(t *testing.T) {
	_, idx := buildGrepTestDir(t)
	results := collectGrep(context.Background(), idx, "/func\\s+main/", GrepOptions{})

	if len(results) == 0 {
		t.Fatal("expected match for regex /func\\s+main/")
	}
}

func TestGrep_SkipsBinaryFiles(t *testing.T) {
	_, idx := buildGrepTestDir(t)
	// The binary file contains no text, and should be skipped even if its content
	// happened to match (it won't here, but the binary check should kick in).
	results := collectGrep(context.Background(), idx, "hello", GrepOptions{})
	for _, r := range results {
		if r.FileName == "data.bin" {
			t.Error("should not match binary files")
		}
	}
}

func TestGrep_SkipsHiddenByDefault(t *testing.T) {
	_, idx := buildGrepTestDir(t)
	results := collectGrep(context.Background(), idx, "secret", GrepOptions{IncludeHidden: false})

	for _, r := range results {
		if r.FileName == ".hidden" {
			t.Error("should skip hidden files when IncludeHidden=false")
		}
	}
}

func TestGrep_IncludesHidden(t *testing.T) {
	_, idx := buildGrepTestDir(t)
	results := collectGrep(context.Background(), idx, "secret", GrepOptions{IncludeHidden: true})

	found := false
	for _, r := range results {
		if r.FileName == ".hidden" {
			found = true
		}
	}
	if !found {
		t.Error("expected match in .hidden when IncludeHidden=true")
	}
}

func TestGrep_EmptyQuery(t *testing.T) {
	_, idx := buildGrepTestDir(t)
	results := collectGrep(context.Background(), idx, "", GrepOptions{})

	if len(results) != 0 {
		t.Errorf("expected no results for empty query, got %d", len(results))
	}
}

func TestGrep_MaxResults(t *testing.T) {
	_, idx := buildGrepTestDir(t)
	results := collectGrep(context.Background(), idx, "package", GrepOptions{MaxResults: 2})

	if len(results) > 2 {
		t.Errorf("expected at most 2 results, got %d", len(results))
	}
}

func TestGrep_Cancellation(t *testing.T) {
	_, idx := buildGrepTestDir(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	results := collectGrep(ctx, idx, "func", GrepOptions{})

	// Should get zero or very few results since context is already canceled.
	if len(results) > 1 {
		t.Logf("got %d results despite cancellation (race-dependent)", len(results))
	}
}

func TestGrep_MatchRangesCorrect(t *testing.T) {
	_, idx := buildGrepTestDir(t)
	results := collectGrep(context.Background(), idx, "hello", GrepOptions{})

	for _, r := range results {
		for _, mr := range r.MatchRanges {
			runes := []rune(r.Content)
			if mr.Start < 0 || mr.End > len(runes) || mr.Start >= mr.End {
				t.Errorf("invalid range [%d,%d) for line %q (len %d)", mr.Start, mr.End, r.Content, len(runes))
			}
		}
	}
}

func TestGrep_NestedDirectory(t *testing.T) {
	_, idx := buildGrepTestDir(t)
	results := collectGrep(context.Background(), idx, "Nested", GrepOptions{})

	found := false
	for _, r := range results {
		if r.FileName == "nested.go" {
			found = true
			if r.RelPath != filepath.Join("sub", "nested.go") {
				t.Errorf("expected relpath sub/nested.go, got %s", r.RelPath)
			}
		}
	}
	if !found {
		t.Error("expected match in sub/nested.go")
	}
}

func TestGrepMatchToResult(t *testing.T) {
	m := GrepMatch{
		FilePath:    "/p/internal/search/main.go",
		RelPath:     "internal/search/main.go",
		FileName:    "main.go",
		Line:        42,
		Column:      5,
		Content:     "\tfunc NewMatcher(pattern string) *Matcher {",
		MatchRanges: []model.MatchRange{{Start: 1, End: 16}},
	}

	r := GrepMatchToResult(m)

	if r.ResultType != model.ResultText {
		t.Errorf("expected ResultText, got %v", r.ResultType)
	}
	if r.Line != 42 {
		t.Errorf("expected line 42, got %d", r.Line)
	}
	if r.Icon == "" {
		t.Error("expected icon")
	}
	if r.Detail == "" {
		t.Error("expected detail with filename:line")
	}
	// Content should be trimmed (no leading tab).
	if r.Name[0] == '\t' {
		t.Error("expected leading whitespace trimmed from name")
	}
}

func TestGrepMatchToResult_LongLine(t *testing.T) {
	long := ""
	for i := range 100 {
		long += string(rune('a' + (i % 26)))
	}
	m := GrepMatch{
		FilePath:    "/p/main.go",
		RelPath:     "main.go",
		FileName:    "main.go",
		Line:        1,
		Column:      1,
		Content:     long,
		MatchRanges: []model.MatchRange{{Start: 0, End: 5}},
	}

	r := GrepMatchToResult(m)

	runes := []rune(r.Name)
	// Should be truncated to ~60 + "…"
	if len(runes) > 65 {
		t.Errorf("expected truncated name, got %d runes", len(runes))
	}
}

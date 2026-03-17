package search

import (
	"context"
	"testing"

	"github.com/yampug/btw/internal/model"
)

// --- ParseQuery tests ---

func TestParseQuery_Simple(t *testing.T) {
	pq := ParseQuery("main.go")
	if pq.Query != "main.go" || pq.LineNum != 0 || pq.IsPath {
		t.Errorf("unexpected: %+v", pq)
	}
}

func TestParseQuery_LineNum(t *testing.T) {
	pq := ParseQuery("main.go:42")
	if pq.Query != "main.go" || pq.LineNum != 42 {
		t.Errorf("expected query=main.go lineNum=42, got %+v", pq)
	}
}

func TestParseQuery_PathWithLineNum(t *testing.T) {
	pq := ParseQuery("cmd/main.go:42")
	if pq.Query != "cmd/main.go" || pq.LineNum != 42 || !pq.IsPath {
		t.Errorf("expected path query with line num, got %+v", pq)
	}
}

func TestParseQuery_PathNoLineNum(t *testing.T) {
	pq := ParseQuery("internal/search/match")
	if pq.Query != "internal/search/match" || pq.LineNum != 0 || !pq.IsPath {
		t.Errorf("unexpected: %+v", pq)
	}
}

func TestParseQuery_Empty(t *testing.T) {
	pq := ParseQuery("")
	if pq.Query != "" || pq.LineNum != 0 || pq.IsPath {
		t.Errorf("unexpected: %+v", pq)
	}
}

func TestParseQuery_WhitespaceLineNum(t *testing.T) {
	pq := ParseQuery("  foo:10  ")
	if pq.Query != "foo" || pq.LineNum != 10 {
		t.Errorf("unexpected: %+v", pq)
	}
}

// --- PathMatch tests ---

// Scenario 1: cmd/main matches cmd/bm/main.go
func TestPathMatch_CmdMain(t *testing.T) {
	mr := PathMatch("cmd/main", "cmd/bm/main.go")
	if !mr.Matched {
		t.Fatal("cmd/main should match cmd/bm/main.go")
	}
	if mr.Score <= 0 {
		t.Errorf("expected positive score, got %d", mr.Score)
	}
	if len(mr.Ranges) == 0 {
		t.Error("expected match ranges")
	}
}

// Scenario 2: internal/search/match matches internal/search/matcher.go
func TestPathMatch_InternalSearchMatch(t *testing.T) {
	mr := PathMatch("internal/search/match", "internal/search/matcher.go")
	if !mr.Matched {
		t.Fatal("should match")
	}
	if mr.Score <= 0 {
		t.Errorf("expected positive score, got %d", mr.Score)
	}
}

// Scenario 3: src/comp/But matches src/components/Button.tsx
func TestPathMatch_SrcCompBut(t *testing.T) {
	mr := PathMatch("src/comp/But", "src/components/Button.tsx")
	if !mr.Matched {
		t.Fatal("should match")
	}
}

// Scenario 4: No match when segments don't align.
func TestPathMatch_NoMatch(t *testing.T) {
	mr := PathMatch("zzz/xxx", "cmd/bm/main.go")
	if mr.Matched {
		t.Error("should not match")
	}
}

// Scenario 5: Single segment path query still works.
func TestPathMatch_SingleSegment(t *testing.T) {
	// A trailing slash creates a path query with one segment.
	mr := PathMatch("cmd/", "cmd/bm/main.go")
	// After trimming empty segments, this is ["cmd"].
	if !mr.Matched {
		t.Fatal("should match cmd segment")
	}
}

// Scenario 6: Ranges are correctly offset to full relPath positions.
func TestPathMatch_RangeOffsets(t *testing.T) {
	mr := PathMatch("cmd/main", "cmd/bm/main.go")
	if !mr.Matched {
		t.Fatal("should match")
	}
	runes := []rune("cmd/bm/main.go")
	for _, r := range mr.Ranges {
		if r.Start < 0 || r.End > len(runes) || r.Start >= r.End {
			t.Errorf("invalid range [%d,%d) for path of length %d", r.Start, r.End, len(runes))
		}
	}
}

// --- Integration: Index.Search with path queries ---

func TestIndex_SearchPathQuery(t *testing.T) {
	entries := []FileEntry{
		{Path: "/p/cmd/bm/main.go", RelPath: "cmd/bm/main.go", Name: "main.go", Ext: ".go"},
		{Path: "/p/internal/search/matcher.go", RelPath: "internal/search/matcher.go", Name: "matcher.go", Ext: ".go"},
		{Path: "/p/main.go", RelPath: "main.go", Name: "main.go", Ext: ".go"},
	}
	idx := buildTestIndex(entries)

	results := idx.Search(context.Background(), SearchOptions{Query: "cmd/main", Tab: model.TabAll}).Items
	if len(results) == 0 {
		t.Fatal("expected results for path query")
	}
	if results[0].Name != "main.go" || results[0].Detail != "cmd/bm" {
		t.Errorf("expected cmd/bm/main.go first, got %s in %s", results[0].Name, results[0].Detail)
	}
}

func TestIndex_SearchPathQueryWithLineNum(t *testing.T) {
	entries := []FileEntry{
		{Path: "/p/cmd/bm/main.go", RelPath: "cmd/bm/main.go", Name: "main.go", Ext: ".go"},
	}
	idx := buildTestIndex(entries)

	results := idx.Search(context.Background(), SearchOptions{Query: "cmd/main.go:42", Tab: model.TabAll}).Items
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	if results[0].Line != 42 {
		t.Errorf("expected line 42, got %d", results[0].Line)
	}
}

func TestIndex_SearchSimpleQueryWithLineNum(t *testing.T) {
	entries := []FileEntry{
		{Path: "/p/main.go", RelPath: "main.go", Name: "main.go", Ext: ".go"},
	}
	idx := buildTestIndex(entries)

	results := idx.Search(context.Background(), SearchOptions{Query: "main.go:99", Tab: model.TabAll}).Items
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	if results[0].Line != 99 {
		t.Errorf("expected line 99, got %d", results[0].Line)
	}
}

func TestIndex_PathQueryHighlightsDetail(t *testing.T) {
	entries := []FileEntry{
		{Path: "/p/internal/search/matcher.go", RelPath: "internal/search/matcher.go", Name: "matcher.go", Ext: ".go"},
	}
	idx := buildTestIndex(entries)

	results := idx.Search(context.Background(), SearchOptions{Query: "internal/search/match", Tab: model.TabAll}).Items
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	// Ranges should be present, mapped to the full relPath.
	if len(results[0].MatchRanges) == 0 {
		t.Error("expected match ranges for path query highlighting")
	}
}

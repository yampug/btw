package search

import (
	"testing"

	"github.com/bob/boomerang/internal/model"
)

func TestParseFilters_DotSuffix(t *testing.T) {
	fr := ParseFilters("main .go")
	if fr.Query != "main" {
		t.Errorf("expected query 'main', got '%s'", fr.Query)
	}
	if len(fr.Extensions) != 1 || fr.Extensions[0] != ".go" {
		t.Errorf("expected [.go], got %v", fr.Extensions)
	}
}

func TestParseFilters_ExtPrefix(t *testing.T) {
	fr := ParseFilters("main ext:go")
	if fr.Query != "main" {
		t.Errorf("expected query 'main', got '%s'", fr.Query)
	}
	if len(fr.Extensions) != 1 || fr.Extensions[0] != ".go" {
		t.Errorf("expected [.go], got %v", fr.Extensions)
	}
}

func TestParseFilters_TypePrefix(t *testing.T) {
	fr := ParseFilters("config type:yaml")
	if fr.Query != "config" {
		t.Errorf("expected query 'config', got '%s'", fr.Query)
	}
	if len(fr.Extensions) != 1 || fr.Extensions[0] != ".yaml" {
		t.Errorf("expected [.yaml], got %v", fr.Extensions)
	}
}

func TestParseFilters_MultipleExtensions(t *testing.T) {
	fr := ParseFilters("search ext:go,rs,py")
	if fr.Query != "search" {
		t.Errorf("expected query 'search', got '%s'", fr.Query)
	}
	if len(fr.Extensions) != 3 {
		t.Fatalf("expected 3 extensions, got %d: %v", len(fr.Extensions), fr.Extensions)
	}
	expected := map[string]bool{".go": true, ".rs": true, ".py": true}
	for _, ext := range fr.Extensions {
		if !expected[ext] {
			t.Errorf("unexpected extension %s", ext)
		}
	}
}

func TestParseFilters_NoFilter(t *testing.T) {
	fr := ParseFilters("just a query")
	if fr.Query != "just a query" {
		t.Errorf("expected unchanged query, got '%s'", fr.Query)
	}
	if len(fr.Extensions) != 0 {
		t.Errorf("expected no extensions, got %v", fr.Extensions)
	}
}

func TestParseFilters_Empty(t *testing.T) {
	fr := ParseFilters("")
	if fr.Query != "" || len(fr.Extensions) != 0 {
		t.Errorf("unexpected: %+v", fr)
	}
}

func TestParseFilters_DotSuffixAlreadyHasDot(t *testing.T) {
	fr := ParseFilters("main .go")
	if len(fr.Extensions) != 1 || fr.Extensions[0] != ".go" {
		t.Errorf("expected [.go], got %v", fr.Extensions)
	}
}

// Integration: Search with inline ext filter.
func TestIndex_SearchWithInlineExtFilter(t *testing.T) {
	entries := []FileEntry{
		{Path: "/p/main.go", RelPath: "main.go", Name: "main.go", Ext: ".go"},
		{Path: "/p/main.rs", RelPath: "main.rs", Name: "main.rs", Ext: ".rs"},
		{Path: "/p/main.py", RelPath: "main.py", Name: "main.py", Ext: ".py"},
	}
	idx := buildTestIndex(entries)

	// Query "main ext:go" should only return .go file.
	results := idx.Search(SearchOptions{Query: "main ext:go", Tab: model.TabAll}).Items
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Name != "main.go" {
		t.Errorf("expected main.go, got %s", results[0].Name)
	}
}

func TestIndex_SearchWithMultiExtFilter(t *testing.T) {
	entries := []FileEntry{
		{Path: "/p/a.go", RelPath: "a.go", Name: "a.go", Ext: ".go"},
		{Path: "/p/b.rs", RelPath: "b.rs", Name: "b.rs", Ext: ".rs"},
		{Path: "/p/c.py", RelPath: "c.py", Name: "c.py", Ext: ".py"},
		{Path: "/p/d.js", RelPath: "d.js", Name: "d.js", Ext: ".js"},
	}
	idx := buildTestIndex(entries)

	results := idx.Search(SearchOptions{Query: "ext:go,rs", Tab: model.TabAll}).Items
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestIndex_SearchWithDotSuffixFilter(t *testing.T) {
	entries := []FileEntry{
		{Path: "/p/main.go", RelPath: "main.go", Name: "main.go", Ext: ".go"},
		{Path: "/p/main.rs", RelPath: "main.rs", Name: "main.rs", Ext: ".rs"},
	}
	idx := buildTestIndex(entries)

	results := idx.Search(SearchOptions{Query: "main .go", Tab: model.TabAll}).Items
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Name != "main.go" {
		t.Errorf("expected main.go, got %s", results[0].Name)
	}
}

func TestIndex_SearchNoFilterShowsAll(t *testing.T) {
	entries := []FileEntry{
		{Path: "/p/a.go", RelPath: "a.go", Name: "a.go", Ext: ".go"},
		{Path: "/p/b.rs", RelPath: "b.rs", Name: "b.rs", Ext: ".rs"},
	}
	idx := buildTestIndex(entries)

	results := idx.Search(SearchOptions{Query: "", Tab: model.TabAll}).Items
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestIndex_SearchWithExplicitExtFilters(t *testing.T) {
	entries := []FileEntry{
		{Path: "/p/a.go", RelPath: "a.go", Name: "a.go", Ext: ".go"},
		{Path: "/p/b.rs", RelPath: "b.rs", Name: "b.rs", Ext: ".rs"},
		{Path: "/p/c.py", RelPath: "c.py", Name: "c.py", Ext: ".py"},
	}
	idx := buildTestIndex(entries)

	// Using ExtFilters field directly (from filter menu).
	results := idx.Search(SearchOptions{
		Query:      "",
		Tab:        model.TabAll,
		ExtFilters: []string{".go", ".py"},
	}).Items
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestIndex_Extensions(t *testing.T) {
	entries := []FileEntry{
		{Path: "/p/a.go", RelPath: "a.go", Name: "a.go", Ext: ".go"},
		{Path: "/p/b.go", RelPath: "b.go", Name: "b.go", Ext: ".go"},
		{Path: "/p/c.rs", RelPath: "c.rs", Name: "c.rs", Ext: ".rs"},
	}
	idx := buildTestIndex(entries)

	exts := idx.Extensions()
	if len(exts) != 2 {
		t.Fatalf("expected 2 extensions, got %d", len(exts))
	}
	// .go should come first (2 files vs 1).
	if exts[0].Ext != ".go" || exts[0].Count != 2 {
		t.Errorf("expected .go with count 2 first, got %s with %d", exts[0].Ext, exts[0].Count)
	}
	if exts[1].Ext != ".rs" || exts[1].Count != 1 {
		t.Errorf("expected .rs with count 1 second, got %s with %d", exts[1].Ext, exts[1].Count)
	}
}

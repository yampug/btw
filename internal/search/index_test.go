package search

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/yampug/btw/internal/model"
)

func buildTestIndex(entries []FileEntry) *Index {
	idx := NewIndex()
	ch := make(chan FileEntry, len(entries))
	for _, e := range entries {
		ch <- e
	}
	close(ch)
	idx.BuildFrom(ch, nil)
	return idx
}

func TestIndex_BuildFrom(t *testing.T) {
	entries := []FileEntry{
		{Path: "/p/main.go", RelPath: "main.go", Name: "main.go", Ext: ".go"},
		{Path: "/p/util.go", RelPath: "util.go", Name: "util.go", Ext: ".go"},
		{Path: "/p/readme.md", RelPath: "readme.md", Name: "readme.md", Ext: ".md"},
	}
	idx := buildTestIndex(entries)

	if idx.Len() != 3 {
		t.Fatalf("expected 3 files, got %d", idx.Len())
	}
}

func TestIndex_SearchExact(t *testing.T) {
	entries := []FileEntry{
		{Path: "/p/main.go", RelPath: "main.go", Name: "main.go", Ext: ".go"},
		{Path: "/p/main_test.go", RelPath: "main_test.go", Name: "main_test.go", Ext: ".go"},
	}
	idx := buildTestIndex(entries)

	results := idx.Search(context.Background(), SearchOptions{Query: "main.go", Tab: model.TabAll}).Items
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	if results[0].Name != "main.go" {
		t.Errorf("expected main.go first, got %s", results[0].Name)
	}
	// Exact match should have the highest score.
	if len(results) > 1 && results[0].Score <= results[1].Score {
		t.Errorf("exact match should rank first: %d <= %d", results[0].Score, results[1].Score)
	}
}

func TestIndex_SearchPrefix(t *testing.T) {
	entries := []FileEntry{
		{Path: "/p/config.yaml", RelPath: "config.yaml", Name: "config.yaml", Ext: ".yaml"},
		{Path: "/p/controller.go", RelPath: "controller.go", Name: "controller.go", Ext: ".go"},
	}
	idx := buildTestIndex(entries)

	results := idx.Search(context.Background(), SearchOptions{Query: "con", Tab: model.TabAll}).Items
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Both should have prefix-level scores (base 800 + bonuses).
	for _, r := range results {
		if r.Score < 800 {
			t.Errorf("expected prefix score >= 800 for %s, got %d", r.Name, r.Score)
		}
	}
}

func TestIndex_SearchSubstring(t *testing.T) {
	entries := []FileEntry{
		{Path: "/p/UserModel.go", RelPath: "UserModel.go", Name: "UserModel.go", Ext: ".go"},
	}
	idx := buildTestIndex(entries)

	results := idx.Search(context.Background(), SearchOptions{Query: "odel", Tab: model.TabAll}).Items
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// Substring base is 500 + contextual bonuses.
	if results[0].Score < 500 {
		t.Errorf("expected substring score >= 500, got %d", results[0].Score)
	}
}

func TestIndex_SearchSubsequence(t *testing.T) {
	entries := []FileEntry{
		{Path: "/p/main.go", RelPath: "main.go", Name: "main.go", Ext: ".go"},
	}
	idx := buildTestIndex(entries)

	results := idx.Search(context.Background(), SearchOptions{Query: "mgo", Tab: model.TabAll}).Items
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// Subsequence with contextual bonuses (boundary alignment for m and g).
	if results[0].Score <= 0 {
		t.Errorf("expected positive score, got %d", results[0].Score)
	}
}

func TestIndex_ExtFilter(t *testing.T) {
	entries := []FileEntry{
		{Path: "/p/a.go", RelPath: "a.go", Name: "a.go", Ext: ".go"},
		{Path: "/p/b.ts", RelPath: "b.ts", Name: "b.ts", Ext: ".ts"},
		{Path: "/p/c.go", RelPath: "c.go", Name: "c.go", Ext: ".go"},
	}
	idx := buildTestIndex(entries)

	results := idx.Search(context.Background(), SearchOptions{Query: "", Tab: model.TabAll, ExtFilter: ".go"}).Items
	if len(results) != 2 {
		t.Fatalf("expected 2 .go results, got %d", len(results))
	}
}

func TestIndex_EmptyQuery(t *testing.T) {
	entries := []FileEntry{
		{Path: "/p/a.go", RelPath: "a.go", Name: "a.go", Ext: ".go"},
		{Path: "/p/b.go", RelPath: "b.go", Name: "b.go", Ext: ".go"},
	}
	idx := buildTestIndex(entries)

	results := idx.Search(context.Background(), SearchOptions{Query: "", Tab: model.TabAll}).Items
	if len(results) != 2 {
		t.Fatalf("expected 2 results for empty query, got %d", len(results))
	}
}

func TestIndex_NoMatch(t *testing.T) {
	entries := []FileEntry{
		{Path: "/p/main.go", RelPath: "main.go", Name: "main.go", Ext: ".go"},
	}
	idx := buildTestIndex(entries)

	results := idx.Search(context.Background(), SearchOptions{Query: "zzzzz", Tab: model.TabAll}).Items
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestIndex_MaxResults(t *testing.T) {
	var entries []FileEntry
	for i := range 200 {
		name := fmt.Sprintf("file_%d.go", i)
		entries = append(entries, FileEntry{
			Path: "/p/" + name, RelPath: name, Name: name, Ext: ".go",
		})
	}
	idx := buildTestIndex(entries)

	results := idx.Search(context.Background(), SearchOptions{Query: "file", Tab: model.TabAll, MaxResults: 10}).Items
	if len(results) != 10 {
		t.Fatalf("expected 10 results, got %d", len(results))
	}
}

func TestIndex_IconsAssigned(t *testing.T) {
	entries := []FileEntry{
		{Path: "/p/main.go", RelPath: "main.go", Name: "main.go", Ext: ".go"},
		{Path: "/p/style.css", RelPath: "style.css", Name: "style.css", Ext: ".css"},
		{Path: "/p/unknown.xyz", RelPath: "unknown.xyz", Name: "unknown.xyz", Ext: ".xyz"},
	}
	idx := buildTestIndex(entries)

	results := idx.Search(context.Background(), SearchOptions{Query: "", Tab: model.TabAll}).Items
	for _, r := range results {
		if r.Icon == "" {
			t.Errorf("expected icon for %s", r.Name)
		}
		if r.IconColor == "" {
			t.Errorf("expected icon color for %s", r.Name)
		}
	}
}

func TestIndex_ConcurrentReads(t *testing.T) {
	var entries []FileEntry
	for i := range 100 {
		name := fmt.Sprintf("f%d.go", i)
		entries = append(entries, FileEntry{
			Path: "/p/" + name, RelPath: name, Name: name, Ext: ".go",
		})
	}
	idx := buildTestIndex(entries)

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				results := idx.Search(context.Background(), SearchOptions{Query: "f", Tab: model.TabAll}).Items
				if len(results) == 0 {
					t.Error("expected results from concurrent read")
				}
			}
		}()
	}
	wg.Wait()
}

func TestIndex_RebuildFrom(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a"), 0o644)

	idx := NewIndex()
	rules := &IgnoreRules{}
	idx.RebuildFrom(context.Background(), dir, rules, WalkOptions{}, nil)

	if idx.Len() != 1 {
		t.Fatalf("expected 1 file after first build, got %d", idx.Len())
	}

	// Add another file and rebuild.
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("package b"), 0o644)
	idx.RebuildFrom(context.Background(), dir, rules, WalkOptions{}, nil)

	if idx.Len() != 2 {
		t.Fatalf("expected 2 files after rebuild, got %d", idx.Len())
	}
}


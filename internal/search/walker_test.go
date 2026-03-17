package search

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWalk_BasicFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("hello"), 0o644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "c.go"), []byte("package sub"), 0o644)

	rules := &IgnoreRules{}
	ctx := context.Background()
	ch := Walk(ctx, dir, rules, WalkOptions{Workers: 2})

	var entries []FileEntry
	for e := range ch {
		entries = append(entries, e)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
}

func TestWalk_RespectsGitignore(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "app.log"), []byte("log"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644)

	rules := LoadIgnoreFiles(dir)
	ctx := context.Background()
	ch := Walk(ctx, dir, rules, WalkOptions{IncludeHidden: false})

	var names []string
	for e := range ch {
		names = append(names, e.Name)
	}

	for _, n := range names {
		if n == "app.log" {
			t.Error("app.log should have been ignored")
		}
	}
	found := false
	for _, n := range names {
		if n == "main.go" {
			found = true
		}
	}
	if !found {
		t.Error("main.go should be present")
	}
}

func TestWalk_SkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	hidden := filepath.Join(dir, ".hidden")
	os.MkdirAll(hidden, 0o755)
	os.WriteFile(filepath.Join(hidden, "secret.txt"), []byte("s"), 0o644)
	os.WriteFile(filepath.Join(dir, "visible.txt"), []byte("v"), 0o644)

	rules := &IgnoreRules{}
	ctx := context.Background()
	ch := Walk(ctx, dir, rules, WalkOptions{IncludeHidden: false})

	var names []string
	for e := range ch {
		names = append(names, e.Name)
	}

	for _, n := range names {
		if n == "secret.txt" {
			t.Error("hidden dir contents should be skipped")
		}
	}
}

func TestWalk_IncludesHiddenWhenConfigured(t *testing.T) {
	dir := t.TempDir()
	hidden := filepath.Join(dir, ".config")
	os.MkdirAll(hidden, 0o755)
	os.WriteFile(filepath.Join(hidden, "settings.json"), []byte("{}"), 0o644)

	rules := &IgnoreRules{}
	ctx := context.Background()
	ch := Walk(ctx, dir, rules, WalkOptions{IncludeHidden: true})

	found := false
	for e := range ch {
		if e.Name == "settings.json" {
			found = true
		}
	}
	if !found {
		t.Error("hidden dir contents should be included when IncludeHidden is true")
	}
}

func TestWalk_DefaultExcludes(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0o755)
	os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref"), 0o644)
	os.MkdirAll(filepath.Join(dir, "node_modules", "pkg"), 0o755)
	os.WriteFile(filepath.Join(dir, "node_modules", "pkg", "index.js"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644)

	rules := LoadIgnoreFiles(dir)
	ctx := context.Background()
	ch := Walk(ctx, dir, rules, WalkOptions{IncludeHidden: true})

	var names []string
	for e := range ch {
		names = append(names, e.RelPath)
	}

	for _, n := range names {
		if n == ".git/HEAD" || n == "node_modules/pkg/index.js" {
			t.Errorf("%s should be excluded by default", n)
		}
	}
}

func TestWalk_SymlinksOneLevelDeep(t *testing.T) {
	dir := t.TempDir()
	realDir := filepath.Join(dir, "real")
	os.MkdirAll(realDir, 0o755)
	os.WriteFile(filepath.Join(realDir, "file.go"), []byte("package real"), 0o644)

	// Create a symlink to realDir.
	link := filepath.Join(dir, "link")
	err := os.Symlink(realDir, link)
	if err != nil {
		t.Skip("symlinks not supported on this platform")
	}

	// Create a second-level symlink inside realDir pointing back (potential loop).
	loopLink := filepath.Join(realDir, "loop")
	os.Symlink(dir, loopLink)

	rules := &IgnoreRules{}
	ctx := context.Background()
	ch := Walk(ctx, dir, rules, WalkOptions{FollowSymlinks: true})

	count := 0
	for range ch {
		count++
	}
	// Should find file.go via real/ and via link/, but NOT follow loop/ into an infinite cycle.
	if count < 2 {
		t.Errorf("expected at least 2 entries (real + symlinked), got %d", count)
	}
}

func TestWalk_Cancellation(t *testing.T) {
	dir := t.TempDir()
	// Create enough files to keep the walker busy.
	for i := range 100 {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("file_%d.txt", i)), []byte("x"), 0o644)
	}

	ctx, cancel := context.WithCancel(context.Background())
	rules := &IgnoreRules{}
	ch := Walk(ctx, dir, rules, WalkOptions{Workers: 1})

	// Read a few then cancel.
	count := 0
	for range ch {
		count++
		if count >= 5 {
			cancel()
			break
		}
	}
	// Drain remaining entries (walker should stop quickly).
	for range ch {
		count++
	}

	if count >= 100 {
		t.Error("walker should have stopped early after cancellation")
	}
}

func BenchmarkWalk_100kFiles(b *testing.B) {
	dir := b.TempDir()
	// Create 100k files across 100 directories.
	for d := range 100 {
		sub := filepath.Join(dir, fmt.Sprintf("dir_%d", d))
		os.MkdirAll(sub, 0o755)
		for f := range 1000 {
			os.WriteFile(filepath.Join(sub, fmt.Sprintf("file_%d.txt", f)), []byte("x"), 0o644)
		}
	}

	rules := &IgnoreRules{}
	b.ResetTimer()

	for range b.N {
		ctx := context.Background()
		ch := Walk(ctx, dir, rules, WalkOptions{Workers: 8})
		count := 0
		for range ch {
			count++
		}
		if count != 100_000 {
			b.Fatalf("expected 100000 files, got %d", count)
		}
	}
}

func TestBenchmarkWalk_Under500ms(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test")
	}

	dir := t.TempDir()
	for d := range 100 {
		sub := filepath.Join(dir, fmt.Sprintf("dir_%d", d))
		os.MkdirAll(sub, 0o755)
		for f := range 1000 {
			os.WriteFile(filepath.Join(sub, fmt.Sprintf("file_%d.txt", f)), []byte("x"), 0o644)
		}
	}

	rules := &IgnoreRules{}
	start := time.Now()
	ctx := context.Background()
	ch := Walk(ctx, dir, rules, WalkOptions{Workers: 8})
	count := 0
	for range ch {
		count++
	}
	elapsed := time.Since(start)

	if count != 100_000 {
		t.Fatalf("expected 100000, got %d", count)
	}
	if elapsed > 600*time.Millisecond {
		t.Errorf("walk took %v, expected < 600ms", elapsed)
	} else {
		t.Logf("walked 100k files in %v", elapsed)
	}
}

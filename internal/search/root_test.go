package search

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectRoot_GitMarker(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "a", "b", "c")
	os.MkdirAll(subdir, 0o755)
	os.Mkdir(filepath.Join(dir, ".git"), 0o755)

	got := DetectRoot(subdir)
	if got != dir {
		t.Errorf("expected %s, got %s", dir, got)
	}
}

func TestDetectRoot_GoMod(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "pkg")
	os.MkdirAll(subdir, 0o755)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0o644)

	got := DetectRoot(subdir)
	if got != dir {
		t.Errorf("expected %s, got %s", dir, got)
	}
}

func TestDetectRoot_PackageJson(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "src")
	os.MkdirAll(subdir, 0o755)
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644)

	got := DetectRoot(subdir)
	if got != dir {
		t.Errorf("expected %s, got %s", dir, got)
	}
}

func TestDetectRoot_CargoToml(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "src")
	os.MkdirAll(subdir, 0o755)
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(""), 0o644)

	got := DetectRoot(subdir)
	if got != dir {
		t.Errorf("expected %s, got %s", dir, got)
	}
}

func TestDetectRoot_PomXml(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "src")
	os.MkdirAll(subdir, 0o755)
	os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(""), 0o644)

	got := DetectRoot(subdir)
	if got != dir {
		t.Errorf("expected %s, got %s", dir, got)
	}
}

func TestDetectRoot_ProjectRootMarker(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "deep", "nested")
	os.MkdirAll(subdir, 0o755)
	os.WriteFile(filepath.Join(dir, ".project-root"), []byte(""), 0o644)

	got := DetectRoot(subdir)
	if got != dir {
		t.Errorf("expected %s, got %s", dir, got)
	}
}

func TestDetectRoot_Fallback(t *testing.T) {
	dir := t.TempDir()
	// No markers at all — should return the start dir itself.
	got := DetectRoot(dir)
	// The fallback walks to filesystem root, then returns abs(startDir).
	abs, _ := filepath.Abs(dir)
	if got != abs {
		t.Errorf("expected fallback to %s, got %s", abs, got)
	}
}

func TestIgnoreRules_DefaultExcludes(t *testing.T) {
	ir := &IgnoreRules{}
	if !ir.IsIgnored(".git", true) {
		t.Error(".git should be excluded")
	}
	if !ir.IsIgnored("node_modules", true) {
		t.Error("node_modules should be excluded")
	}
	if !ir.IsIgnored("vendor", true) {
		t.Error("vendor should be excluded")
	}
	// Files named "vendor" should NOT be excluded (only dirs).
	if ir.IsIgnored("vendor", false) {
		t.Error("vendor file should not be excluded")
	}
}

func TestIgnoreRules_GitignorePatterns(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\nbuild/\n"), 0o644)

	ir := LoadIgnoreFiles(dir)

	if !ir.IsIgnored("app.log", false) {
		t.Error("*.log should match app.log")
	}
	if !ir.IsIgnored("build", true) {
		t.Error("build/ should match build dir")
	}
	if ir.IsIgnored("build", false) {
		t.Error("build/ should not match build file")
	}
	if ir.IsIgnored("main.go", false) {
		t.Error("main.go should not be ignored")
	}
}

func TestIgnoreRules_BmignorePatterns(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".bmignore"), []byte("tmp/\n*.bak\n"), 0o644)

	ir := LoadIgnoreFiles(dir)

	if !ir.IsIgnored("tmp", true) {
		t.Error("tmp/ should match")
	}
	if !ir.IsIgnored("file.bak", false) {
		t.Error("*.bak should match")
	}
}

func TestIgnoreRules_Negation(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\n!important.log\n"), 0o644)

	ir := LoadIgnoreFiles(dir)

	if !ir.IsIgnored("debug.log", false) {
		t.Error("debug.log should be ignored")
	}
	if ir.IsIgnored("important.log", false) {
		t.Error("important.log should NOT be ignored (negated)")
	}
}

func TestIgnoreRules_NestedGitignore(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0o755)
	os.WriteFile(filepath.Join(sub, ".gitignore"), []byte("*.tmp\n"), 0o644)

	ir := LoadIgnoreFiles(dir)
	ir.LoadNested(dir, filepath.Join(sub, ".gitignore"))

	if !ir.IsIgnored("sub/foo.tmp", false) {
		t.Error("sub/foo.tmp should be ignored by nested gitignore")
	}
}

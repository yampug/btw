package search

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Markers that indicate a project root directory.
var rootMarkers = []string{
	".git",
	"go.mod",
	"package.json",
	"Cargo.toml",
	"pom.xml",
	".project-root",
}

// DefaultExcludes are directory names always excluded from walking.
var DefaultExcludes = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
}

// DetectRoot walks up from startDir looking for a project root marker.
// Returns the first directory containing a marker, or startDir if none found.
func DetectRoot(startDir string) string {
	home, _ := os.UserHomeDir()
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return startDir
	}

	for {
		for _, marker := range rootMarkers {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				// Special case: if we found a marker in the home directory,
				// be extra careful. Only .git is usually a good indicator.
				// package.json/go.mod in home are often noise.
				if dir == home && marker != ".git" {
					continue
				}
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	abs, err := filepath.Abs(startDir)
	if err != nil {
		return startDir
	}
	return abs
}

// IgnoreRules holds the set of patterns parsed from .gitignore / .bmignore files.
type IgnoreRules struct {
	patterns []ignorePattern
}

// Clone returns a deep copy of the ignore rules.
func (ir *IgnoreRules) Clone() *IgnoreRules {
	if ir == nil {
		return &IgnoreRules{}
	}
	newPatterns := make([]ignorePattern, len(ir.patterns))
	copy(newPatterns, ir.patterns)
	return &IgnoreRules{patterns: newPatterns}
}

type ignorePattern struct {
	pattern  string
	negated  bool
	dirOnly  bool
	anchored bool // contains a slash → only matches relative to the ignore file's dir
}

// LoadIgnoreFiles parses .gitignore and .bmignore at the given root.
// Additional nested .gitignore files can be loaded with LoadNested.
func LoadIgnoreFiles(root string) *IgnoreRules {
	ir := &IgnoreRules{}
	ir.loadFile(filepath.Join(root, ".gitignore"))
	ir.loadFile(filepath.Join(root, ".bmignore"))
	return ir
}

// LoadNested parses a .gitignore found in a subdirectory.
// Patterns are prefixed with the relative directory so they match correctly.
func (ir *IgnoreRules) LoadNested(root, path string) {
	rel, err := filepath.Rel(root, filepath.Dir(path))
	if err != nil {
		return
	}

	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p := parseLine(line)
		// Prefix with relative dir so pattern is anchored correctly.
		if rel != "." && rel != "" {
			p.pattern = rel + "/" + p.pattern
			p.anchored = true
		}
		ir.patterns = append(ir.patterns, p)
	}
}

func (ir *IgnoreRules) loadFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ir.patterns = append(ir.patterns, parseLine(line))
	}
}

func parseLine(line string) ignorePattern {
	p := ignorePattern{}

	if strings.HasPrefix(line, "!") {
		p.negated = true
		line = line[1:]
	}

	if strings.HasSuffix(line, "/") {
		p.dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}

	// A pattern containing a slash (other than trailing) is anchored.
	if strings.Contains(line, "/") {
		p.anchored = true
		line = strings.TrimPrefix(line, "/")
	}

	p.pattern = line
	return p
}

// IsIgnored reports whether a path (relative to root) should be excluded.
// isDir indicates whether the path is a directory.
func (ir *IgnoreRules) IsIgnored(relPath string, isDir bool) bool {
	// Always-excluded directories.
	base := filepath.Base(relPath)
	if isDir && DefaultExcludes[base] {
		return true
	}

	ignored := false
	for _, p := range ir.patterns {
		if p.dirOnly && !isDir {
			continue
		}
		if matchPattern(p, relPath, base) {
			ignored = !p.negated
		}
	}
	return ignored
}

// matchPattern checks whether a single pattern matches the given path.
func matchPattern(p ignorePattern, relPath, base string) bool {
	pattern := p.pattern

	if p.anchored {
		// Match against full relative path.
		matched, _ := filepath.Match(pattern, relPath)
		if matched {
			return true
		}
		// Try with ** prefix behavior: pattern matches a suffix of path segments.
		if strings.HasPrefix(pattern, "**/") {
			inner := pattern[3:]
			matched, _ = filepath.Match(inner, relPath)
			if matched {
				return true
			}
			// Also try against each suffix.
			for i := 0; i < len(relPath); i++ {
				if relPath[i] == '/' {
					matched, _ = filepath.Match(inner, relPath[i+1:])
					if matched {
						return true
					}
				}
			}
		}
		return false
	}

	// Unanchored: match against basename, or full path if pattern has a wildcard that could span.
	matched, _ := filepath.Match(pattern, base)
	if matched {
		return true
	}
	// Also try matching the full relative path.
	matched, _ = filepath.Match(pattern, relPath)
	return matched
}

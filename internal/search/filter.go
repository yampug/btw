
package search

import (
	"regexp"
	"sort"
	"strings"
)

var (
	extPrefixRe  = regexp.MustCompile(`(?i)\b(?:ext|type):([a-zA-Z0-9,]+)`)
	dotSuffixRe  = regexp.MustCompile(`\s+(\.[a-zA-Z0-9]+)$`)
)

// FilterResult holds the parsed query and any extracted extension filters.
type FilterResult struct {
	Query      string   // query text with filter directives removed
	Extensions []string // e.g. [".go", ".rs"] — normalized with leading dot
}

// ParseFilters extracts extension filters from the query string.
// Supports:
//   - `.go` at end of query (after whitespace)
//   - `ext:go` or `type:go`
//   - `ext:go,rs,py` (comma-separated)
//
// Returns the cleaned query and the list of extensions.
func ParseFilters(query string) FilterResult {
	var exts []string
	cleaned := query

	// Match ext:X or type:X directives.
	if m := extPrefixRe.FindStringSubmatchIndex(cleaned); m != nil {
		raw := cleaned[m[2]:m[3]] // capture group 1
		for _, e := range strings.Split(raw, ",") {
			e = strings.TrimSpace(e)
			if e != "" {
				exts = append(exts, normalizeExt(e))
			}
		}
		cleaned = cleaned[:m[0]] + cleaned[m[1]:]
	}

	// Match trailing .ext suffix (after whitespace).
	if len(exts) == 0 {
		if m := dotSuffixRe.FindStringSubmatchIndex(cleaned); m != nil {
			ext := cleaned[m[2]:m[3]]
			exts = append(exts, strings.ToLower(ext))
			cleaned = cleaned[:m[0]]
		}
	}

	cleaned = strings.TrimSpace(cleaned)

	return FilterResult{Query: cleaned, Extensions: exts}
}

// normalizeExt ensures the extension has a leading dot and is lowercase.
func normalizeExt(ext string) string {
	ext = strings.ToLower(strings.TrimSpace(ext))
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return ext
}

// ExtensionStats holds an extension and its file count.
type ExtensionStats struct {
	Ext   string
	Count int
}

// Extensions returns all file extensions in the index sorted by frequency (most common first).
func (idx *Index) Extensions() []ExtensionStats {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var stats []ExtensionStats
	for ext, indices := range idx.extIndex {
		stats = append(stats, ExtensionStats{Ext: ext, Count: len(indices)})
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Count != stats[j].Count {
			return stats[i].Count > stats[j].Count
		}
		return stats[i].Ext < stats[j].Ext
	})
	return stats
}

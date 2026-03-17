package search

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bob/boomerang/internal/model"
)

// fileIconMap maps common extensions to an icon and color.
var fileIconMap = map[string]struct{ icon, color string }{
	".go":   {"\U000f07d3", "#00ADD8"},
	".js":   {"\U000f0031", "#F7DF1E"},
	".ts":   {"\U000f06e6", "#3178C6"},
	".py":   {"\U000f0320", "#3776AB"},
	".rb":   {"\U000f0d2d", "#CC342D"},
	".rs":   {"\U000f0617", "#DEA584"},
	".java": {"\U000f0176", "#ED8B00"},
	".c":    {"\U000f0671", "#A8B9CC"},
	".cpp":  {"\U000f0672", "#00599C"},
	".h":    {"\U000f0673", "#A8B9CC"},
	".css":  {"\U000f031c", "#1572B6"},
	".html": {"\U000f031b", "#E34F26"},
	".json": {"\U000f0626", "#CBCB41"},
	".yaml": {"\U000f0626", "#CB171E"},
	".yml":  {"\U000f0626", "#CB171E"},
	".md":   {"\U000f0354", "#519ABA"},
	".sh":   {"\U000f0239", "#4EAA25"},
	".sql":  {"\U000f0c49", "#E38C00"},
	".toml": {"\U000f0626", "#9C4221"},
	".xml":  {"\U000f05c0", "#E37933"},
}

const defaultIcon = "\U000f0214"
const defaultIconColor = "#888888"

// Index is an in-memory file index built from walker output.
// It is safe for concurrent reads once built. Use RebuildFrom to replace
// the index contents.
type Index struct {
	mu        sync.RWMutex
	files     []FileEntry
	nameIndex map[string][]int // lowercase filename → indices
	extIndex  map[string][]int // extension (e.g. ".go") → indices
}

// NewIndex returns an empty Index.
func NewIndex() *Index {
	return &Index{
		nameIndex: make(map[string][]int),
		extIndex:  make(map[string][]int),
	}
}

// BuildFrom consumes all entries from the channel in a single pass and builds
// the index. This blocks until the channel is closed.
func (idx *Index) BuildFrom(ch <-chan FileEntry) {
	var files []FileEntry
	nameIdx := make(map[string][]int)
	extIdx := make(map[string][]int)

	for entry := range ch {
		i := len(files)
		files = append(files, entry)

		lowerName := strings.ToLower(entry.Name)
		nameIdx[lowerName] = append(nameIdx[lowerName], i)

		if entry.Ext != "" {
			ext := strings.ToLower(entry.Ext)
			extIdx[ext] = append(extIdx[ext], i)
		}
	}

	idx.mu.Lock()
	idx.files = files
	idx.nameIndex = nameIdx
	idx.extIndex = extIdx
	idx.mu.Unlock()
}

// RebuildFrom rebuilds the index from a new walk, replacing existing data.
// Can be called while readers are using the old index (swap is atomic under lock).
func (idx *Index) RebuildFrom(ctx context.Context, root string, rules *IgnoreRules, opts WalkOptions) {
	ch := Walk(ctx, root, rules, opts)
	idx.BuildFrom(ch)
}

// Len returns the number of indexed files.
func (idx *Index) Len() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.files)
}

// SearchOptions controls how Search filters and ranks results.
type SearchOptions struct {
	Query       string
	Tab         model.Tab
	ExtFilter   string // e.g. ".go" — empty means no filter
	MaxResults  int    // 0 means no limit
}

// Search returns ranked SearchResult entries matching the given options.
// Uses FuzzyMatch for matching and Score for contextual ranking.
// Queries containing `/` are handled as path-aware matches.
// A `:N` suffix is stripped and stored as LineNum on results.
func (idx *Index) Search(opts SearchOptions) []model.SearchResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if opts.MaxResults <= 0 {
		opts.MaxResults = 100
	}

	pq := ParseQuery(opts.Query)

	// Determine candidate set.
	candidates := idx.candidates(opts)

	var results []model.SearchResult
	for _, i := range candidates {
		entry := idx.files[i]

		var mr MatchResult
		if pq.IsPath {
			mr = PathMatch(pq.Query, entry.RelPath)
		} else {
			mr = FuzzyMatch(pq.Query, entry.Name)
			if !mr.Matched {
				mr = FuzzyMatch(pq.Query, entry.RelPath)
			}
		}
		if !mr.Matched {
			continue
		}

		params := ScoreParams{
			RelPath: entry.RelPath,
			Name:    entry.Name,
		}
		finalScore := Score(mr, params)
		r := idx.toResult(entry, finalScore, mr.Ranges)
		r.Line = pq.LineNum
		results = append(results, r)
	}

	RankResults(results)

	if len(results) > opts.MaxResults {
		results = results[:opts.MaxResults]
	}
	return results
}

// candidates returns the set of file indices to search over.
func (idx *Index) candidates(opts SearchOptions) []int {
	// Extension filter: O(1) lookup.
	if opts.ExtFilter != "" {
		ext := strings.ToLower(opts.ExtFilter)
		return idx.extIndex[ext]
	}

	// Tab-based filtering.
	switch opts.Tab {
	case model.TabFiles, model.TabAll:
		// All files.
		all := make([]int, len(idx.files))
		for i := range all {
			all[i] = i
		}
		return all
	default:
		// Other tabs (Classes, Symbols, etc.) will be handled in later epics.
		// For now, return all files.
		all := make([]int, len(idx.files))
		for i := range all {
			all[i] = i
		}
		return all
	}
}


func (idx *Index) toResult(entry FileEntry, score int, ranges []model.MatchRange) model.SearchResult {
	icon := defaultIcon
	iconColor := defaultIconColor
	if m, ok := fileIconMap[strings.ToLower(entry.Ext)]; ok {
		icon = m.icon
		iconColor = m.color
	}

	detail := filepath.Dir(entry.RelPath)
	if detail == "." {
		detail = ""
	}

	return model.SearchResult{
		Name:        entry.Name,
		Detail:      detail,
		ResultType:  model.ResultFile,
		FilePath:    entry.Path,
		Score:       score,
		MatchRanges: ranges,
		Icon:        icon,
		IconColor:   iconColor,
	}
}

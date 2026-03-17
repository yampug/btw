package search

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/bob/boomerang/internal/config"
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
	symbols   []Symbol         // populated by ExtractSymbols
	root      string
}

// Root returns the project root path.
func (idx *Index) Root() string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.root
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
	idx.mu.Lock()
	idx.root = root
	idx.mu.Unlock()
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
	Query         string
	Tab           model.Tab
	ExtFilter     string   // e.g. ".go" — single ext (legacy, used by TUI badge)
	ExtFilters    []string // multiple extensions (e.g. [".go", ".rs"])
	MaxResults    int      // 0 means no limit
	IncludeHidden bool     // include hidden files in results
	ProjectOnly   bool     // exclude vendor, node_modules, etc.
	History       *config.History // for score boosting
}

// SearchResultSet holds results plus metadata for the UI.
type SearchResultSet struct {
	Items        []model.SearchResult
	TotalMatched int // count before MaxResults truncation
}

// Search returns ranked SearchResult entries matching the given options.
// Uses FuzzyMatch for matching and Score for contextual ranking.
// Queries containing `/` are handled as path-aware matches.
// A `:N` suffix is stripped and stored as LineNum on results.
func (idx *Index) Search(opts SearchOptions) SearchResultSet {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if opts.MaxResults <= 0 {
		opts.MaxResults = 100
	}

	// Parse inline extension filters from query.
	fr := ParseFilters(opts.Query)
	queryForMatch := fr.Query

	// Merge inline filters with explicit filters.
	allExts := append(opts.ExtFilters, fr.Extensions...)
	if opts.ExtFilter != "" {
		allExts = append(allExts, strings.ToLower(opts.ExtFilter))
	}

	pq := ParseQuery(queryForMatch)

	// Determine candidate set.
	candidates := idx.candidatesFiltered(opts, allExts)

	var results []model.SearchResult
	for _, ci := range candidates {
		entry := idx.files[ci]

		// Skip hidden files unless requested.
		if !opts.IncludeHidden && strings.HasPrefix(entry.Name, ".") {
			continue
		}

		// Project Only: exclude vendor, node_modules, etc.
		if opts.ProjectOnly && isVendored(entry.RelPath) {
			continue
		}

		// Empty query: return all candidates (sorted by mod time later).
		if strings.TrimSpace(pq.Query) == "" {
			score := 0
			if opts.History != nil {
				score = opts.History.GetBoost(entry.Path)
			}
			r := idx.toResult(entry, score, nil)
			r.Line = pq.LineNum
			results = append(results, r)
			continue
		}

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
		if opts.History != nil {
			finalScore += opts.History.GetBoost(entry.Path)
		}
		r := idx.toResult(entry, finalScore, mr.Ranges)
		r.Line = pq.LineNum
		results = append(results, r)
	}

	// For empty queries, sort by score (history boost) then modification time.
	if strings.TrimSpace(pq.Query) == "" {
		idx.sortByScoreAndModTime(results)
	} else {
		RankResults(results)
	}

	totalMatched := len(results)
	if len(results) > opts.MaxResults {
		results = results[:opts.MaxResults]
	}
	return SearchResultSet{Items: results, TotalMatched: totalMatched}
}

// isVendored returns true if any segment of the relative path is in DefaultExcludes.
func isVendored(relPath string) bool {
	segments := strings.Split(relPath, string(filepath.Separator))
	for _, s := range segments {
		if DefaultExcludes[s] {
			return true
		}
	}
	return false
}

// SearchSymbols returns ranked symbols matching the query.
func (idx *Index) SearchSymbols(query string, maxResults int, includeHidden bool, projectOnly bool, history *config.History) SearchResultSet {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var results []model.SearchResult
	for _, s := range idx.symbols {
		if !includeHidden && strings.HasPrefix(filepath.Base(s.FilePath), ".") {
			continue
		}
		if projectOnly {
			// Find the relative path to check for vendor/etc.
			// This is a bit expensive without relPath stored in Symbol.
			// For now, let's assume Symbol has enough info or we can get it.
			// Actually Symbols are extracted from files in the index, so we can check
			// their path against the root.
			rel, err := filepath.Rel(idx.root, s.FilePath)
			if err == nil && isVendored(rel) {
				continue
			}
		}

		if query == "" {
			score := 0
			if history != nil {
				score = history.GetBoost(s.FilePath)
			}
			results = append(results, SymbolToResult(s, score, nil))
			continue
		}

		mr := FuzzyMatch(query, s.Name)
		if !mr.Matched {
			continue
		}

		score := mr.Score
		if history != nil {
			score += history.GetBoost(s.FilePath)
		}
		results = append(results, SymbolToResult(s, score, mr.Ranges))
	}

	RankResults(results)
	totalMatched := len(results)
	if maxResults > 0 && len(results) > maxResults {
		results = results[:maxResults]
	}
	return SearchResultSet{Items: results, TotalMatched: totalMatched}
}

// SearchClasses returns only type-level symbols matching the query.
func (idx *Index) SearchClasses(query string, maxResults int, includeHidden bool, projectOnly bool, history *config.History) SearchResultSet {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var results []model.SearchResult
	for _, s := range idx.symbols {
		if !isClassLike(s.Kind) {
			continue
		}
		if !includeHidden && strings.HasPrefix(filepath.Base(s.FilePath), ".") {
			continue
		}
		if projectOnly {
			rel, err := filepath.Rel(idx.root, s.FilePath)
			if err == nil && isVendored(rel) {
				continue
			}
		}

		if query == "" {
			score := 0
			if history != nil {
				score = history.GetBoost(s.FilePath)
			}
			results = append(results, SymbolToResult(s, score, nil))
			continue
		}

		mr := FuzzyMatch(query, s.Name)
		if !mr.Matched {
			continue
		}

		score := mr.Score
		if history != nil {
			score += history.GetBoost(s.FilePath)
		}
		results = append(results, SymbolToResult(s, score, mr.Ranges))
	}

	RankResults(results)
	totalMatched := len(results)
	if maxResults > 0 && len(results) > maxResults {
		results = results[:maxResults]
	}
	return SearchResultSet{Items: results, TotalMatched: totalMatched}
}

// sortByScoreAndModTime sorts results by score (desc) then modification time (newest first).
func (idx *Index) sortByScoreAndModTime(results []model.SearchResult) {
	// Build a map from FilePath → ModTime for the sort.
	modTimes := make(map[string]int64, len(idx.files))
	for _, f := range idx.files {
		modTimes[f.Path] = f.ModTime.UnixNano()
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return modTimes[results[i].FilePath] > modTimes[results[j].FilePath]
	})
}

// candidatesFiltered returns the set of file indices to search over,
// applying extension filters if provided.
func (idx *Index) candidatesFiltered(opts SearchOptions, exts []string) []int {
	if len(exts) > 0 {
		// Collect unique indices from all matching extensions.
		seen := make(map[int]bool)
		var result []int
		for _, ext := range exts {
			for _, i := range idx.extIndex[strings.ToLower(ext)] {
				if !seen[i] {
					seen[i] = true
					result = append(result, i)
				}
			}
		}
		return result
	}

	// No extension filter — return all files.
	all := make([]int, len(idx.files))
	for i := range all {
		all[i] = i
	}
	return all
}


// Files returns a snapshot of the indexed file entries.
func (idx *Index) Files() []FileEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	result := make([]FileEntry, len(idx.files))
	copy(result, idx.files)
	return result
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

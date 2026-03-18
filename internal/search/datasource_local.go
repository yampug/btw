package search

import "context"

// LocalDataSource implements DataSource by wrapping the existing os.* based functions.
type LocalDataSource struct{}

// NewLocalDataSource creates a new LocalDataSource.
func NewLocalDataSource() *LocalDataSource {
	return &LocalDataSource{}
}

func (l *LocalDataSource) Walk(ctx context.Context, root string, opts WalkOptions) <-chan FileEntry {
	rules := LoadIgnoreFiles(root)
	if len(opts.ExtraIgnorePatterns) > 0 {
		rules.LoadPatterns(opts.ExtraIgnorePatterns)
	}
	return Walk(ctx, root, rules, opts)
}

func (l *LocalDataSource) Grep(ctx context.Context, root string, query string, opts GrepOptions) <-chan GrepMatch {
	// First gather files via walking.
	rules := LoadIgnoreFiles(root)
	walkOpts := WalkOptions{
		IncludeHidden: true, // we get all files, and let Grep do the filtering
		MaxDepth:      0,
	}
	ch := Walk(ctx, root, rules, walkOpts)
	
	var files []FileEntry
	for entry := range ch {
		files = append(files, entry)
	}

	// Now run the standard grep logic
	idx := NewIndex()
	idx.root = root
	idx.files = files
	
	return Grep(ctx, idx, query, opts)
}

func (l *LocalDataSource) ExtractSymbols(ctx context.Context, files []FileEntry) []Symbol {
	idx := NewIndex()
	idx.files = files
	idx.ExtractSymbols()
	return idx.Symbols()
}

func (l *LocalDataSource) DetectRoot(startDir string) (string, error) {
	return DetectRoot(startDir), nil
}

func (l *LocalDataSource) LoadIgnoreFiles(root string) (*IgnoreRules, error) {
	return LoadIgnoreFiles(root), nil
}

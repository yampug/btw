package search

import "context"

// DataSource abstracts file system operations so the application can work
// seamlessly against both local and remote (via SSH agent) filesystems.
type DataSource interface {
	Walk(ctx context.Context, root string, opts WalkOptions) <-chan FileEntry
	Grep(ctx context.Context, root string, query string, opts GrepOptions) <-chan GrepMatch
	ExtractSymbols(ctx context.Context, files []FileEntry) []Symbol
	DetectRoot(startDir string) (string, error)
	LoadIgnoreFiles(root string) (*IgnoreRules, error)
	// Done returns a channel that closes when the underlying source is permanently disconnected.
	Done() <-chan struct{}
}

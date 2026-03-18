package remote

import (
	"context"

	"github.com/yampug/btw/internal/search"
)

// RemoteDataSource implements search.DataSource by streaming requests over an SSH Session.
type RemoteDataSource struct {
	sess *Session
}

// NewRemoteDataSource creates a new RemoteDataSource using the provided session.
func NewRemoteDataSource(sess *Session) *RemoteDataSource {
	return &RemoteDataSource{sess: sess}
}

func (r *RemoteDataSource) Walk(ctx context.Context, root string, opts search.WalkOptions) <-chan search.FileEntry {
	// Stub to panic initially per acceptance criteria
	panic("RemoteDataSource.Walk not implemented")
}

func (r *RemoteDataSource) Grep(ctx context.Context, root string, query string, opts search.GrepOptions) <-chan search.GrepMatch {
	// Stub to panic initially per acceptance criteria
	panic("RemoteDataSource.Grep not implemented")
}

func (r *RemoteDataSource) ExtractSymbols(ctx context.Context, files []search.FileEntry) []search.Symbol {
	// Stub to panic initially per acceptance criteria
	panic("RemoteDataSource.ExtractSymbols not implemented")
}

func (r *RemoteDataSource) DetectRoot(startDir string) (string, error) {
	// Stub to panic initially per acceptance criteria
	panic("RemoteDataSource.DetectRoot not implemented")
}

func (r *RemoteDataSource) LoadIgnoreFiles(root string) (*search.IgnoreRules, error) {
	// Stub to panic initially per acceptance criteria
	panic("RemoteDataSource.LoadIgnoreFiles not implemented")
}

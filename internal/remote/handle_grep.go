package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/yampug/btw/internal/search"
)

// HandleGrep is the agent-side handler for "grep" requests.
// It builds a file index for the given root (if not already cached),
// runs a content search, and streams GrepMatchResult messages back.
func HandleGrep(ctx context.Context, id int, params json.RawMessage, enc *Encoder) error {
	var p GrepParams
	if err := json.Unmarshal(params, &p); err != nil {
		return enc.Send(NewErrorEnvelope(id, ErrCodeInternal, fmt.Sprintf("bad grep params: %v", err)))
	}

	if p.Root == "" {
		return enc.Send(NewErrorEnvelope(id, ErrCodeInternal, "grep: root is required"))
	}
	if p.Query == "" {
		return enc.Send(NewErrorEnvelope(id, ErrCodeInternal, "grep: query is required"))
	}

	// Verify root exists.
	info, err := os.Stat(p.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return enc.Send(NewNotFoundEnvelope(id, p.Root))
		}
		return enc.Send(NewInternalErrorEnvelope(id, err))
	}
	if !info.IsDir() {
		return enc.Send(NewErrorEnvelope(id, ErrCodeInternal, fmt.Sprintf("grep: %s is not a directory", p.Root)))
	}

	// Build a temporary index for this grep request.
	// In production the agent would cache this, but for correctness each
	// request gets a fresh walk matching the current state of the filesystem.
	rules := search.LoadIgnoreFiles(p.Root)
	idx := search.NewIndex()
	// Walk with IncludeHidden=true so all files are in the index.
	// The grep options handle hidden-file filtering at query time.
	idx.RebuildFrom(ctx, p.Root, rules, search.WalkOptions{IncludeHidden: true}, nil)

	if ctx.Err() != nil {
		return enc.Send(NewCancelledEnvelope(id))
	}

	// Run grep.
	grepOpts := search.GrepOptions{
		IncludeHidden: p.IncludeHidden,
		MaxResults:    p.MaxResults,
		ProjectOnly:   p.ProjectOnly,
	}

	ch := search.Grep(ctx, idx, p.Query, grepOpts)
	for match := range ch {
		select {
		case <-ctx.Done():
			return enc.Send(NewCancelledEnvelope(id))
		default:
		}

		var matchRanges []MatchRange
		for _, mr := range match.MatchRanges {
			matchRanges = append(matchRanges, MatchRange{
				Start: mr.Start,
				End:   mr.End,
			})
		}

		result := GrepMatchResult{
			FilePath:    match.FilePath,
			RelPath:     match.RelPath,
			FileName:    match.FileName,
			Line:        match.Line,
			Column:      match.Column,
			Content:     match.Content,
			MatchRanges: matchRanges,
		}

		if err := enc.Send(Envelope{
			Method: MethodGrepMatch,
			ID:     id,
			Result: result,
		}); err != nil {
			return err
		}
	}

	// Send done.
	return enc.Send(Envelope{
		Method: MethodGrepMatch,
		ID:     id,
		Done:   true,
	})
}

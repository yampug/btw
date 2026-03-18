package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/yampug/btw/internal/search"
)

// HandleWalk is the agent-side handler for "walk" requests.
// It walks the file tree at the given root and streams FileEntryResult
// messages back to the client, followed by a done message.
func HandleWalk(ctx context.Context, id int, params json.RawMessage, enc *Encoder) error {
	var p WalkParams
	if err := json.Unmarshal(params, &p); err != nil {
		return enc.Send(NewErrorEnvelope(id, ErrCodeInternal, fmt.Sprintf("bad walk params: %v", err)))
	}

	if p.Root == "" {
		return enc.Send(NewErrorEnvelope(id, ErrCodeInternal, "walk: root is required"))
	}

	// Verify root exists.
	info, err := os.Stat(p.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return enc.Send(NewNotFoundEnvelope(id, p.Root))
		}
		if os.IsPermission(err) {
			return enc.Send(NewPermissionDeniedEnvelope(id, p.Root))
		}
		return enc.Send(NewInternalErrorEnvelope(id, err))
	}
	if !info.IsDir() {
		return enc.Send(NewErrorEnvelope(id, ErrCodeInternal, fmt.Sprintf("walk: %s is not a directory", p.Root)))
	}

	// Build ignore rules.
	rules := search.LoadIgnoreFiles(p.Root)
	if len(p.IgnorePatterns) > 0 {
		rules.LoadPatterns(p.IgnorePatterns)
	}

	// Build walk options.
	opts := search.WalkOptions{
		IncludeHidden:  p.IncludeHidden,
		FollowSymlinks: p.FollowSymlinks,
		MaxDepth:       p.MaxDepth,
	}

	// Walk and stream entries.
	ch := search.Walk(ctx, p.Root, rules, opts)
	for entry := range ch {
		// Check for cancellation between entries.
		select {
		case <-ctx.Done():
			return enc.Send(NewCancelledEnvelope(id))
		default:
		}

		result := FileEntryResult{
			Path:    entry.Path,
			RelPath: entry.RelPath,
			Name:    entry.Name,
			Ext:     entry.Ext,
			Size:    entry.Size,
			ModTime: entry.ModTime,
			IsDir:   entry.IsDir,
		}

		if err := enc.Send(Envelope{
			Method: MethodWalkEntry,
			ID:     id,
			Result: result,
		}); err != nil {
			return err
		}
	}

	// Send done.
	return enc.Send(Envelope{
		Method: MethodWalkEntry,
		ID:     id,
		Done:   true,
	})
}

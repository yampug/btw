package remote

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/yampug/btw/internal/model"
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
	// Use an atomic or just lock the session for requests if they were concurrent,
	// but the TUI currently operates serially for remote connections.
	id := int(time.Now().UnixNano() % 100000)
	
	p := WalkParams{
		Root:           root,
		IncludeHidden:  opts.IncludeHidden,
		FollowSymlinks: opts.FollowSymlinks,
		MaxDepth:       opts.MaxDepth,
		IgnorePatterns: opts.ExtraIgnorePatterns,
	}

	out := make(chan search.FileEntry, 64)

	// We must run the receive loop in a goroutine so Walk returns immediately.
	go func() {
		defer close(out)

		r.sess.mu.Lock()
		err := r.sess.Enc.Send(Envelope{
			Method: MethodWalk,
			ID:     id,
			Params: p,
		})
		r.sess.mu.Unlock()

		if err != nil {
			return
		}

		// Also handle cancellation. If our context cancels, we need to send MethodCancel.
		// A goroutine to monitor context.
		doneRead := make(chan struct{})
		defer close(doneRead)
		go func() {
			select {
			case <-ctx.Done():
				r.sess.mu.Lock()
				_ = r.sess.Enc.Send(Envelope{
					Method: MethodCancel,
					Params: CancelParams{CancelID: id},
				})
				r.sess.mu.Unlock()
			case <-doneRead:
			}
		}()

		for {
			// We lock the decoder implicitly via the loop, assuming one active stream.
			// This works for our initial single-threaded pipeline.
			raw, err := r.sess.Dec.Receive()
			if err != nil {
				return
			}
			
			resp, err := ParseResponse(raw)
			if err != nil {
				return
			}

			if resp.Done {
				return
			}
			
			if resp.Method == MethodWalkEntry {
				var result FileEntryResult
				if err := json.Unmarshal(resp.Result, &result); err == nil {
					select {
					case out <- search.FileEntry{
						Path:     result.Path,
						RelPath:  result.RelPath,
						Name:     result.Name,
						Ext:      result.Ext,
						Size:     result.Size,
						ModTime:  result.ModTime,
						IsDir:    result.IsDir,
						IsRemote: true,
					}:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return out
}

func (r *RemoteDataSource) Grep(ctx context.Context, root string, query string, opts search.GrepOptions) <-chan search.GrepMatch {
	id := int(time.Now().UnixNano() % 100000)
	
	p := GrepParams{
		Root:          root,
		Query:         query,
		MaxResults:    opts.MaxResults,
		IncludeHidden: opts.IncludeHidden,
		ProjectOnly:   opts.ProjectOnly,
	}

	out := make(chan search.GrepMatch, 64)

	go func() {
		defer close(out)

		r.sess.mu.Lock()
		err := r.sess.Enc.Send(Envelope{
			Method: MethodGrep,
			ID:     id,
			Params: p,
		})
		r.sess.mu.Unlock()

		if err != nil {
			return
		}

		doneRead := make(chan struct{})
		defer close(doneRead)
		go func() {
			select {
			case <-ctx.Done():
				r.sess.mu.Lock()
				_ = r.sess.Enc.Send(Envelope{
					Method: MethodCancel,
					Params: CancelParams{CancelID: id},
				})
				r.sess.mu.Unlock()
			case <-doneRead:
			}
		}()

		for {
			raw, err := r.sess.Dec.Receive()
			if err != nil {
				return
			}
			
			resp, err := ParseResponse(raw)
			if err != nil {
				return
			}

			if resp.Done {
				return
			}
			
			if resp.Method == MethodGrepMatch {
				var result GrepMatchResult
				if err := json.Unmarshal(resp.Result, &result); err == nil {
					ranges := make([]model.MatchRange, len(result.MatchRanges))
					for i, mr := range result.MatchRanges {
						ranges[i] = model.MatchRange{Start: mr.Start, End: mr.End}
					}
					select {
					case out <- search.GrepMatch{
						FilePath:    result.FilePath,
						RelPath:     result.RelPath,
						FileName:    result.FileName,
						Line:        result.Line,
						Column:      result.Column,
						Content:     result.Content,
						MatchRanges: ranges,
					}:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return out
}

func (r *RemoteDataSource) ExtractSymbols(ctx context.Context, files []search.FileEntry) []search.Symbol {
	// files argument is not really used because RemoteDataSource operates on the root.
	// We'll extract root from the first file. Wait, what if files is empty?
	// This means ExtractSymbols needs `root` or should just do an empty return.
	// Actually, the app delegates to the DataSource the entire extraction for simplicity.
	// We can cheat: grab root from the first file's Path vs RelPath, but this is messy.
	// Better: RemoteDataSource SHOULD know its root if it needs to, but wait.
	// Our `tui` rebuilt the index via `Walk(ctx, root)`. We could save the root when Walk is called over `RemoteDataSource`?
	// For now, let's look at `index.files` or `index.Root()`. In `search.Index.RebuildFrom`, `ds.ExtractSymbols(ctx, idx.files)` is called.
	// However, `RemoteDataSource` doesn't strictly need `files`, it just needs the root.
	// To avoid changing `DataSource` interface again, we can infer root from `files` or we just mutate `RemoteDataSource` to temporarily hold `root` from `Walk` calls?
	// It's safe since `RemoteDataSource` is per-session/per-tab right now.
	// Wait, the story says "Index stores them as usual."
	// Let's deduce root from the first file:
	
	if len(files) == 0 {
		return nil
	}
	
	// Deduce root safely
	first := files[0]
	root := strings.TrimSuffix(first.Path, first.RelPath)
	// Example: path="/foo/bar/a.go", rel="a.go" -> "/foo/bar/"
	root = strings.TrimSuffix(root, "/")

	id := int(time.Now().UnixNano() % 100000)
	p := SymbolsParams{Root: root}

	r.sess.mu.Lock()
	err := r.sess.Enc.Send(Envelope{
		Method: MethodSymbols,
		ID:     id,
		Params: p,
	})
	r.sess.mu.Unlock()

	if err != nil {
		return nil
	}

	doneRead := make(chan struct{})
	defer close(doneRead)
	go func() {
		select {
		case <-ctx.Done():
			r.sess.mu.Lock()
			_ = r.sess.Enc.Send(Envelope{
				Method: MethodCancel,
				Params: CancelParams{CancelID: id},
			})
			r.sess.mu.Unlock()
		case <-doneRead:
		}
	}()

	var syms []search.Symbol

	for {
		raw, err := r.sess.Dec.Receive()
		if err != nil {
			return syms
		}
		
		resp, err := ParseResponse(raw)
		if err != nil {
			return syms
		}

		if resp.Done {
			return syms
		}
		
		if resp.Method == MethodSymbolEntry {
			var result SymbolEntryResult
			if err := json.Unmarshal(resp.Result, &result); err == nil {
				
				// Translate kind string back to SymbolKind
				var kind search.SymbolKind
				switch result.Kind {
				case "func": kind = search.SymbolFunc
				case "type": kind = search.SymbolType
				case "constant": kind = search.SymbolConstant
				case "variable": kind = search.SymbolVariable
				}

				syms = append(syms, search.Symbol{
					Name:      result.Name,
					Signature: result.Signature,
					Kind:      kind,
					FilePath:  result.FilePath,
					RelPath:   result.RelPath,
					FileName:  result.FileName,
					Line:      result.Line,
				})
			}
		}
	}
}

func (r *RemoteDataSource) DetectRoot(startDir string) (string, error) {
	id := int(time.Now().UnixNano() % 100000)
	
	r.sess.mu.Lock()
	err := r.sess.Enc.Send(Envelope{
		Method: MethodDetectRoot,
		ID:     id,
		Params: DetectRootParams{StartDir: startDir},
	})
	r.sess.mu.Unlock()

	if err != nil {
		return startDir, err
	}

	raw, err := r.sess.Dec.Receive()
	if err != nil {
		return startDir, err
	}

	resp, err := ParseResponse(raw)
	if err != nil {
		return startDir, err
	}

	var res DetectRootResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return startDir, err
	}

	return res.Root, nil
}

func (r *RemoteDataSource) LoadIgnoreFiles(root string) (*search.IgnoreRules, error) {
	id := int(time.Now().UnixNano() % 100000)
	
	r.sess.mu.Lock()
	err := r.sess.Enc.Send(Envelope{
		Method: MethodReadIgnore,
		ID:     id,
		Params: ReadIgnoreParams{Root: root},
	})
	r.sess.mu.Unlock()

	if err != nil {
		return &search.IgnoreRules{}, err
	}

	raw, err := r.sess.Dec.Receive()
	if err != nil {
		return &search.IgnoreRules{}, err
	}

	resp, err := ParseResponse(raw)
	if err != nil {
		return &search.IgnoreRules{}, err
	}

	var res ReadIgnoreResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return &search.IgnoreRules{}, err
	}

	rules := &search.IgnoreRules{}
	rules.LoadPatterns(res.Patterns)
	return rules, nil
}

func (r *RemoteDataSource) Done() <-chan struct{} {
	return r.sess.Done()
}

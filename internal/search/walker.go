package search

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FileEntry represents a single file discovered by the walker.
type FileEntry struct {
	Path    string    // Absolute file path
	RelPath string    // Relative to project root
	Name    string    // Base filename
	Ext     string    // File extension (e.g., ".go")
	Size    int64     // File size in bytes
	ModTime time.Time // Last modification time
	IsDir   bool
}

// WalkOptions controls walker behaviour.
type WalkOptions struct {
	// IncludeHidden allows walking hidden directories (dot-prefixed).
	IncludeHidden bool
	// FollowSymlinks follows symlinks one level deep.
	FollowSymlinks bool
	// Workers sets the number of concurrent directory readers.
	// Defaults to 8 if <= 0.
	Workers int
}

// Walk concurrently traverses the project tree rooted at root, emitting
// FileEntry values on the returned channel. The walk respects the given
// IgnoreRules and WalkOptions. Cancel the context to stop early.
func Walk(ctx context.Context, root string, rules *IgnoreRules, opts WalkOptions) <-chan FileEntry {
	if opts.Workers <= 0 {
		opts.Workers = 8
	}

	out := make(chan FileEntry, 1024)
	
	// Queue and coordination for bounded workers.
	var mu sync.Mutex
	cond := sync.NewCond(&mu)
	queue := []walkJob{{dir: root, depth: 0, symlink: false, rules: rules}}
	active := 0
	done := false

	var wg sync.WaitGroup
	for i := 0; i < opts.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				var job walkJob
				mu.Lock()
				for len(queue) == 0 && !done {
					cond.Wait()
				}
				if done {
					mu.Unlock()
					return
				}
				
				// Pop job from queue.
				job = queue[0]
				queue = queue[1:]
				active++
				mu.Unlock()

				// Process directory.
				newJobs := walkOneDir(ctx, root, job, opts, out)
				
				mu.Lock()
				active--
				queue = append(queue, newJobs...)
				if active == 0 && len(queue) == 0 {
					done = true
					cond.Broadcast()
				} else if len(newJobs) > 0 {
					cond.Broadcast()
				}
				mu.Unlock()
			}
		}()
	}

	// Close out channel once all workers are done.
	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

type walkJob struct {
	dir     string
	depth   int
	symlink bool // true if this dir was reached via a symlink
	rules   *IgnoreRules
}

func walkOneDir(
	ctx context.Context,
	root string,
	job walkJob,
	opts WalkOptions,
	out chan<- FileEntry,
) []walkJob {
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	entries, err := os.ReadDir(job.dir)
	if err != nil {
		return nil
	}

	currentRules := job.rules
	// Check for nested .gitignore.
	for _, e := range entries {
		if e.Name() == ".gitignore" && !e.IsDir() {
			// Copy rules before modifying so we don't affect other branches.
			currentRules = currentRules.Clone()
			currentRules.LoadNested(root, filepath.Join(job.dir, ".gitignore"))
		}
	}

	var newJobs []walkJob
	for _, e := range entries {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		name := e.Name()
		absPath := filepath.Join(job.dir, name)
		relPath, err := filepath.Rel(root, absPath)
		if err != nil {
			continue
		}

		info, err := e.Info()
		if err != nil {
			continue
		}

		isDir := e.IsDir()
		isSymlink := e.Type()&os.ModeSymlink != 0

		// Resolve symlinks.
		if isSymlink {
			if !opts.FollowSymlinks || job.symlink {
				continue
			}
			target, err := os.Stat(absPath)
			if err != nil {
				continue
			}
			isDir = target.IsDir()
			info = target
		}

		if !opts.IncludeHidden && strings.HasPrefix(name, ".") {
			continue
		}

		if currentRules.IsIgnored(relPath, isDir) {
			continue
		}

		if isDir {
			newJobs = append(newJobs, walkJob{
				dir:     absPath,
				depth:   job.depth + 1,
				symlink: isSymlink || job.symlink,
				rules:   currentRules,
			})
			continue
		}

		entry := FileEntry{
			Path:    absPath,
			RelPath: relPath,
			Name:    name,
			Ext:     filepath.Ext(name),
			Size:    info.Size(),
			ModTime: info.ModTime(),
			IsDir:   false,
		}

		select {
		case out <- entry:
		case <-ctx.Done():
			return nil
		}
	}
	return newJobs
}

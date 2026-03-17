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

	out := make(chan FileEntry, 256)
	dirs := make(chan walkJob, 256)
	var wg sync.WaitGroup

	// Seed with the root directory.
	dirs <- walkJob{dir: root, depth: 0, symlink: false, rules: rules}

	// Track in-flight directory jobs so we know when to close dirs.
	var inflight sync.WaitGroup
	inflight.Add(1)

	// Spawn workers.
	for range opts.Workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range dirs {
				walkDir(ctx, root, job, opts, out, dirs, &inflight)
			}
		}()
	}

	// Close dirs channel once all directory jobs are done, then wait for workers.
	go func() {
		inflight.Wait()
		close(dirs)
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

func walkDir(
	ctx context.Context,
	root string,
	job walkJob,
	opts WalkOptions,
	out chan<- FileEntry,
	dirs chan<- walkJob,
	inflight *sync.WaitGroup,
) {
	defer inflight.Done()

	select {
	case <-ctx.Done():
		return
	default:
	}

	entries, err := os.ReadDir(job.dir)
	if err != nil {
		return
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

	for _, e := range entries {
		select {
		case <-ctx.Done():
			return
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
				// Don't follow symlinks deeper than one level.
				continue
			}
			target, err := os.Stat(absPath)
			if err != nil {
				continue
			}
			isDir = target.IsDir()
			info = target
		}

		// Skip hidden entries unless configured.
		if !opts.IncludeHidden && strings.HasPrefix(name, ".") {
			continue
		}

		// Check ignore rules.
		if currentRules.IsIgnored(relPath, isDir) {
			continue
		}

		if isDir {
			inflight.Add(1)
			newJob := walkJob{
				dir:     absPath,
				depth:   job.depth + 1,
				symlink: isSymlink || job.symlink,
				rules:   currentRules,
			}
			select {
			case dirs <- newJob:
			case <-ctx.Done():
				inflight.Done()
				return
			default:
				// Buffer is full. Spawn a goroutine to avoid deadlocking the worker pool.
				go func(j walkJob) {
					select {
					case dirs <- j:
					case <-ctx.Done():
						inflight.Done()
					}
				}(newJob)
			}
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
			return
		}
	}
}

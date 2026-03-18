package remote

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yampug/btw/internal/search"
)

// HandleDetectRoot is the agent-side handler for "detect_root" requests.
// It walks up from start_dir looking for project root markers and returns
// the detected root path.
func HandleDetectRoot(ctx context.Context, id int, params json.RawMessage, enc *Encoder) error {
	var p DetectRootParams
	if err := json.Unmarshal(params, &p); err != nil {
		return enc.Send(NewErrorEnvelope(id, ErrCodeInternal, fmt.Sprintf("bad detect_root params: %v", err)))
	}

	if p.StartDir == "" {
		return enc.Send(NewErrorEnvelope(id, ErrCodeInternal, "detect_root: start_dir is required"))
	}

	// Verify start_dir exists.
	info, err := os.Stat(p.StartDir)
	if err != nil {
		if os.IsNotExist(err) {
			return enc.Send(NewNotFoundEnvelope(id, p.StartDir))
		}
		return enc.Send(NewInternalErrorEnvelope(id, err))
	}
	if !info.IsDir() {
		return enc.Send(NewErrorEnvelope(id, ErrCodeInternal, fmt.Sprintf("detect_root: %s is not a directory", p.StartDir)))
	}

	root := search.DetectRoot(p.StartDir)

	return enc.Send(Envelope{
		Method: MethodDetectRoot,
		ID:     id,
		Result: DetectRootResult{Root: root},
		Done:   true,
	})
}

// HandleReadIgnore is the agent-side handler for "read_ignore" requests.
// It reads .gitignore and .bmignore at the given root and returns
// the raw patterns as a list of strings.
func HandleReadIgnore(ctx context.Context, id int, params json.RawMessage, enc *Encoder) error {
	var p ReadIgnoreParams
	if err := json.Unmarshal(params, &p); err != nil {
		return enc.Send(NewErrorEnvelope(id, ErrCodeInternal, fmt.Sprintf("bad read_ignore params: %v", err)))
	}

	if p.Root == "" {
		return enc.Send(NewErrorEnvelope(id, ErrCodeInternal, "read_ignore: root is required"))
	}

	info, err := os.Stat(p.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return enc.Send(NewNotFoundEnvelope(id, p.Root))
		}
		return enc.Send(NewInternalErrorEnvelope(id, err))
	}
	if !info.IsDir() {
		return enc.Send(NewErrorEnvelope(id, ErrCodeInternal, fmt.Sprintf("read_ignore: %s is not a directory", p.Root)))
	}

	var patterns []string
	for _, name := range []string{".gitignore", ".bmignore"} {
		lines := readIgnoreFile(filepath.Join(p.Root, name))
		patterns = append(patterns, lines...)
	}

	return enc.Send(Envelope{
		Method: MethodReadIgnore,
		ID:     id,
		Result: ReadIgnoreResult{Patterns: patterns},
		Done:   true,
	})
}

// readIgnoreFile reads a .gitignore / .bmignore file and returns non-empty,
// non-comment lines.
func readIgnoreFile(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

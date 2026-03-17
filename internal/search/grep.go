package search

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/bob/boomerang/internal/model"
)

const (
	grepMaxLineLen = 1000
	grepDefaultMax = 200
	grepTextIcon   = "\U000f0219" // nf-md-file_document_outline
	grepTextColor  = "#6C8EBF"
)

// binaryExts lists common binary file extensions to skip.
var binaryExts = map[string]bool{
	".exe": true, ".bin": true, ".o": true, ".a": true, ".so": true,
	".dylib": true, ".dll": true, ".class": true, ".jar": true,
	".zip": true, ".gz": true, ".tar": true, ".bz2": true, ".xz": true,
	".7z": true, ".rar": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true,
	".ico": true, ".svg": true, ".webp": true, ".avif": true,
	".mp3": true, ".mp4": true, ".wav": true, ".avi": true, ".mov": true,
	".mkv": true, ".flac": true, ".ogg": true,
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".wasm": true, ".pyc": true, ".pyo": true,
	".ttf": true, ".otf": true, ".woff": true, ".woff2": true, ".eot": true,
}

// GrepOptions controls content search behavior.
type GrepOptions struct {
	IncludeHidden bool
	MaxResults    int
	ProjectOnly   bool
}

// GrepMatch is a single content match from grep search.
type GrepMatch struct {
	FilePath    string
	RelPath     string
	FileName    string
	Line        int
	Column      int
	Content     string
	MatchRanges []model.MatchRange
}

// Grep searches file contents for query using the index's file list.
// If query starts with "/", it is treated as a regex pattern.
// Cancel ctx to stop early. Results are sent on the returned channel.
func Grep(ctx context.Context, idx *Index, query string, opts GrepOptions) <-chan GrepMatch {
	out := make(chan GrepMatch, 64)
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = grepDefaultMax
	}

	go func() {
		defer close(out)

		if strings.TrimSpace(query) == "" {
			return
		}

		// Determine search mode: regex or literal.
		var re *regexp.Regexp
		var literal string
		if strings.HasPrefix(query, "/") && len(query) > 1 {
			pattern := query[1:]
			if strings.HasSuffix(pattern, "/") && len(pattern) > 1 {
				pattern = pattern[:len(pattern)-1]
			}
			var err error
			re, err = regexp.Compile("(?i)" + pattern)
			if err != nil {
				return
			}
		} else {
			literal = strings.ToLower(query)
		}

		files := idx.Files()
		count := 0

		for _, entry := range files {
			if ctx.Err() != nil {
				return
			}
			if entry.IsDir {
				continue
			}
			if !opts.IncludeHidden && strings.HasPrefix(entry.Name, ".") {
				continue
			}
			if opts.ProjectOnly && isVendored(entry.RelPath) {
				continue
			}
			if binaryExts[strings.ToLower(entry.Ext)] {
				continue
			}

			matches := grepFile(ctx, entry, literal, re, maxResults-count)
			for _, m := range matches {
				select {
				case out <- m:
					count++
					if count >= maxResults {
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out
}

// grepFile searches a single file for matches.
func grepFile(ctx context.Context, entry FileEntry, literal string, re *regexp.Regexp, limit int) []GrepMatch {
	f, err := os.Open(entry.Path)
	if err != nil {
		return nil
	}
	defer f.Close()

	// Quick binary check on first 512 bytes.
	header := make([]byte, 512)
	n, _ := f.Read(header)
	if n > 0 {
		for _, b := range header[:n] {
			if b == 0 {
				return nil
			}
		}
	}
	f.Seek(0, 0)

	var results []GrepMatch
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNum := 0

	for scanner.Scan() {
		if ctx.Err() != nil {
			break
		}
		lineNum++
		line := scanner.Text()

		if len(line) > grepMaxLineLen {
			continue
		}

		var matchRanges []model.MatchRange

		if re != nil {
			locs := re.FindAllStringIndex(line, -1)
			if len(locs) == 0 {
				continue
			}
			for _, loc := range locs {
				start := utf8.RuneCountInString(line[:loc[0]])
				end := start + utf8.RuneCountInString(line[loc[0]:loc[1]])
				matchRanges = append(matchRanges, model.MatchRange{Start: start, End: end})
			}
		} else {
			lowerLine := strings.ToLower(line)
			searchFrom := 0
			for {
				pos := strings.Index(lowerLine[searchFrom:], literal)
				if pos < 0 {
					break
				}
				byteStart := searchFrom + pos
				byteEnd := byteStart + len(literal)
				start := utf8.RuneCountInString(line[:byteStart])
				end := start + utf8.RuneCountInString(line[byteStart:byteEnd])
				matchRanges = append(matchRanges, model.MatchRange{Start: start, End: end})
				searchFrom = byteEnd
			}
			if len(matchRanges) == 0 {
				continue
			}
		}

		col := 0
		if len(matchRanges) > 0 {
			col = matchRanges[0].Start + 1
		}

		results = append(results, GrepMatch{
			FilePath:    entry.Path,
			RelPath:     entry.RelPath,
			FileName:    entry.Name,
			Line:        lineNum,
			Column:      col,
			Content:     line,
			MatchRanges: matchRanges,
		})

		if len(results) >= limit {
			break
		}
	}

	return results
}

// GrepMatchToResult converts a GrepMatch to a model.SearchResult for TUI display.
func GrepMatchToResult(m GrepMatch) model.SearchResult {
	content := strings.TrimSpace(m.Content)
	leadingBytes := len(m.Content) - len(strings.TrimLeft(m.Content, " \t"))
	leadingRunes := utf8.RuneCountInString(m.Content[:leadingBytes])

	contentRunes := []rune(content)

	// Adjust match ranges for trimmed leading whitespace.
	var ranges []model.MatchRange
	for _, mr := range m.MatchRanges {
		adj := model.MatchRange{
			Start: mr.Start - leadingRunes,
			End:   mr.End - leadingRunes,
		}
		if adj.Start < 0 {
			adj.Start = 0
		}
		if adj.End > len(contentRunes) {
			adj.End = len(contentRunes)
		}
		if adj.Start < adj.End {
			ranges = append(ranges, adj)
		}
	}

	// Truncate to ~60 runes.
	maxLen := 60
	if len(contentRunes) > maxLen {
		content = string(contentRunes[:maxLen]) + "…"
		var trimmed []model.MatchRange
		for _, mr := range ranges {
			if mr.Start >= maxLen {
				continue
			}
			if mr.End > maxLen {
				mr.End = maxLen
			}
			trimmed = append(trimmed, mr)
		}
		ranges = trimmed
	}

	dir := filepath.Dir(m.RelPath)
	if dir == "." {
		dir = ""
	}
	detail := fmt.Sprintf("%s:%d", m.FileName, m.Line)
	if dir != "" {
		detail += "  " + dir
	}

	return model.SearchResult{
		Name:        content,
		Detail:      detail,
		ResultType:  model.ResultText,
		FilePath:    m.FilePath,
		Line:        m.Line,
		Column:      m.Column,
		Score:       0,
		MatchRanges: ranges,
		Icon:        grepTextIcon,
		IconColor:   grepTextColor,
	}
}

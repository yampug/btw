package search

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/bob/boomerang/internal/model"
)

// SymbolKind classifies the type of a code symbol.
type SymbolKind int

const (
	SymbolFunc     SymbolKind = iota
	SymbolType
	SymbolConstant
	SymbolVariable
)

var symbolKindDisplay = map[SymbolKind]struct{ icon, color string }{
	SymbolFunc:     {"ƒ", "#A855F7"},
	SymbolType:     {"T", "#3B82F6"},
	SymbolConstant: {"C", "#F59E0B"},
	SymbolVariable: {"V", "#10B981"},
}

// Symbol represents a code symbol extracted from a source file.
type Symbol struct {
	Name      string // identifier name (e.g., "NewMatcher")
	Signature string // display text (e.g., "func NewMatcher(pattern string)")
	Kind      SymbolKind
	FilePath  string
	RelPath   string
	FileName  string
	Line      int
}

// langPattern defines a regex pattern for extracting symbols from a language.
type langPattern struct {
	re        *regexp.Regexp
	kind      SymbolKind
	nameGroup int // capture group index for the symbol name
}

// symbolPatterns maps file extensions to their extraction patterns.
// Order matters: first match wins per line.
var symbolPatterns map[string][]langPattern

func init() {
	symbolPatterns = make(map[string][]langPattern)

	// Go
	goPatterns := []langPattern{
		{regexp.MustCompile(`^func\s+\([^)]*\)\s+(\w+)\s*\(`), SymbolFunc, 1},
		{regexp.MustCompile(`^func\s+(\w+)\s*\(`), SymbolFunc, 1},
		{regexp.MustCompile(`^type\s+(\w+)\s`), SymbolType, 1},
		{regexp.MustCompile(`^interface\s+(\w+)\s`), SymbolType, 1},
		{regexp.MustCompile(`^const\s+(\w+)\s`), SymbolConstant, 1},
		{regexp.MustCompile(`^var\s+(\w+)\s`), SymbolVariable, 1},
	}
	symbolPatterns[".go"] = goPatterns

	// JavaScript
	jsPatterns := []langPattern{
		{regexp.MustCompile(`^(?:export\s+)?(?:async\s+)?function\s+(\w+)\s*\(`), SymbolFunc, 1},
		{regexp.MustCompile(`^(?:export\s+)?class\s+(\w+)`), SymbolType, 1},
		{regexp.MustCompile(`^(?:export\s+)?(?:const|let|var)\s+(\w+)\s*=`), SymbolVariable, 1},
	}
	symbolPatterns[".js"] = jsPatterns
	symbolPatterns[".jsx"] = jsPatterns
	symbolPatterns[".mjs"] = jsPatterns

	// TypeScript
	tsPatterns := []langPattern{
		{regexp.MustCompile(`^(?:export\s+)?(?:async\s+)?function\s+(\w+)\s*[<(]`), SymbolFunc, 1},
		{regexp.MustCompile(`^(?:export\s+)?class\s+(\w+)`), SymbolType, 1},
		{regexp.MustCompile(`^(?:export\s+)?interface\s+(\w+)`), SymbolType, 1},
		{regexp.MustCompile(`^(?:export\s+)?type\s+(\w+)\s*[<=]`), SymbolType, 1},
		{regexp.MustCompile(`^(?:export\s+)?(?:const|let|var)\s+(\w+)\s*[=:]`), SymbolVariable, 1},
	}
	symbolPatterns[".ts"] = tsPatterns
	symbolPatterns[".tsx"] = tsPatterns

	// Python
	pyPatterns := []langPattern{
		{regexp.MustCompile(`^(?:\s*)(?:async\s+)?def\s+(\w+)\s*\(`), SymbolFunc, 1},
		{regexp.MustCompile(`^(?:\s*)class\s+(\w+)`), SymbolType, 1},
	}
	symbolPatterns[".py"] = pyPatterns

	// Rust
	rsPatterns := []langPattern{
		{regexp.MustCompile(`^(?:\s*)(?:pub(?:\([^)]*\))?\s+)?(?:async\s+)?fn\s+(\w+)\s*[<(]`), SymbolFunc, 1},
		{regexp.MustCompile(`^(?:\s*)(?:pub(?:\([^)]*\))?\s+)?struct\s+(\w+)`), SymbolType, 1},
		{regexp.MustCompile(`^(?:\s*)(?:pub(?:\([^)]*\))?\s+)?enum\s+(\w+)`), SymbolType, 1},
		{regexp.MustCompile(`^(?:\s*)(?:pub(?:\([^)]*\))?\s+)?trait\s+(\w+)`), SymbolType, 1},
		{regexp.MustCompile(`^impl(?:<[^>]*>)?\s+(\w+)`), SymbolType, 1},
		{regexp.MustCompile(`^(?:\s*)(?:pub(?:\([^)]*\))?\s+)?const\s+(\w+)\s*:`), SymbolConstant, 1},
		{regexp.MustCompile(`^(?:\s*)(?:pub(?:\([^)]*\))?\s+)?static\s+(\w+)\s*:`), SymbolVariable, 1},
	}
	symbolPatterns[".rs"] = rsPatterns

	// Ruby
	rbPatterns := []langPattern{
		{regexp.MustCompile(`^\s*def\s+(?:self\.)?(\w+[?!=]?)`), SymbolFunc, 1},
		{regexp.MustCompile(`^\s*class\s+(\w+)`), SymbolType, 1},
		{regexp.MustCompile(`^\s*module\s+(\w+)`), SymbolType, 1},
	}
	symbolPatterns[".rb"] = rbPatterns

	// Java
	javaPatterns := []langPattern{
		{regexp.MustCompile(`(?:public|private|protected|static|abstract|final|\s)+class\s+(\w+)`), SymbolType, 1},
		{regexp.MustCompile(`(?:public|private|protected|static|abstract|final|\s)+interface\s+(\w+)`), SymbolType, 1},
		{regexp.MustCompile(`(?:public|private|protected|static|abstract|final|\s)+enum\s+(\w+)`), SymbolType, 1},
	}
	symbolPatterns[".java"] = javaPatterns

	// Kotlin
	ktPatterns := []langPattern{
		{regexp.MustCompile(`(?:fun|suspend\s+fun)\s+(?:<[^>]*>\s+)?(\w+)\s*\(`), SymbolFunc, 1},
		{regexp.MustCompile(`(?:class|data\s+class|sealed\s+class|object|interface)\s+(\w+)`), SymbolType, 1},
		{regexp.MustCompile(`(?:val|var)\s+(\w+)\s*[=:]`), SymbolVariable, 1},
	}
	symbolPatterns[".kt"] = ktPatterns
	symbolPatterns[".kts"] = ktPatterns
}

// ExtractSymbols scans all indexed files and extracts code symbols in parallel.
func (idx *Index) ExtractSymbols() {
	files := idx.Files()

	workers := 8
	var mu sync.Mutex
	var allSymbols []Symbol
	var wg sync.WaitGroup
	ch := make(chan FileEntry, 256)

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for entry := range ch {
				syms := extractFileSymbols(entry)
				if len(syms) > 0 {
					mu.Lock()
					allSymbols = append(allSymbols, syms...)
					mu.Unlock()
				}
			}
		}()
	}

	for _, f := range files {
		if f.IsDir {
			continue
		}
		ext := strings.ToLower(f.Ext)
		if _, ok := symbolPatterns[ext]; !ok {
			continue // unsupported language
		}
		if binaryExts[ext] {
			continue
		}
		ch <- f
	}
	close(ch)
	wg.Wait()

	idx.mu.Lock()
	idx.symbols = allSymbols
	idx.mu.Unlock()
}

// extractFileSymbols extracts symbols from a single file.
func extractFileSymbols(entry FileEntry) []Symbol {
	ext := strings.ToLower(entry.Ext)
	patterns, ok := symbolPatterns[ext]
	if !ok {
		return nil
	}

	f, err := os.Open(entry.Path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var symbols []Symbol
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNum := 0
	inComment := false

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip block comments (/* ... */).
		if inComment {
			if idx := strings.Index(line, "*/"); idx >= 0 {
				inComment = false
				line = line[idx+2:]
			} else {
				continue
			}
		}
		if idx := strings.Index(line, "/*"); idx >= 0 {
			if !strings.Contains(line[idx:], "*/") {
				inComment = true
			}
			line = line[:idx]
		}

		// Skip single-line comments.
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
			continue
		}

		for _, p := range patterns {
			m := p.re.FindStringSubmatch(line)
			if m == nil || p.nameGroup >= len(m) {
				continue
			}
			name := m[p.nameGroup]
			if name == "_" || name == "" {
				continue
			}

			sig := strings.TrimSpace(line)
			// Truncate very long signatures.
			if len([]rune(sig)) > 80 {
				sig = string([]rune(sig)[:80]) + "…"
			}

			symbols = append(symbols, Symbol{
				Name:      name,
				Signature: sig,
				Kind:      p.kind,
				FilePath:  entry.Path,
				RelPath:   entry.RelPath,
				FileName:  entry.Name,
				Line:      lineNum,
			})
			break // first match wins for this line
		}
	}

	return symbols
}

// SymbolToResult converts a Symbol to a model.SearchResult.
func SymbolToResult(sym Symbol, score int, ranges []model.MatchRange) model.SearchResult {
	display := symbolKindDisplay[sym.Kind]

	dir := filepath.Dir(sym.RelPath)
	if dir == "." {
		dir = ""
	}
	detail := sym.FileName
	if dir != "" {
		detail += "  " + dir
	}

	return model.SearchResult{
		Name:        sym.Signature,
		Detail:      detail,
		ResultType:  model.ResultSymbol,
		FilePath:    sym.FilePath,
		Line:        sym.Line,
		Score:       score,
		MatchRanges: ranges,
		Icon:        display.icon,
		IconColor:   display.color,
	}
}

// isClassLike returns true if the kind is type-level.
func isClassLike(k SymbolKind) bool {
	return k == SymbolType
}

func sortSymbolsAlpha(results []model.SearchResult) {
	n := len(results)
	for i := 1; i < n; i++ {
		for j := i; j > 0 && results[j].Name < results[j-1].Name; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}
}

// Symbols returns a snapshot of all extracted symbols.
func (idx *Index) Symbols() []Symbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	result := make([]Symbol, len(idx.symbols))
	copy(result, idx.symbols)
	return result
}

// SymbolCount returns the number of extracted symbols.
func (idx *Index) SymbolCount() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.symbols)
}

// SymbolMatchToDetail formats a symbol's detail string with line info.
func SymbolMatchToDetail(sym Symbol) string {
	dir := filepath.Dir(sym.RelPath)
	if dir == "." {
		dir = ""
	}
	detail := fmt.Sprintf("%s:%d", sym.FileName, sym.Line)
	if dir != "" {
		detail += "  " + dir
	}
	return detail
}

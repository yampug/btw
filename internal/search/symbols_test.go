package search

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/yampug/btw/internal/model"
)

func buildSymbolTestIndex(t *testing.T, files map[string]string) *Index {
	t.Helper()
	dir := t.TempDir()

	for name, content := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}

	idx := NewIndex()
	idx.RebuildFrom(context.Background(), NewLocalDataSource(), dir, WalkOptions{}, nil)
	return idx
}

func TestExtractSymbols_GoFunctions(t *testing.T) {
	idx := buildSymbolTestIndex(t, map[string]string{
		"main.go": `package main

func main() {
	fmt.Println("hello")
}

func NewMatcher(pattern string) *Matcher {
	return &Matcher{}
}
`,
	})

	syms := idx.Symbols()
	if len(syms) < 2 {
		t.Fatalf("expected at least 2 symbols, got %d", len(syms))
	}

	names := map[string]bool{}
	for _, s := range syms {
		names[s.Name] = true
		if s.Kind != SymbolFunc {
			t.Errorf("expected SymbolFunc for %s, got %d", s.Name, s.Kind)
		}
	}
	if !names["main"] || !names["NewMatcher"] {
		t.Errorf("expected main and NewMatcher, got %v", names)
	}
}

func TestExtractSymbols_GoMethods(t *testing.T) {
	idx := buildSymbolTestIndex(t, map[string]string{
		"matcher.go": `package search

func (m *Matcher) Match(s string) bool {
	return true
}
`,
	})

	syms := idx.Symbols()
	if len(syms) != 1 {
		t.Fatalf("expected 1 symbol, got %d", len(syms))
	}
	if syms[0].Name != "Match" || syms[0].Kind != SymbolFunc {
		t.Errorf("expected Match func, got %+v", syms[0])
	}
}

func TestExtractSymbols_GoTypes(t *testing.T) {
	idx := buildSymbolTestIndex(t, map[string]string{
		"types.go": `package model

type SearchResult struct {
	Name string
}

type Matcher interface {
	Match(s string) bool
}
`,
	})

	syms := idx.Symbols()
	if len(syms) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(syms))
	}
	for _, s := range syms {
		if s.Kind != SymbolType {
			t.Errorf("expected SymbolType for %s, got %d", s.Name, s.Kind)
		}
	}
}

func TestExtractSymbols_GoConstVar(t *testing.T) {
	idx := buildSymbolTestIndex(t, map[string]string{
		"consts.go": `package main

const MaxRetries = 3
var DefaultTimeout = 30
`,
	})

	syms := idx.Symbols()
	if len(syms) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(syms))
	}
	kinds := map[string]SymbolKind{}
	for _, s := range syms {
		kinds[s.Name] = s.Kind
	}
	if kinds["MaxRetries"] != SymbolConstant {
		t.Error("expected MaxRetries to be constant")
	}
	if kinds["DefaultTimeout"] != SymbolVariable {
		t.Error("expected DefaultTimeout to be variable")
	}
}

func TestExtractSymbols_Python(t *testing.T) {
	idx := buildSymbolTestIndex(t, map[string]string{
		"app.py": `class Application:
    def __init__(self):
        pass

    def run(self):
        pass

async def fetch_data(url):
    pass

def helper():
    pass
`,
	})

	syms := idx.Symbols()
	names := map[string]SymbolKind{}
	for _, s := range syms {
		names[s.Name] = s.Kind
	}
	if names["Application"] != SymbolType {
		t.Error("expected Application type")
	}
	if names["helper"] != SymbolFunc {
		t.Error("expected helper func")
	}
	if names["fetch_data"] != SymbolFunc {
		t.Error("expected fetch_data func")
	}
}

func TestExtractSymbols_Rust(t *testing.T) {
	idx := buildSymbolTestIndex(t, map[string]string{
		"lib.rs": `pub struct Config {
    name: String,
}

pub enum Color {
    Red,
    Blue,
}

pub fn new_config(name: &str) -> Config {
    Config { name: name.to_string() }
}

impl Config {
    pub fn name(&self) -> &str {
        &self.name
    }
}

pub trait Configurable {
    fn configure(&mut self);
}
`,
	})

	syms := idx.Symbols()
	names := map[string]SymbolKind{}
	for _, s := range syms {
		names[s.Name] = s.Kind
	}
	if names["Config"] != SymbolType {
		t.Errorf("expected Config type, kinds=%v", names)
	}
	if names["Color"] != SymbolType {
		t.Error("expected Color type")
	}
	if names["new_config"] != SymbolFunc {
		t.Error("expected new_config func")
	}
	if names["Configurable"] != SymbolType {
		t.Error("expected Configurable type")
	}
}

func TestExtractSymbols_TypeScript(t *testing.T) {
	idx := buildSymbolTestIndex(t, map[string]string{
		"api.ts": `export interface ApiResponse {
    data: unknown;
}

export type UserId = string;

export class ApiClient {
    constructor(private url: string) {}
}

export function fetchData(url: string): Promise<void> {
    return fetch(url).then(() => {});
}

export const API_VERSION = "v1";
`,
	})

	syms := idx.Symbols()
	names := map[string]SymbolKind{}
	for _, s := range syms {
		names[s.Name] = s.Kind
	}
	if names["ApiResponse"] != SymbolType {
		t.Error("expected ApiResponse type")
	}
	if names["UserId"] != SymbolType {
		t.Error("expected UserId type")
	}
	if names["ApiClient"] != SymbolType {
		t.Error("expected ApiClient type")
	}
	if names["fetchData"] != SymbolFunc {
		t.Error("expected fetchData func")
	}
	if names["API_VERSION"] != SymbolVariable {
		t.Error("expected API_VERSION variable")
	}
}

func TestExtractSymbols_Ruby(t *testing.T) {
	idx := buildSymbolTestIndex(t, map[string]string{
		"app.rb": `module MyApp
  class Server
    def initialize(port)
      @port = port
    end

    def self.start!
      new(8080).run
    end

    def running?
      true
    end
  end
end
`,
	})

	syms := idx.Symbols()
	names := map[string]bool{}
	for _, s := range syms {
		names[s.Name] = true
	}
	if !names["MyApp"] {
		t.Error("expected MyApp module")
	}
	if !names["Server"] {
		t.Error("expected Server class")
	}
	if !names["initialize"] {
		t.Error("expected initialize method")
	}
	if !names["start!"] {
		t.Error("expected start! method")
	}
	if !names["running?"] {
		t.Error("expected running? method")
	}
}

func TestExtractSymbols_SkipsComments(t *testing.T) {
	idx := buildSymbolTestIndex(t, map[string]string{
		"main.go": `package main

// func NotAFunction() {}

/* func AlsoNotAFunction() {} */

func RealFunction() {}
`,
	})

	syms := idx.Symbols()
	if len(syms) != 1 {
		t.Fatalf("expected 1 symbol, got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "RealFunction" {
		t.Errorf("expected RealFunction, got %s", syms[0].Name)
	}
}

func TestExtractSymbols_SkipsUnsupportedFiles(t *testing.T) {
	idx := buildSymbolTestIndex(t, map[string]string{
		"readme.md":    "# Hello\nfunc not_code() {}\n",
		"data.json":    `{"func": "not_code"}`,
		"real.go":      "package main\nfunc RealFunc() {}\n",
	})

	syms := idx.Symbols()
	if len(syms) != 1 {
		t.Fatalf("expected 1 symbol from .go file only, got %d", len(syms))
	}
}

func TestSearchSymbols_FuzzyMatch(t *testing.T) {
	idx := buildSymbolTestIndex(t, map[string]string{
		"search.go": `package search

func NewMatcher(pattern string) *Matcher {
	return nil
}

func FuzzyMatch(query, candidate string) MatchResult {
	return MatchResult{}
}

type MatchResult struct {}
`,
	})

	results := idx.SearchSymbols(context.Background(), "NM", 100, false, false, nil).Items
	if len(results) == 0 {
		t.Fatal("expected CamelCase match for 'NM' → NewMatcher")
	}
	// NewMatcher should be the top result.
	found := false
	for _, r := range results {
		if r.ResultType == model.ResultSymbol && contains(r.Name, "NewMatcher") {
			found = true
		}
	}
	if !found {
		t.Error("expected NewMatcher in results")
	}
}

func TestSearchSymbols_EmptyQuery(t *testing.T) {
	idx := buildSymbolTestIndex(t, map[string]string{
		"main.go": `package main

func Bravo() {}
func Alpha() {}
`,
	})

	results := idx.SearchSymbols(context.Background(), "", 100, false, false, nil).Items
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}
	// Should be sorted alphabetically.
	if !contains(results[0].Name, "Alpha") {
		t.Errorf("expected Alpha first (alphabetical), got %s", results[0].Name)
	}
}

func TestSearchSymbols_MaxResults(t *testing.T) {
	idx := buildSymbolTestIndex(t, map[string]string{
		"funcs.go": `package main

func A() {}
func B() {}
func C() {}
func D() {}
func E() {}
`,
	})

	rs := idx.SearchSymbols(context.Background(), "", 3, false, false, nil)
	if len(rs.Items) != 3 {
		t.Errorf("expected 3 results, got %d", len(rs.Items))
	}
	if rs.TotalMatched != 5 {
		t.Errorf("expected TotalMatched=5, got %d", rs.TotalMatched)
	}
}

func TestSearchSymbols_ResultFormat(t *testing.T) {
	idx := buildSymbolTestIndex(t, map[string]string{
		"sub/matcher.go": `package search

func NewMatcher() {}
type Matcher struct {}
`,
	})

	results := idx.SearchSymbols(context.Background(), "", 100, false, false, nil).Items
	for _, r := range results {
		if r.Icon == "" {
			t.Errorf("expected icon for %s", r.Name)
		}
		if r.IconColor == "" {
			t.Errorf("expected icon color for %s", r.Name)
		}
		if r.ResultType != model.ResultSymbol {
			t.Errorf("expected ResultSymbol for %s", r.Name)
		}
		if r.FilePath == "" {
			t.Errorf("expected file path for %s", r.Name)
		}
		if r.Line == 0 {
			t.Errorf("expected line number for %s", r.Name)
		}
	}
}

func TestSearchSymbols_SkipsHidden(t *testing.T) {
	// Build index with hidden files included so we can test filtering
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".hidden.go"), []byte("package h\nfunc Hidden() {}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "visible.go"), []byte("package v\nfunc Visible() {}\n"), 0o644)

	idx := NewIndex()
	idx.RebuildFrom(context.Background(), NewLocalDataSource(), dir, WalkOptions{IncludeHidden: true}, nil)

	results := idx.SearchSymbols(context.Background(), "", 100, false, false, nil).Items
	for _, r := range results {
		if contains(r.Name, "Hidden") {
			t.Error("should skip hidden files when includeHidden=false")
		}
	}

	resultsAll := idx.SearchSymbols(context.Background(), "", 100, true, false, nil).Items
	foundHidden := false
	for _, r := range resultsAll {
		if contains(r.Name, "Hidden") {
			foundHidden = true
		}
	}
	if !foundHidden {
		t.Error("should include hidden files when includeHidden=true")
	}
}

func TestSearchClasses_FiltersTypeSymbols(t *testing.T) {
	idx := buildSymbolTestIndex(t, map[string]string{
		"main.go": `package main

type User struct {
	Name string
}

func (u *User) GetName() string {
	return u.Name
}

interface Error {
	Error() string
}

const MaxUsers = 100

var globalVar = "test"
`,
	})

	// SearchClasses should only return type-level symbols
	results := idx.SearchClasses(context.Background(), "", 100, false, false, nil).Items
	
	// Should find User (struct) and Error (interface), but not functions, consts, or vars
	if len(results) != 2 {
		t.Errorf("expected 2 type symbols, got %d", len(results))
	}
	
	foundUser := false
	foundError := false
	for _, r := range results {
		if contains(r.Name, "User") {
			foundUser = true
		}
		if contains(r.Name, "Error") {
			foundError = true
		}
		// Ensure no function/const/var symbols slip through
		if contains(r.Name, "GetName") || contains(r.Name, "MaxUsers") || contains(r.Name, "globalVar") {
			t.Errorf("SearchClasses should not return non-type symbols, found: %s", r.Name)
		}
	}
	
	if !foundUser {
		t.Error("expected to find User struct")
	}
	if !foundError {
		t.Error("expected to find Error interface")
	}
}

func TestSearchClasses_FuzzyMatch(t *testing.T) {
	idx := buildSymbolTestIndex(t, map[string]string{
		"types.go": `package main

type SearchResult struct {
	Name string
}

type Matcher struct {
	pattern string
}

type SearchProvider interface {
	Search(query string) []SearchResult
}
`,
	})

	results := idx.SearchSymbols(context.Background(), "SR", 100, false, false, nil).Items
	if len(results) != 1 || !contains(results[0].Name, "SearchResult") {
		t.Errorf("expected to find SearchResult with query 'SR', got %v", results)
	}
}

func TestSearchClasses_EmptyQuery(t *testing.T) {
	idx := buildSymbolTestIndex(t, map[string]string{
		"types.go": `package main

type A struct {}
type B struct {}
type C struct {}
`,
	})

	results := idx.SearchClasses(context.Background(), "", 100, false, false, nil).Items
	if len(results) != 3 {
		t.Errorf("expected 3 type symbols for empty query, got %d", len(results))
	}
	
	// Should be sorted alphabetically for empty query
	if results[0].Name != "type A struct {}" {
		t.Errorf("expected first result to be 'type A struct {}', got %s", results[0].Name)
	}
}

func TestExtractSymbols_LineNumbers(t *testing.T) {
	idx := buildSymbolTestIndex(t, map[string]string{
		"main.go": `package main

import "fmt"

func main() {
	fmt.Println("hello")
}

type Config struct {
	Name string
}
`,
	})

	syms := idx.Symbols()
	lineMap := map[string]int{}
	for _, s := range syms {
		lineMap[s.Name] = s.Line
	}
	if lineMap["main"] != 5 {
		t.Errorf("expected main at line 5, got %d", lineMap["main"])
	}
	if lineMap["Config"] != 9 {
		t.Errorf("expected Config at line 9, got %d", lineMap["Config"])
	}
}

func TestExtractSymbols_Parallel(t *testing.T) {
	// Verify no races with many files.
	files := make(map[string]string)
	for i := range 20 {
		name := filepath.Join("pkg", fmt.Sprintf("f%d.go", i))
		files[name] = fmt.Sprintf("package pkg\nfunc Func%d() {}\ntype Type%d struct {}\n", i, i)
	}

	idx := buildSymbolTestIndex(t, files)
	if idx.SymbolCount() != 40 {
		t.Errorf("expected 40 symbols (20 funcs + 20 types), got %d", idx.SymbolCount())
	}
}

// contains checks if substr appears in s.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && findSubstr(s, substr))
}

func findSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

package model

// ResultType classifies what kind of item a search result is.
type ResultType int

const (
	ResultAll    ResultType = iota
	ResultClass
	ResultFile
	ResultSymbol
	ResultAction
	ResultText
)

var resultTypeStrings = [...]string{
	"All",
	"Class",
	"File",
	"Symbol",
	"Action",
	"Text",
}

func (r ResultType) String() string {
	if int(r) < len(resultTypeStrings) {
		return resultTypeStrings[r]
	}
	return "Unknown"
}

// Tab represents one of the search tabs.
type Tab int

const (
	TabAll Tab = iota
	TabClasses
	TabFiles
	TabSymbols
	TabActions
	TabText
)

var tabStrings = [...]string{
	"All",
	"Classes",
	"Files",
	"Symbols",
	"Actions",
	"Text",
}

func (t Tab) String() string {
	if int(t) < len(tabStrings) {
		return tabStrings[t]
	}
	return "Unknown"
}

// MatchRange represents a character range within a name that matched the query.
// Start is inclusive, End is exclusive. Used for per-character highlight rendering
// rather than simple substring matching.
type MatchRange struct {
	Start int
	End   int
}

// SearchResult represents a single item in the result list.
type SearchResult struct {
	Name        string       // Display name (e.g., "main.go")
	Detail      string       // Secondary info (e.g., "cmd/boomerang/")
	ResultType  ResultType   // File, Symbol, Action, Text, Class
	FilePath    string       // Absolute path to the file
	Line        int          // Line number (0 = not applicable)
	Column      int          // Column number (0 = not applicable)
	Score       int          // Relevance score (higher = better)
	MatchRanges []MatchRange // Character ranges that matched the query
	Icon        string       // Nerd Font icon or emoji
	IconColor   string       // Lip Gloss color for the icon
	IsHeader    bool         // If true, this item is a section header
	SectionTab  Tab          // Tab to switch to when clicking 'more...' or the header
}

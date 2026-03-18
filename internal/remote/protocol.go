// Package remote implements the JSON-Lines protocol for communication between
// a local btw client and a remote btw-agent process over SSH stdin/stdout.
//
// # Wire Format
//
// Each message is a single JSON object terminated by a newline character ('\n').
// Messages MUST NOT contain embedded newlines within the JSON.
//
// # Request / Response Semantics
//
// Every request carries an integer "id" field. The agent echoes this "id" on all
// response messages for that request — including streamed results. This allows the
// client to multiplex concurrent requests (though the initial implementation is
// single-threaded).
//
// # Streaming
//
// Methods that produce multiple results (walk, grep, symbols) stream them as
// individual response messages sharing the same "id". Each streamed message has
// "done": false. The final message for a stream sets "done": true and may carry
// summary metadata but no result payload.
//
// # Cancellation
//
// The client may send a "cancel" request with the "id" of an in-flight operation.
// The agent MUST stop the corresponding work within one iteration cycle and send
// a final response with "done": true.
//
// # Errors
//
// Error responses use the shape:
//
//	{"id": N, "error": {"code": <int>, "message": "<string>"}}
//
// Error codes:
//
//	1 = NotFound          — requested path does not exist
//	2 = PermissionDenied  — OS-level permission error
//	3 = Cancelled         — request was cancelled by the client
//	4 = InternalError     — unexpected agent-side failure
package remote

import "time"

// ---------------------------------------------------------------------------
// Method constants
// ---------------------------------------------------------------------------

// Method identifies the type of a protocol message.
const (
	MethodWalk       = "walk"
	MethodWalkEntry  = "walk_entry"
	MethodGrep       = "grep"
	MethodGrepMatch  = "grep_match"
	MethodSymbols    = "symbols"
	MethodSymbolEntry = "symbol_entry"
	MethodReadIgnore = "read_ignore"
	MethodDetectRoot = "detect_root"
	MethodCancel     = "cancel"
	MethodPing       = "ping"
	MethodPong       = "pong"
)

// ---------------------------------------------------------------------------
// Error codes
// ---------------------------------------------------------------------------

const (
	ErrCodeNotFound         = 1
	ErrCodePermissionDenied = 2
	ErrCodeCancelled        = 3
	ErrCodeInternal         = 4
)

// ---------------------------------------------------------------------------
// Envelope — the outer wrapper for every message on the wire
// ---------------------------------------------------------------------------

// Envelope is the top-level JSON object sent over the wire. Every message —
// request, response, or streamed result — is wrapped in an Envelope.
//
// For requests:  Method + ID + Params are set; Result/Error/Done are zero.
// For responses: Method + ID + (Result | Error) + Done are set.
type Envelope struct {
	Method string      `json:"method"`
	ID     int         `json:"id"`
	Params interface{} `json:"params,omitempty"`
	Result interface{} `json:"result,omitempty"`
	Error  *ProtoError `json:"error,omitempty"`
	Done   bool        `json:"done,omitempty"`
}

// ProtoError represents a structured error in a response message.
type ProtoError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Error implements the error interface.
func (e *ProtoError) Error() string {
	return e.Message
}

// ---------------------------------------------------------------------------
// Request parameter structs (client → agent)
// ---------------------------------------------------------------------------

// WalkParams are the parameters for a "walk" request.
type WalkParams struct {
	Root           string   `json:"root"`
	IncludeHidden  bool     `json:"include_hidden,omitempty"`
	FollowSymlinks bool     `json:"follow_symlinks,omitempty"`
	MaxDepth       int      `json:"max_depth,omitempty"`
	IgnorePatterns []string `json:"ignore_patterns,omitempty"`
}

// GrepParams are the parameters for a "grep" request.
type GrepParams struct {
	Root          string `json:"root"`
	Query         string `json:"query"`
	MaxResults    int    `json:"max_results,omitempty"`
	IncludeHidden bool   `json:"include_hidden,omitempty"`
	ProjectOnly   bool   `json:"project_only,omitempty"`
}

// SymbolsParams are the parameters for a "symbols" request.
type SymbolsParams struct {
	Root       string `json:"root"`
	KindFilter string `json:"kind_filter,omitempty"` // "func", "type", "constant", "variable", or "" for all
}

// ReadIgnoreParams are the parameters for a "read_ignore" request.
type ReadIgnoreParams struct {
	Root string `json:"root"`
}

// DetectRootParams are the parameters for a "detect_root" request.
type DetectRootParams struct {
	StartDir string `json:"start_dir"`
}

// CancelParams are the parameters for a "cancel" request.
type CancelParams struct {
	CancelID int `json:"cancel_id"` // ID of the request to cancel
}

// PingParams are the parameters for a "ping" request (currently empty).
type PingParams struct{}

// ---------------------------------------------------------------------------
// Response / streamed result structs (agent → client)
// ---------------------------------------------------------------------------

// FileEntryResult is a single file entry streamed in response to a "walk" request.
// It mirrors search.FileEntry but with JSON-serialisable fields.
type FileEntryResult struct {
	Path    string    `json:"path"`
	RelPath string    `json:"rel_path"`
	Name    string    `json:"name"`
	Ext     string    `json:"ext"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
	IsDir   bool      `json:"is_dir,omitempty"`
}

// GrepMatchResult is a single content match streamed in response to a "grep" request.
// It mirrors search.GrepMatch but with JSON-serialisable fields.
type GrepMatchResult struct {
	FilePath    string       `json:"file_path"`
	RelPath     string       `json:"rel_path"`
	FileName    string       `json:"file_name"`
	Line        int          `json:"line"`
	Column      int          `json:"column"`
	Content     string       `json:"content"`
	MatchRanges []MatchRange `json:"match_ranges,omitempty"`
}

// MatchRange represents a character range within a matched line.
// Start is inclusive, End is exclusive.
type MatchRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// SymbolEntryResult is a single symbol streamed in response to a "symbols" request.
// It mirrors search.Symbol but with JSON-serialisable fields.
type SymbolEntryResult struct {
	Name      string `json:"name"`
	Signature string `json:"signature"`
	Kind      string `json:"kind"` // "func", "type", "constant", "variable"
	FilePath  string `json:"file_path"`
	RelPath   string `json:"rel_path"`
	FileName  string `json:"file_name"`
	Line      int    `json:"line"`
}

// ReadIgnoreResult is the response to a "read_ignore" request.
type ReadIgnoreResult struct {
	Patterns []string `json:"patterns"`
}

// DetectRootResult is the response to a "detect_root" request.
type DetectRootResult struct {
	Root string `json:"root"`
}

// PongResult is the response to a "ping" request.
type PongResult struct{}

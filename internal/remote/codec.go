package remote

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Encoder writes JSON-Lines messages to an io.Writer.
// Each call to Send marshals the value as a single JSON line terminated by '\n'.
// Encoder is safe for concurrent use.
type Encoder struct {
	mu sync.Mutex
	w  io.Writer
}

// NewEncoder returns an Encoder that writes to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// Send marshals msg as JSON and writes it followed by a newline.
func (e *Encoder) Send(msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("remote: marshal: %w", err)
	}

	// Append newline delimiter.
	data = append(data, '\n')

	e.mu.Lock()
	defer e.mu.Unlock()

	_, err = e.w.Write(data)
	if err != nil {
		return fmt.Errorf("remote: write: %w", err)
	}
	return nil
}

// Decoder reads JSON-Lines messages from an io.Reader.
// Each call to Receive reads one newline-delimited line and returns it as
// raw JSON. Decoder is NOT safe for concurrent use — callers must serialise
// calls to Receive.
type Decoder struct {
	scanner *bufio.Scanner
}

// NewDecoder returns a Decoder that reads from r.
// The internal buffer is sized to handle lines up to 10 MB.
func NewDecoder(r io.Reader) *Decoder {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	return &Decoder{scanner: s}
}

// Receive reads the next JSON-Lines message and returns the raw JSON.
// It returns io.EOF when the underlying reader is exhausted.
func (d *Decoder) Receive() (json.RawMessage, error) {
	if !d.scanner.Scan() {
		if err := d.scanner.Err(); err != nil {
			return nil, fmt.Errorf("remote: read: %w", err)
		}
		return nil, io.EOF
	}

	line := d.scanner.Bytes()
	if len(line) == 0 {
		// Skip blank lines (shouldn't happen in well-formed streams, but
		// be defensive).
		return d.Receive()
	}

	// Copy to avoid holding a reference to the scanner's internal buffer.
	raw := make(json.RawMessage, len(line))
	copy(raw, line)
	return raw, nil
}

// requestShape is used internally by ParseRequest to extract routing fields
// without deserialising the full params payload.
type requestShape struct {
	Method string          `json:"method"`
	ID     int             `json:"id"`
	Params json.RawMessage `json:"params"`
}

// ParseRequest extracts the method, id, and raw params from an incoming
// JSON-Lines message. This is the primary demux function used by the agent's
// request router.
//
// The returned params is the unparsed JSON of the "params" field (or nil if
// absent). Callers should json.Unmarshal it into the appropriate typed struct
// based on the method.
func ParseRequest(raw json.RawMessage) (method string, id int, params json.RawMessage, err error) {
	var shape requestShape
	if err := json.Unmarshal(raw, &shape); err != nil {
		return "", 0, nil, fmt.Errorf("remote: parse request: %w", err)
	}
	if shape.Method == "" {
		return "", 0, nil, fmt.Errorf("remote: parse request: missing method field")
	}
	return shape.Method, shape.ID, shape.Params, nil
}

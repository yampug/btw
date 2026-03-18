package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// DefaultAgentPath is the default location of btw-agent on the remote host.
const DefaultAgentPath = "~/.local/bin/btw-agent"

// DefaultSSHPort is the default SSH port.
const DefaultSSHPort = 22

// SessionConfig holds the configuration for a remote SSH session.
type SessionConfig struct {
	Host      string // e.g., "dev-server" or "user@192.168.1.10"
	Port      int    // SSH port, default 22
	AgentPath string // path to btw-agent on remote, default DefaultAgentPath
	SSHBinary string // path to ssh binary, default "ssh"
}

// Session represents an active connection to a remote btw-agent over SSH.
// It wraps a subprocess running `ssh <host> <agent-path>` and provides
// Encoder/Decoder access to the agent's stdin/stdout.
type Session struct {
	config  SessionConfig
	cmd     *exec.Cmd
	Enc     *Encoder
	Dec     *Decoder
	stderr  *stderrCollector
	cancel  context.CancelFunc
	done    chan struct{} // closed when the SSH process exits
	doneErr error        // exit error from the SSH process

	mu     sync.Mutex
	closed bool
}

// stderrCollector captures stderr output from the SSH process for
// diagnostics. It stores the last N bytes in a ring buffer.
type stderrCollector struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func newStderrCollector(maxBytes int) *stderrCollector {
	return &stderrCollector{max: maxBytes}
}

func (s *stderrCollector) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf = append(s.buf, p...)
	if len(s.buf) > s.max {
		s.buf = s.buf[len(s.buf)-s.max:]
	}
	return len(p), nil
}

func (s *stderrCollector) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return string(s.buf)
}

// Dial creates and starts a new SSH session to the remote host.
// It launches `ssh <host> <agent-path>` as a subprocess and wires up
// the JSON-Lines codec over the process's stdin/stdout.
//
// Use the returned Session's Enc/Dec fields to send/receive protocol
// messages. Call Close() when done.
func Dial(ctx context.Context, cfg SessionConfig) (*Session, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("remote: ssh: host is required")
	}
	if cfg.Port <= 0 {
		cfg.Port = DefaultSSHPort
	}
	if cfg.AgentPath == "" {
		cfg.AgentPath = DefaultAgentPath
	}
	if cfg.SSHBinary == "" {
		cfg.SSHBinary = "ssh"
	}

	// Build the ssh command.
	args := []string{
		"-o", "BatchMode=yes",         // no interactive prompts
		"-o", "StrictHostKeyChecking=accept-new", // auto-accept new keys
	}
	if cfg.Port != DefaultSSHPort {
		args = append(args, "-p", strconv.Itoa(cfg.Port))
	}
	args = append(args, cfg.Host, cfg.AgentPath)

	ctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, cfg.SSHBinary, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("remote: ssh: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("remote: ssh: stdout pipe: %w", err)
	}

	stderrBuf := newStderrCollector(8192)
	cmd.Stderr = stderrBuf

	s := &Session{
		config: cfg,
		cmd:    cmd,
		Enc:    NewEncoder(stdin),
		Dec:    NewDecoder(stdout),
		stderr: stderrBuf,
		cancel: cancel,
		done:   make(chan struct{}),
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, &SSHError{
			Host:    cfg.Host,
			Message: fmt.Sprintf("failed to start ssh: %v", err),
		}
	}

	// Wait for the process in the background.
	go func() {
		s.doneErr = cmd.Wait()
		close(s.done)
	}()

	return s, nil
}

// Ping sends a ping request and waits for a pong, verifying the agent
// is alive and responsive. Returns an error if the agent doesn't respond
// within the context deadline.
func (s *Session) Ping(ctx context.Context) error {
	if err := s.Enc.Send(Envelope{Method: MethodPing, ID: 0}); err != nil {
		return fmt.Errorf("remote: ping: send: %w", err)
	}

	// Read in a goroutine so we can respect the context deadline.
	type result struct {
		raw json.RawMessage
		err error
	}
	ch := make(chan result, 1)
	go func() {
		raw, err := s.Dec.Receive()
		ch <- result{raw, err}
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("remote: ping: %w", ctx.Err())
	case r := <-ch:
		if r.err != nil {
			return fmt.Errorf("remote: ping: receive: %w", r.err)
		}
		resp, err := ParseResponse(r.raw)
		if err != nil {
			return fmt.Errorf("remote: ping: %w", err)
		}
		if resp.Method != MethodPong {
			return fmt.Errorf("remote: ping: unexpected response method %q", resp.Method)
		}
		return nil
	case <-s.done:
		return &SSHError{
			Host:    s.config.Host,
			Message: fmt.Sprintf("ssh process exited: %v; stderr: %s", s.doneErr, s.StderrOutput()),
		}
	}
}

// Close terminates the SSH session. It is safe to call multiple times.
func (s *Session) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	s.cancel() // signals exec.CommandContext to kill the process

	// Wait for the process to exit (with a short grace period since
	// cancel already sent SIGKILL via CommandContext).
	<-s.done

	return s.doneErr
}

// StderrOutput returns the captured stderr from the SSH process.
// Useful for diagnostics when the connection fails.
func (s *Session) StderrOutput() string {
	return strings.TrimSpace(s.stderr.String())
}

// Done returns a channel that is closed when the SSH process exits.
func (s *Session) Done() <-chan struct{} {
	return s.done
}

// Alive returns true if the SSH process is still running.
func (s *Session) Alive() bool {
	select {
	case <-s.done:
		return false
	default:
		return true
	}
}

// ExitError returns the SSH process's exit error, or nil if still running.
func (s *Session) ExitError() error {
	select {
	case <-s.done:
		return s.doneErr
	default:
		return nil
	}
}

// SSHError is returned when the SSH connection itself fails (as opposed
// to protocol-level errors from the agent).
type SSHError struct {
	Host    string
	Message string
}

func (e *SSHError) Error() string {
	return fmt.Sprintf("ssh %s: %s", e.Host, e.Message)
}

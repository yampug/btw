package remote

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// buildTestAgent builds the btw-agent binary into a temp dir and returns
// the path. This is used to test the Session against a real agent process
// without needing SSH.
func buildTestAgent(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	binPath := filepath.Join(dir, "btw-agent")

	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/btw-agent")
	cmd.Dir = findModuleRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build test agent: %v\n%s", err, out)
	}

	return binPath
}

// findModuleRoot walks up to find go.mod.
func findModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod")
		}
		dir = parent
	}
}

// createFakeSSH creates a shell script that runs the agent binary directly
// (simulating ssh launching btw-agent). It ignores the "host" argument
// and just execs the agent path argument.
func createFakeSSH(t *testing.T, agentBin string) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-ssh")
	content := "#!/bin/sh\n# Fake SSH: skip ssh flags and host, exec the agent binary.\n" +
		"# Usage: fake-ssh [ssh-flags...] <host> <agent-path>\n" +
		"# We just exec the last argument.\n" +
		"exec \"" + agentBin + "\"\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

// ---------------------------------------------------------------------------
// Session.Start (via Dial) + Ping
// ---------------------------------------------------------------------------

func TestSession_DialAndPing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping agent build in short mode")
	}

	agentBin := buildTestAgent(t)
	fakeSSH := createFakeSSH(t, agentBin)

	ctx := context.Background()
	sess, err := Dial(ctx, SessionConfig{
		Host:      "test-host",
		AgentPath: agentBin,
		SSHBinary: fakeSSH,
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer sess.Close()

	// Ping should succeed.
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := sess.Ping(pingCtx); err != nil {
		t.Fatalf("ping: %v", err)
	}

	if !sess.Alive() {
		t.Error("session should be alive after ping")
	}
}

// ---------------------------------------------------------------------------
// Session.Close terminates cleanly
// ---------------------------------------------------------------------------

func TestSession_Close(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping agent build in short mode")
	}

	agentBin := buildTestAgent(t)
	fakeSSH := createFakeSSH(t, agentBin)

	sess, err := Dial(context.Background(), SessionConfig{
		Host:      "test-host",
		AgentPath: agentBin,
		SSHBinary: fakeSSH,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify alive before close.
	if !sess.Alive() {
		t.Error("should be alive before close")
	}

	err = sess.Close()
	// err may be non-nil (signal: killed) which is expected.
	_ = err

	if sess.Alive() {
		t.Error("should not be alive after close")
	}

	// Double close should not panic.
	sess.Close()
}

// ---------------------------------------------------------------------------
// Connection failure — bad ssh binary
// ---------------------------------------------------------------------------

func TestSession_DialBadSSHBinary(t *testing.T) {
	_, err := Dial(context.Background(), SessionConfig{
		Host:      "test-host",
		SSHBinary: "/nonexistent/ssh",
	})
	if err == nil {
		t.Fatal("expected error for bad ssh binary")
	}

	sshErr, ok := err.(*SSHError)
	if !ok {
		t.Fatalf("expected *SSHError, got %T: %v", err, err)
	}
	if sshErr.Host != "test-host" {
		t.Errorf("host = %q", sshErr.Host)
	}
}

// ---------------------------------------------------------------------------
// Connection failure — process exits immediately (simulating bad host)
// ---------------------------------------------------------------------------

func TestSession_ProcessExitsOnPing(t *testing.T) {
	// Create a fake ssh that exits immediately with an error.
	dir := t.TempDir()
	script := filepath.Join(dir, "fail-ssh")
	content := "#!/bin/sh\necho 'ssh: Could not resolve hostname bad-host' >&2\nexit 255\n"
	os.WriteFile(script, []byte(content), 0o755)

	sess, err := Dial(context.Background(), SessionConfig{
		Host:      "bad-host",
		SSHBinary: script,
	})
	if err != nil {
		t.Fatalf("dial should succeed (process starts): %v", err)
	}
	defer sess.Close()

	// Ping should fail because the process exits.
	pingCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = sess.Ping(pingCtx)
	if err == nil {
		t.Fatal("expected ping error for dying process")
	}
	t.Logf("ping error: %v", err)
}

// ---------------------------------------------------------------------------
// Empty host — error
// ---------------------------------------------------------------------------

func TestSession_DialEmptyHost(t *testing.T) {
	_, err := Dial(context.Background(), SessionConfig{})
	if err == nil {
		t.Fatal("expected error for empty host")
	}
}

// ---------------------------------------------------------------------------
// Defaults
// ---------------------------------------------------------------------------

func TestSession_DefaultConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping agent build in short mode")
	}

	agentBin := buildTestAgent(t)
	fakeSSH := createFakeSSH(t, agentBin)

	sess, err := Dial(context.Background(), SessionConfig{
		Host:      "test-host",
		SSHBinary: fakeSSH,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	// Defaults should have been applied.
	if sess.config.Port != DefaultSSHPort {
		t.Errorf("port = %d, want %d", sess.config.Port, DefaultSSHPort)
	}
	if sess.config.AgentPath != DefaultAgentPath {
		t.Errorf("agent_path = %q, want %q", sess.config.AgentPath, DefaultAgentPath)
	}
}

// ---------------------------------------------------------------------------
// Walk over session
// ---------------------------------------------------------------------------

func TestSession_WalkOverSession(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping agent build in short mode")
	}

	agentBin := buildTestAgent(t)
	fakeSSH := createFakeSSH(t, agentBin)

	// Create a fixture to walk.
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "hello.go"), []byte("package main"), 0o644)
	os.MkdirAll(filepath.Join(root, "sub"), 0o755)
	os.WriteFile(filepath.Join(root, "sub", "lib.go"), []byte("package sub"), 0o644)

	sess, err := Dial(context.Background(), SessionConfig{
		Host:      "test-host",
		AgentPath: agentBin,
		SSHBinary: fakeSSH,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	// Send walk request.
	if err := sess.Enc.Send(Envelope{
		Method: MethodWalk,
		ID:     1,
		Params: WalkParams{Root: root},
	}); err != nil {
		t.Fatal(err)
	}

	// Collect results.
	count := 0
	for {
		raw, err := sess.Dec.Receive()
		if err != nil {
			t.Fatalf("receive: %v", err)
		}
		resp, parseErr := ParseResponse(raw)
		if parseErr != nil {
			t.Fatalf("parse: %v", parseErr)
		}
		if resp.Done {
			break
		}
		count++
	}

	if count != 2 {
		t.Errorf("expected 2 walk entries (hello.go, sub/lib.go), got %d", count)
	}
}

// ---------------------------------------------------------------------------
// StderrOutput
// ---------------------------------------------------------------------------

func TestSession_StderrOutput(t *testing.T) {
	// Create a fake ssh that writes to stderr then exits.
	dir := t.TempDir()
	script := filepath.Join(dir, "stderr-ssh")
	content := "#!/bin/sh\necho 'diagnostic info' >&2\nexit 1\n"
	os.WriteFile(script, []byte(content), 0o755)

	sess, err := Dial(context.Background(), SessionConfig{
		Host:      "test-host",
		SSHBinary: script,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Wait for process to exit.
	<-sess.Done()

	stderr := sess.StderrOutput()
	if stderr != "diagnostic info" {
		t.Errorf("stderr = %q, want %q", stderr, "diagnostic info")
	}

	sess.Close()
}

// ---------------------------------------------------------------------------
// SSHError implements error
// ---------------------------------------------------------------------------

func TestSSHError_Interface(t *testing.T) {
	var err error = &SSHError{Host: "myhost", Message: "connection refused"}
	expected := "ssh myhost: connection refused"
	if err.Error() != expected {
		t.Errorf("error = %q, want %q", err.Error(), expected)
	}
}

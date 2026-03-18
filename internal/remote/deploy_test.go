package remote

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupMockSSH creates a shell script simulating SSH responses for deploy tests.
func setupMockSSH(t *testing.T, deployDir string) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "mock-ssh")

	content := `#!/bin/sh
cmd="$LAST_ARG"

for arg in "$@"; do
	LAST_ARG="$arg"
done
cmd="$LAST_ARG"

if echo "$cmd" | grep -q "uname -s -m"; then
	echo "Linux x86_64"
	exit 0
fi

if echo "$cmd" | grep -q "btw-agent --version"; then
	if [ -f "` + deployDir + `/btw-agent" ]; then
		"` + deployDir + `/btw-agent" --version
		exit $?
	fi
	echo "command not found" >&2
	exit 127
fi

if echo "$cmd" | grep -q "mkdir -p"; then
	# It's the deploy command
	# Extract the path from "cat > <path>"
	dest=$(echo "$cmd" | sed -n 's/.*cat > \([^ ]*\).*/\1/p')
	if [ -z "$dest" ]; then
		# fallback if exact sed fails
		dest="` + deployDir + `/btw-agent"
	fi
	
	mkdir -p "$(dirname "$dest")"
	cat > "$dest"
	chmod +x "$dest"
	exit 0
fi

echo "unknown command: $cmd" >&2
exit 1
`
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

func TestDetectRemote(t *testing.T) {
	mockSSH := setupMockSSH(t, t.TempDir())
	ctx := context.Background()

	cfg := DeployConfig{
		Host:      "test-host",
		SSHBinary: mockSSH,
	}

	osName, archName, err := DetectRemote(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if osName != "linux" {
		t.Errorf("expected linux, got %s", osName)
	}
	if archName != "amd64" {
		t.Errorf("expected amd64, got %s", archName)
	}
}

func TestCheckAgentVersion_NotInstalled(t *testing.T) {
	mockSSH := setupMockSSH(t, t.TempDir())
	ctx := context.Background()

	cfg := DeployConfig{
		Host:      "test-host",
		SSHBinary: mockSSH,
		AgentPath: filepath.Join(t.TempDir(), "btw-agent"), // doesn't exist yet
	}

	ver, err := CheckAgentVersion(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ver != "" {
		t.Errorf("expected empty string for uninstalled agent, got %q", ver)
	}
}

func TestAutoDeploy_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping agent build in short mode")
	}

	// 1. Build a real local agent (dev version)
	localBinDir := t.TempDir()
	expectedBin := filepath.Join(localBinDir, "btw-agent-linux-amd64") // Matches detected mock OS/Arch
	
	// Create a dummy binary that responds to --version
	dummyBinary := `#!/bin/sh
if [ "$1" = "--version" ]; then
	echo "btw-agent version dev"
	exit 0
fi
echo "dummy"
`
	err := os.WriteFile(expectedBin, []byte(dummyBinary), 0o755)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Setup mock SSH
	remoteDir := t.TempDir()
	mockSSH := setupMockSSH(t, remoteDir)
	ctx := context.Background()

	cfg := DeployConfig{
		Host:        "test-host",
		SSHBinary:   mockSSH,
		AgentPath:   filepath.Join(remoteDir, "btw-agent"),
		LocalBinDir: localBinDir,
	}

	// 3. First run should deploy
	deployed, err := AutoDeploy(ctx, cfg, "dev")
	if err != nil {
		t.Fatalf("unexpected error during AutoDeploy: %v", err)
	}
	if !deployed {
		t.Error("expected first AutoDeploy to return true (deployed)")
	}

	// Verify it was copied
	if _, err := os.Stat(cfg.AgentPath); err != nil {
		t.Fatalf("agent was not copied to remote path: %v", err)
	}

	// 4. Second run should skip
	deployed, err = AutoDeploy(ctx, cfg, "dev")
	if err != nil {
		t.Fatalf("unexpected error during second AutoDeploy: %v", err)
	}
	if deployed {
		t.Error("expected second AutoDeploy to return false (up-to-date)")
	}
}

func TestAutoDeploy_ArchitectureMismatch(t *testing.T) {
	// Let the remote be linux/amd64 (from mock ssh), but we only provide a linux/arm64 binary locally.
	localBinDir := t.TempDir()
	err := os.WriteFile(filepath.Join(localBinDir, "btw-agent-linux-arm64"), []byte("dummy binary"), 0o755)
	if err != nil {
		t.Fatal(err)
	}

	remoteDir := t.TempDir()
	mockSSH := setupMockSSH(t, remoteDir)
	ctx := context.Background()

	cfg := DeployConfig{
		Host:        "test-host",
		SSHBinary:   mockSSH,
		AgentPath:   filepath.Join(remoteDir, "btw-agent"),
		LocalBinDir: localBinDir,
	}

	// Should fail because `btw-agent-linux-amd64` is missing locally
	deployed, err := AutoDeploy(ctx, cfg, "")
	if err == nil {
		t.Fatal("expected error due to missing matching binary")
	}
	if deployed {
		t.Error("should not report deployed on error")
	}
	if !strings.Contains(err.Error(), "local agent binary not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

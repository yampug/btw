package remote

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DeployConfig specifies the parameters for agent deployment.
type DeployConfig struct {
	Host        string
	Port        int
	SSHBinary   string // Default "ssh"
	AgentPath   string // Remote path, default DefaultAgentPath
	LocalBinDir string // Directory containing pre-built binaries (btw-agent-linux-amd64, etc.)
}

// DetectRemote detects the OS and architecture of the remote host.
// It runs `uname -s -m` over SSH.
// Returns OS (e.g., "linux", "darwin") and Arch (e.g., "amd64", "arm64").
func DetectRemote(ctx context.Context, cfg DeployConfig) (string, string, error) {
	sshBin := cfg.SSHBinary
	if sshBin == "" {
		sshBin = "ssh"
	}

	args := []string{
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
	}
	if cfg.Port > 0 && cfg.Port != DefaultSSHPort {
		args = append(args, "-p", fmt.Sprint(cfg.Port))
	}
	args = append(args, cfg.Host, "uname -s -m")

	cmd := exec.CommandContext(ctx, sshBin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("ssh uname failed: %v (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	out := strings.TrimSpace(stdout.String())
	parts := strings.Fields(out)
	if len(parts) < 2 {
		return "", "", fmt.Errorf("unexpected uname output: %q", out)
	}

	osName := strings.ToLower(parts[0])
	archRaw := strings.ToLower(parts[1])

	archName := archRaw
	switch archRaw {
	case "x86_64":
		archName = "amd64"
	case "aarch64", "arm64":
		archName = "arm64"
	}

	return osName, archName, nil
}

// CheckAgentVersion checks the version of the agent installed on the remote host.
// If the agent is not installed, it returns an empty string and no error.
func CheckAgentVersion(ctx context.Context, cfg DeployConfig) (string, error) {
	sshBin := cfg.SSHBinary
	if sshBin == "" {
		sshBin = "ssh"
	}
	agentPath := cfg.AgentPath
	if agentPath == "" {
		agentPath = DefaultAgentPath
	}

	args := []string{
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
	}
	if cfg.Port > 0 && cfg.Port != DefaultSSHPort {
		args = append(args, "-p", fmt.Sprint(cfg.Port))
	}
	args = append(args, cfg.Host, agentPath+" --version")

	cmd := exec.CommandContext(ctx, sshBin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// If the command fails because the file doesn't exist, we consider it uninstalled.
		// Usually this exits with 127 "command not found" or 1 if the shell fails to find it.
		// We can just rely on stdout being empty or missing "version".
		return "", nil
	}

	out := strings.TrimSpace(stdout.String())
	// Expected output: "btw-agent version X.Y.Z"
	if strings.HasPrefix(out, "btw-agent version ") {
		return strings.TrimPrefix(out, "btw-agent version "), nil
	}
	return out, nil
}

// DeployAgent uploads the specified binary stream to the remote host.
func DeployAgent(ctx context.Context, cfg DeployConfig, content io.Reader) error {
	sshBin := cfg.SSHBinary
	if sshBin == "" {
		sshBin = "ssh"
	}
	agentPath := cfg.AgentPath
	if agentPath == "" {
		agentPath = DefaultAgentPath
	}

	dir := filepath.Dir(agentPath)

	// Command to ensure directory exists, write stdin to file, and make executable.
	remoteCmd := fmt.Sprintf("mkdir -p %s && cat > %s && chmod +x %s", dir, agentPath, agentPath)

	args := []string{
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
	}
	if cfg.Port > 0 && cfg.Port != DefaultSSHPort {
		args = append(args, "-p", fmt.Sprint(cfg.Port))
	}
	args = append(args, cfg.Host, remoteCmd)

	cmd := exec.CommandContext(ctx, sshBin, args...)
	cmd.Stdin = content
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ssh upload failed: %v (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// AutoDeploy automates the process of checking, detecting OS/Arch, and uploading
// the pre-built agent if it doesn't exist or is outdated.
func AutoDeploy(ctx context.Context, cfg DeployConfig, expectedVersion string) (bool, error) {
	// First check if already installed and correct version
	ver, err := CheckAgentVersion(ctx, cfg)
	if err == nil && ver != "" && (expectedVersion == "" || ver == expectedVersion) {
		return false, nil // Already up-to-date
	}

	osName, archName, err := DetectRemote(ctx, cfg)
	if err != nil {
		return false, fmt.Errorf("detect remote: %w", err)
	}

	if osName != "linux" {
		return false, fmt.Errorf("unsupported remote OS: %s (only linux is supported for pre-built agents)", osName)
	}
	if archName != "amd64" && archName != "arm64" {
		return false, fmt.Errorf("unsupported remote architecture: %s", archName)
	}

	binName := fmt.Sprintf("btw-agent-%s-%s", osName, archName)
	localBinPath := filepath.Join(cfg.LocalBinDir, binName)

	f, err := os.Open(localBinPath)
	if err != nil {
		return false, fmt.Errorf("local agent binary not found: %s", localBinPath)
	}
	defer f.Close()

	if err := DeployAgent(ctx, cfg, f); err != nil {
		return false, fmt.Errorf("deploy agent: %w", err)
	}

	// Verify deployment
	newVer, err := CheckAgentVersion(ctx, cfg)
	if err != nil {
		return true, fmt.Errorf("verify deployment: %w", err)
	}
	if newVer == "" && expectedVersion != "" {
		return true, fmt.Errorf("deployed binary failed to return version")
	}

	return true, nil
}

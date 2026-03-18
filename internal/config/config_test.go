package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Remotes(t *testing.T) {
	configYAML := `
remotes:
  dev-server:
    host: "user@dev-server.internal"
    port: 2222
    agent_path: "/usr/local/bin/btw-agent"
    default_root: "/home/user/projects/myapp"
  staging:
    host: "deploy@staging.example.com"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Remotes) != 2 {
		t.Fatalf("expected 2 remotes, got %d", len(cfg.Remotes))
	}

	dev := cfg.Remotes["dev-server"]
	if dev.Host != "user@dev-server.internal" {
		t.Errorf("dev.Host = %q", dev.Host)
	}
	if dev.Port != 2222 {
		t.Errorf("dev.Port = %d", dev.Port)
	}
	if dev.AgentPath != "/usr/local/bin/btw-agent" {
		t.Errorf("dev.AgentPath = %q", dev.AgentPath)
	}
	if dev.DefaultRoot != "/home/user/projects/myapp" {
		t.Errorf("dev.DefaultRoot = %q", dev.DefaultRoot)
	}

	staging := cfg.Remotes["staging"]
	if staging.Host != "deploy@staging.example.com" {
		t.Errorf("staging.Host = %q", staging.Host)
	}
	if staging.Port != 0 {
		t.Errorf("staging.Port = %d", staging.Port)
	}
}

func TestLoad_RemoteNotFound(t *testing.T) {
	cfg := NewDefaultConfig()
	_, ok := cfg.Remotes["unknown-host"]
	if ok {
		t.Fatal("expected unknown-host to not exist")
	}
}

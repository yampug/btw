package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds user preferences for boomerang.
type Config struct {
	Theme          string            `yaml:"theme"`           // dark | light | auto
	Editor         string            `yaml:"editor"`          // overrides $EDITOR
	MaxResults     int               `yaml:"max_results"`     // max results per tab
	ShowHidden     bool              `yaml:"show_hidden"`     // show hidden files by default
	DefaultScope   string            `yaml:"default_scope"`   // project | all
	IgnorePatterns []string          `yaml:"ignore_patterns"` // additional ignore patterns
	Keybindings    map[string]string `yaml:"keybindings"`     // custom key overrides
	SymbolLanguages []string         `yaml:"symbol_languages"` // languages for symbol extraction
}

// NewDefaultConfig returns a Config with sensible defaults.
func NewDefaultConfig() *Config {
	return &Config{
		Theme:        "auto",
		MaxResults:   200,
		ShowHidden:   false,
		DefaultScope: "project",
		Keybindings:  make(map[string]string),
		SymbolLanguages: []string{"go", "typescript", "python", "rust", "ruby", "java", "kotlin"},
	}
}

// Load loads the configuration from disk.
// It checks the provided path, then ~/.config/boomerang/config.yaml, then ~/.boomerang.yaml.
func Load(customPath string) (*Config, error) {
	cfg := NewDefaultConfig()

	paths := []string{}
	if customPath != "" {
		paths = append(paths, customPath)
	}

	home, _ := os.UserHomeDir()
	if home != "" {
		paths = append(paths, filepath.Join(home, ".config", "boomerang", "config.yaml"))
		paths = append(paths, filepath.Join(home, ".boomerang.yaml"))
	}

	var lastErr error
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				lastErr = err
			}
			continue
		}

		if err := yaml.Unmarshal(data, cfg); err != nil {
			return cfg, err
		}
		return cfg, nil // stop at first found config
	}

	return cfg, lastErr
}

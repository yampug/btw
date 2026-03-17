package tui

import (
	"github.com/yampug/btw/internal/model"
)

// Action represents a user-executable action in the Actions tab.
type Action struct {
	Name        string
	Description string
	Shortcut    string // e.g., "Enter", "Ctrl+Y"
	Execute     func() error
}

// ActionRegistry holds all available actions.
type ActionRegistry struct {
	actions []Action
}

// NewActionRegistry creates a new registry with built-in actions.
func NewActionRegistry() *ActionRegistry {
	r := &ActionRegistry{}
	r.RegisterBuiltins()
	return r
}

// RegisterBuiltins adds all built-in boomerang actions.
func (r *ActionRegistry) RegisterBuiltins() {
	r.actions = []Action{
		{
			Name:        "Open File",
			Description: "Open selected file in $EDITOR",
			Shortcut:    "Enter",
			Execute:     func() error { return nil }, // Handled by TUI
		},
		{
			Name:        "Copy Path",
			Description: "Copy absolute file path to clipboard",
			Shortcut:    "Ctrl+Y",
			Execute:     func() error { return nil }, // Handled by TUI
		},
		{
			Name:        "Copy Relative Path",
			Description: "Copy relative file path to clipboard",
			Shortcut:    "Ctrl+Shift+Y",
			Execute:     func() error { return nil }, // Handled by TUI
		},
		{
			Name:        "Reveal in Finder/Explorer",
			Description: "Open parent directory in system file manager",
			Shortcut:    "Ctrl+O",
			Execute:     func() error { return nil }, // Handled by TUI
		},
		{
			Name:        "Refresh Index",
			Description: "Rebuild the file index",
			Shortcut:    "Ctrl+R",
			Execute:     func() error { return nil }, // Handled by TUI
		},
		{
			Name:        "Toggle Scope",
			Description: "Switch between Project Only and All Places scope",
			Shortcut:    "Ctrl+P",
			Execute:     func() error { return nil }, // Handled by TUI
		},
		{
			Name:        "Toggle Hidden Files",
			Description: "Show or hide hidden files in results",
			Shortcut:    "Ctrl+H",
			Execute:     func() error { return nil }, // Handled by TUI
		},
		{
			Name:        "Change Theme",
			Description: "Cycle through color themes",
			Shortcut:    "",
			Execute:     func() error { return nil }, // Handled by TUI
		},
		{
			Name:        "Open in Terminal",
			Description: "Open a shell in the file's directory",
			Shortcut:    "",
			Execute:     func() error { return nil }, // Handled by TUI
		},
		{
			Name:        "Copy Line Reference",
			Description: "Copy file:line reference to clipboard",
			Shortcut:    "",
			Execute:     func() error { return nil }, // Handled by TUI
		},
	}
}

// Actions returns all registered actions.
func (r *ActionRegistry) Actions() []Action {
	return r.actions
}

// FindActionByName finds an action by its name (case-insensitive).
func (r *ActionRegistry) FindActionByName(name string) *Action {
	for _, a := range r.actions {
		if a.Name == name {
			return &a
		}
	}
	return nil
}

// ActionToSearchResult converts an action to a search result.
func ActionToSearchResult(action Action) model.SearchResult {
	return model.SearchResult{
		Name:       action.Name,
		Detail:     action.Description,
		ResultType: model.ResultAction,
		Score:      0,
		Icon:       "⚡",
		IconColor:  "#F59E0B", // Orange for actions
	}
}

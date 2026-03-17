package tui

import (
	"testing"

	"github.com/yampug/btw/internal/model"
)

func TestActionRegistry(t *testing.T) {
	r := NewActionRegistry()
	actions := r.Actions()

	if len(actions) == 0 {
		t.Fatal("expected at least one action")
	}

	// Check that all built-in actions are present
	expectedActions := []string{
		"Open File",
		"Copy Path", 
		"Copy Relative Path",
		"Reveal in Finder/Explorer",
		"Refresh Index",
		"Toggle Scope",
		"Toggle Hidden Files",
		"Change Theme",
		"Open in Terminal",
		"Copy Line Reference",
	}

	foundCount := 0
	for _, expected := range expectedActions {
		found := false
		for _, action := range actions {
			if action.Name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected action '%s' not found", expected)
		} else {
			foundCount++
		}
	}

	if foundCount != len(expectedActions) {
		t.Errorf("expected %d actions, found %d", len(expectedActions), foundCount)
	}
}

func TestActionToSearchResult(t *testing.T) {
	action := Action{
		Name:        "Test Action",
		Description: "Test description",
		Shortcut:    "Ctrl+T",
	}

	result := ActionToSearchResult(action)

	if result.Name != "Test Action" {
		t.Errorf("expected Name 'Test Action', got '%s'", result.Name)
	}
	if result.Detail != "Test description" {
		t.Errorf("expected Detail 'Test description', got '%s'", result.Detail)
	}
	if result.ResultType != model.ResultAction {
		t.Errorf("expected ResultType ResultAction, got %v", result.ResultType)
	}
	if result.Icon != "⚡" {
		t.Errorf("expected Icon '⚡', got '%s'", result.Icon)
	}
	if result.IconColor != "#F59E0B" {
		t.Errorf("expected IconColor '#F59E0B', got '%s'", result.IconColor)
	}
}

func TestFindActionByName(t *testing.T) {
	r := NewActionRegistry()

	// Test finding existing action
	action := r.FindActionByName("Open File")
	if action == nil {
		t.Fatal("expected to find 'Open File' action")
	}
	if action.Name != "Open File" {
		t.Errorf("expected Name 'Open File', got '%s'", action.Name)
	}

	// Test finding non-existent action
	none := r.FindActionByName("Non Existent")
	if none != nil {
		t.Errorf("expected nil for non-existent action, got %v", none)
	}
}

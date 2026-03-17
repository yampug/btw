package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ScopeChangedMsg is emitted when the scope toggle changes.
type ScopeChangedMsg struct {
	ProjectOnly bool
}

// StatusBar renders the footer with a scope toggle and result count.
type StatusBar struct {
	projectOnly bool
	selected    int
	total       int
	width       int
}

// NewStatusBar returns an initialized StatusBar defaulting to project-only scope.
func NewStatusBar() StatusBar {
	return StatusBar{projectOnly: true}
}

// SetWidth sets the available rendering width.
func (s *StatusBar) SetWidth(w int) {
	s.width = w
}

// SetCounts updates the displayed result counts.
func (s *StatusBar) SetCounts(selected, total int) {
	s.selected = selected
	s.total = total
}

// ProjectOnly reports the current scope.
func (s StatusBar) ProjectOnly() bool {
	return s.projectOnly
}

func (s StatusBar) Update(msg tea.Msg) (StatusBar, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok && km.String() == "ctrl+p" {
		s.projectOnly = !s.projectOnly
		return s, func() tea.Msg {
			return ScopeChangedMsg{ProjectOnly: s.projectOnly}
		}
	}
	return s, nil
}

// View renders the status bar.
func (s StatusBar) View() string {
	// Scope toggle.
	check := "  "
	label := "All Places"
	if s.projectOnly {
		check = "✓"
		label = "Project Only"
	}

	toggleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}).
		Bold(s.projectOnly)

	toggle := toggleStyle.Render(fmt.Sprintf(" [%s %s]", check, label))

	// Result count.
	countText := fmt.Sprintf("%d/%d results", s.selected, s.total)
	countStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#888888"})
	count := countStyle.Render(countText)

	// Fill gap between left and right.
	gap := s.width - lipgloss.Width(toggle) - lipgloss.Width(count)
	if gap < 1 {
		gap = 1
	}
	filler := lipgloss.NewStyle().Render(repeatChar(" ", gap))

	return toggle + filler + count
}

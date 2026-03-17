package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ScopeChangedMsg is emitted when the scope toggle changes.
type ScopeChangedMsg struct {
	ProjectOnly bool
}

// ClearMessageMsg is emitted after a delay to clear the status bar message.
type ClearMessageMsg struct {
	ID int
}

// StatusBar renders the footer with a scope toggle and result count.
type StatusBar struct {
	projectOnly bool
	selected    int
	total       int
	width       int
	message     string
	messageID   int // to avoid clearing a newer message
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

// SetMessage sets a temporary status message that clears after a delay.
func (s *StatusBar) SetMessage(msg string, d time.Duration) tea.Cmd {
	s.message = msg
	s.messageID++
	id := s.messageID
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return ClearMessageMsg{ID: id}
	})
}

// ProjectOnly reports the current scope.
func (s StatusBar) ProjectOnly() bool {
	return s.projectOnly
}

func (s StatusBar) Update(msg tea.Msg) (StatusBar, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+p" {
			s.projectOnly = !s.projectOnly
			return s, func() tea.Msg {
				return ScopeChangedMsg{ProjectOnly: s.projectOnly}
			}
		}
	case ClearMessageMsg:
		if msg.ID == s.messageID {
			s.message = ""
		}
	}
	return s, nil
}

// View renders the status bar.
func (s StatusBar) View() string {
	// Left side: Scope toggle.
	check := "  "
	label := "All Places"
	if s.projectOnly {
		check = "✓"
		label = "Project Only"
	}

	toggleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}).
		Bold(s.projectOnly)

	left := toggleStyle.Render(fmt.Sprintf(" [%s %s]", check, label))

	// Center: Message (if any).
	message := ""
	if s.message != "" {
		messageStyle := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFFFF"}).
			Background(lipgloss.AdaptiveColor{Light: "#28A745", Dark: "#218838"}).
			Padding(0, 1)
		message = " " + messageStyle.Render(s.message)
	}

	// Right side: Result count.
	countText := fmt.Sprintf("%d/%d results", s.selected, s.total)
	countStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#888888"})
	right := countStyle.Render(countText)

	// Fill gap between parts.
	gap := s.width - lipgloss.Width(left) - lipgloss.Width(message) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	filler := repeatChar(" ", gap)

	return left + message + filler + right
}

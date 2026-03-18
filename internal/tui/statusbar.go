package tui

import (
	"fmt"
	"os"
	"strings"
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
	theme       Theme
	root        string // formatted project root
	remoteHost  string // non-empty if connected to a remote session
	rtt         time.Duration
}

// NewStatusBar returns an initialized StatusBar with the given theme.
func NewStatusBar(theme Theme) StatusBar {
	return StatusBar{
		projectOnly: true,
		theme:       theme,
	}
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

// SetRemoteHost sets the remote host name to display the link indicator.
func (s *StatusBar) SetRemoteHost(host string) {
	s.remoteHost = host
}

// SetRTT sets the round trip time latency for remote mode.
func (s *StatusBar) SetRTT(rtt time.Duration) {
	s.rtt = rtt
}

// SetRoot sets and formats the project root path.
func (s *StatusBar) SetRoot(root string) {
	if s.remoteHost == "" {
		home, err := os.UserHomeDir()
		if err == nil && strings.HasPrefix(root, home) {
			root = "~" + strings.TrimPrefix(root, home)
		}
	}
	s.root = root
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

// SetProjectOnly sets the scope toggle.
func (s *StatusBar) SetProjectOnly(v bool) {
	s.projectOnly = v
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
	baseStyle := lipgloss.NewStyle().
		Background(s.theme.StatusBg).
		Foreground(s.theme.StatusFg).
		Width(s.width)

	// Left side: Scope toggle.
	check := "  "
	label := "All Places"
	if s.projectOnly {
		check = "✓"
		label = "Project Only"
	}

	toggleStyle := lipgloss.NewStyle().
		Foreground(s.theme.InputIcon). // use accent/icon color
		Bold(s.projectOnly)

	left := toggleStyle.Render(fmt.Sprintf(" [%s %s]", check, label))

	// Center: Message (if any).
	message := ""
	if s.message != "" {
		messageStyle := lipgloss.NewStyle().
			Foreground(s.theme.TabActive).
			Background(s.theme.StatusMessage).
			Padding(0, 1)
		message = " " + messageStyle.Render(s.message)
	}

	// Right side: Root + Result count.
	countText := fmt.Sprintf("%d/%d results", s.selected, s.total)
	
	pathText := s.root
	if s.remoteHost != "" {
		// Use accent color for remote host to visually differentiate
		remoteStyle := lipgloss.NewStyle().Foreground(s.theme.InputIcon).Bold(true)
		pathStyle := lipgloss.NewStyle().Foreground(s.theme.DimForeground)
		
		rttIndicator := ""
		if s.rtt > 0 {
			rttIndicator = fmt.Sprintf(" (~%dms)", s.rtt.Milliseconds())
		}
		
		pathText = fmt.Sprintf("🔗 %s%s  %s", remoteStyle.Render(s.remoteHost), pathStyle.Render(rttIndicator), pathStyle.Render(s.root))
	} else if pathText != "" {
		pathText = lipgloss.NewStyle().Foreground(s.theme.DimForeground).Render(pathText)
	}

	rightContent := pathText
	if rightContent != "" {
		rightContent += "  ·  "
	}
	rightContent += lipgloss.NewStyle().Foreground(s.theme.DimForeground).Render(countText) + " "
	
	right := rightContent

	// Fill gap between parts.
	gap := s.width - lipgloss.Width(left) - lipgloss.Width(message) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	filler := repeatChar(" ", gap)

	return baseStyle.Render(left + message + filler + right)
}

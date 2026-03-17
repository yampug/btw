package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Adaptive colors that work in both dark and light terminal themes.
var (
	titleColor  = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	borderColor = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	bgColor     = lipgloss.AdaptiveColor{Light: "#F5F5F5", Dark: "#1A1A2E"}
	fgColor     = lipgloss.AdaptiveColor{Light: "#1A1A2E", Dark: "#EEEEEE"}
)

// Model is the top-level Bubble Tea model for Boomerang.
type Model struct {
	width  int
	height int
}

// New returns an initialized Model.
func New() Model {
	return Model{}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(titleColor)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Background(bgColor).
		Foreground(fgColor).
		Padding(1, 3).
		Align(lipgloss.Center)

	title := titleStyle.Render("Boomerang")
	box := boxStyle.Render(fmt.Sprintf("%s\n\nPress q or Esc to quit", title))

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

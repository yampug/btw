package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Adaptive colors that work in both dark and light terminal themes.
var (
	borderColor = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
)

// Model is the top-level Bubble Tea model for Boomerang.
type Model struct {
	width  int
	height int
	tabBar TabBar
}

// New returns an initialized Model.
func New() Model {
	return Model{
		tabBar: NewTabBar(),
	}
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
		m.tabBar.SetWidth(m.width)
	case TabChangedMsg:
		// Will be used to re-trigger search in later stories.
	}

	var cmd tea.Cmd
	m.tabBar, cmd = m.tabBar.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	tabBarView := m.tabBar.View()

	containerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(m.width - 2).
		Height(m.height - 2)

	content := tabBarView

	return containerStyle.Render(content)
}

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
	width      int
	height     int
	tabBar     TabBar
	input      SearchInput
	resultList ResultList
	statusBar  StatusBar
}

// New returns an initialized Model.
func New() Model {
	return Model{
		tabBar:     NewTabBar(),
		input:      NewSearchInput(),
		resultList: NewResultList(),
		statusBar:  NewStatusBar(),
	}
}

func (m Model) Init() tea.Cmd {
	return m.input.Focus()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.tabBar.SetWidth(m.width)
		m.input.SetWidth(m.width - 2) // account for border
		m.statusBar.SetWidth(m.width - 2)
		// 2 border + 1 tabbar + 1 divider + 1 input + 1 divider + 1 statusbar = 7 lines overhead
		listHeight := m.height - 7
		if listHeight < 1 {
			listHeight = 1
		}
		m.resultList.SetSize(m.width-2, listHeight)
	case TabChangedMsg:
		// Will be used to re-trigger search in later stories.
	case QueryChangedMsg:
		// Will be used to trigger search in later stories.
	case ScopeChangedMsg:
		// Will be used to re-trigger search in later stories.
	}

	var cmd tea.Cmd

	// Only forward non-number keys to the tab bar when input is focused,
	// so typing numbers goes into the search field.
	if isTabKey(msg) {
		m.tabBar, cmd = m.tabBar.Update(msg)
		cmds = append(cmds, cmd)
	}

	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)

	m.resultList, cmd = m.resultList.Update(msg)
	cmds = append(cmds, cmd)

	m.statusBar, cmd = m.statusBar.Update(msg)
	cmds = append(cmds, cmd)

	// Keep status bar counts in sync.
	m.statusBar.SetCounts(m.resultList.Cursor()+1, m.resultList.Len())

	return m, tea.Batch(cmds...)
}

// isTabKey returns true if the message is a key that should go to the tab bar.
func isTabKey(msg tea.Msg) bool {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return true // non-key messages go everywhere
	}
	switch km.String() {
	case "tab", "shift+tab":
		return true
	}
	return false
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	tabBarView := m.tabBar.View()
	inputView := m.input.View()

	divider := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#CCCCCC", Dark: "#444444"}).
		Render(repeatChar("─", m.width-4))

	containerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(m.width - 2).
		Height(m.height - 2)

	divider2 := divider
	resultView := m.resultList.View()

	statusView := m.statusBar.View()

	content := tabBarView + "\n" + divider + "\n" + inputView + "\n" + divider2 + "\n" + resultView + "\n" + statusView

	return containerStyle.Render(content)
}

func repeatChar(ch string, n int) string {
	if n < 0 {
		n = 0
	}
	out := make([]byte, 0, n*len(ch))
	for i := 0; i < n; i++ {
		out = append(out, ch...)
	}
	return string(out)
}

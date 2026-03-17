package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bob/boomerang/internal/model"
)

const tabCount = 6

// TabChangedMsg is emitted when the active tab changes.
type TabChangedMsg struct {
	Tab model.Tab
}

// TabBar is a Bubble Tea component that renders the horizontal tab strip.
type TabBar struct {
	active model.Tab
	width  int
}

// NewTabBar returns an initialized TabBar with TabAll selected.
func NewTabBar() TabBar {
	return TabBar{active: model.TabAll}
}

// Active returns the currently selected tab.
func (t TabBar) Active() model.Tab {
	return t.active
}

// SetWidth sets the available width for rendering.
func (t *TabBar) SetWidth(w int) {
	t.width = w
}

// SetActive sets the active tab.
func (t *TabBar) SetActive(tab model.Tab) {
	t.active = tab
}

func (t TabBar) Update(msg tea.Msg) (TabBar, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			t.active = model.Tab((int(t.active) + 1) % tabCount)
			return t, t.changed()
		case "shift+tab":
			t.active = model.Tab((int(t.active) + tabCount - 1) % tabCount)
			return t, t.changed()
		case "1":
			return t.selectTab(model.TabAll)
		case "2":
			return t.selectTab(model.TabClasses)
		case "3":
			return t.selectTab(model.TabFiles)
		case "4":
			return t.selectTab(model.TabSymbols)
		case "5":
			return t.selectTab(model.TabActions)
		case "6":
			return t.selectTab(model.TabText)
		}
	}
	return t, nil
}

func (t *TabBar) selectTab(tab model.Tab) (TabBar, tea.Cmd) {
	if t.active == tab {
		return *t, nil
	}
	t.active = tab
	return *t, t.changed()
}

func (t TabBar) changed() tea.Cmd {
	return func() tea.Msg {
		return TabChangedMsg{Tab: t.active}
	}
}

// View renders the tab bar.
func (t TabBar) View() string {
	tabs := []string{
		model.TabAll.String(),
		model.TabClasses.String(),
		model.TabFiles.String(),
		model.TabSymbols.String(),
		model.TabActions.String(),
		model.TabText.String(),
	}

	activeStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFFFF"}).
		Background(lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}).
		Padding(0, 1)

	inactiveStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#888888"}).
		Padding(0, 1)

	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#555555"})

	sep := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#CCCCCC", Dark: "#444444"}).
		Render("│")

	var rendered []string
	for i, label := range tabs {
		hint := hintStyle.Render(strings.Repeat(" ", 0) + string(rune('1'+i)))
		if model.Tab(i) == t.active {
			rendered = append(rendered, activeStyle.Render(hint+" "+label))
		} else {
			rendered = append(rendered, inactiveStyle.Render(hint+" "+label))
		}
	}

	bar := strings.Join(rendered, sep)

	// Truncate if wider than terminal
	if t.width > 0 {
		barStyle := lipgloss.NewStyle().MaxWidth(t.width)
		bar = barStyle.Render(bar)
	}

	return bar
}

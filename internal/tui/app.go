package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bob/boomerang/internal/model"
)

// Adaptive colors that work in both dark and light terminal themes.
var (
	borderColor = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	dividerFg   = lipgloss.AdaptiveColor{Light: "#CCCCCC", Dark: "#444444"}
)

// ResultsMsg delivers search results to the TUI.
type ResultsMsg struct {
	Items []model.SearchResult
}

// OpenResultMsg is emitted when the user presses Enter on a selected result.
type OpenResultMsg struct {
	Result model.SearchResult
}

// App is the top-level Bubble Tea model composing all TUI components.
type App struct {
	width      int
	height     int
	tabBar     TabBar
	input      SearchInput
	resultList ResultList
	statusBar  StatusBar
	chosen     *model.SearchResult // set when user presses Enter
}

// NewApp returns an initialized App.
func NewApp() App {
	return App{
		tabBar:     NewTabBar(),
		input:      NewSearchInput(),
		resultList: NewResultList(),
		statusBar:  NewStatusBar(),
	}
}

// Chosen returns the result selected by the user, or nil if they quit without selecting.
func (a App) Chosen() *model.SearchResult {
	return a.chosen
}

func (a App) Init() tea.Cmd {
	return a.input.Focus()
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			return a, tea.Quit
		case "enter":
			if r, ok := a.resultList.Selected(); ok {
				a.chosen = &r
				return a, tea.Quit
			}
			return a, nil
		}
	case tea.WindowSizeMsg:
		a.layout(msg.Width, msg.Height)
	case ResultsMsg:
		a.resultList.SetItems(msg.Items)
	case TabChangedMsg:
		a.resultList.SetLoading(true)
		cmds = append(cmds, a.resultList.SpinnerTick(), a.triggerSearch())
	case QueryChangedMsg:
		a.resultList.SetLoading(true)
		cmds = append(cmds, a.resultList.SpinnerTick(), a.triggerSearch())
	case ScopeChangedMsg:
		a.resultList.SetLoading(true)
		cmds = append(cmds, a.resultList.SpinnerTick(), a.triggerSearch())
	}

	var cmd tea.Cmd

	if isTabKey(msg) {
		a.tabBar, cmd = a.tabBar.Update(msg)
		cmds = append(cmds, cmd)
	}

	a.input, cmd = a.input.Update(msg)
	cmds = append(cmds, cmd)

	a.resultList, cmd = a.resultList.Update(msg)
	cmds = append(cmds, cmd)

	a.statusBar, cmd = a.statusBar.Update(msg)
	cmds = append(cmds, cmd)

	// Sync status bar counts.
	total := a.resultList.Len()
	sel := 0
	if total > 0 {
		sel = a.resultList.Cursor() + 1
	}
	a.statusBar.SetCounts(sel, total)

	return a, tea.Batch(cmds...)
}

// layout recalculates component sizes on terminal resize.
func (a *App) layout(w, h int) {
	a.width = w
	a.height = h
	innerW := w - 2 // border left + right
	a.tabBar.SetWidth(innerW)
	a.input.SetWidth(innerW)
	a.statusBar.SetWidth(innerW)
	// overhead: 2 border + 1 tabbar + 1 divider + 1 input + 1 divider + 1 statusbar = 7
	listH := h - 7
	if listH < 1 {
		listH = 1
	}
	a.resultList.SetSize(innerW, listH)
}

// triggerSearch returns a cmd that will produce a ResultsMsg.
// This is a placeholder until the search engine is wired in Epic 3.
func (a App) triggerSearch() tea.Cmd {
	return func() tea.Msg {
		// Placeholder: return empty results. The real search engine will replace this.
		return ResultsMsg{Items: nil}
	}
}

func (a App) View() string {
	if a.width == 0 || a.height == 0 {
		return ""
	}

	divider := lipgloss.NewStyle().
		Foreground(dividerFg).
		Render(repeatChar("─", a.width-4))

	container := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(a.width - 2).
		Height(a.height - 2)

	content := a.tabBar.View() + "\n" +
		divider + "\n" +
		a.input.View() + "\n" +
		divider + "\n" +
		a.resultList.View() + "\n" +
		a.statusBar.View()

	return container.Render(content)
}

// isTabKey returns true if the message is a key that should route to the tab bar.
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

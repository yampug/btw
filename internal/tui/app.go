package tui

import (
	"context"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bob/boomerang/internal/model"
	"github.com/bob/boomerang/internal/search"
)

// Adaptive colors that work in both dark and light terminal themes.
var (
	borderColor = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	dividerFg   = lipgloss.AdaptiveColor{Light: "#CCCCCC", Dark: "#444444"}
)

// ResultsMsg delivers search results to the TUI.
type ResultsMsg struct {
	Items        []model.SearchResult
	TotalMatched int
}

// OpenResultMsg is emitted when the user presses Enter on a selected result.
type OpenResultMsg struct {
	Result model.SearchResult
}

// searchCanceler provides safe cancellation of in-flight searches.
// It is a pointer-based shared state that survives Bubble Tea's value copies.
type searchCanceler struct {
	mu     sync.Mutex
	cancel context.CancelFunc
}

func (sc *searchCanceler) Cancel() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.cancel != nil {
		sc.cancel()
		sc.cancel = nil
	}
}

func (sc *searchCanceler) Set(cancel context.CancelFunc) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.cancel != nil {
		sc.cancel()
	}
	sc.cancel = cancel
}

// App is the top-level Bubble Tea model composing all TUI components.
type App struct {
	width        int
	height       int
	tabBar       TabBar
	input        SearchInput
	resultList   ResultList
	statusBar    StatusBar
	filterMenu   FilterMenu
	index        *search.Index
	searchCancel *searchCanceler
	chosen       *model.SearchResult // set when user presses Enter
}

// NewApp returns an initialized App with the given file index.
func NewApp(idx *search.Index) App {
	fm := NewFilterMenu()
	if idx != nil {
		fm.SetExtensions(idx.Extensions())
	}
	return App{
		tabBar:       NewTabBar(),
		input:        NewSearchInput(),
		resultList:   NewResultList(),
		statusBar:    NewStatusBar(),
		filterMenu:   fm,
		index:        idx,
		searchCancel: &searchCanceler{},
	}
}

// Chosen returns the result selected by the user, or nil if they quit without selecting.
func (a App) Chosen() *model.SearchResult {
	return a.chosen
}

func (a App) Init() tea.Cmd {
	return tea.Batch(a.input.Focus(), a.triggerSearch())
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+f":
			a.filterMenu.Toggle()
			if a.filterMenu.Visible() {
				a.filterMenu.SetSize(a.width-4, a.height-8)
			}
			return a, nil
		case "esc":
			if a.filterMenu.Visible() {
				a.filterMenu.Toggle()
				return a, nil
			}
			return a, tea.Quit
		case "ctrl+c":
			return a, tea.Quit
		case "enter":
			if a.filterMenu.Visible() {
				// Let filter menu handle it.
				break
			}
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
		a.resultList.SetTotalMatched(msg.TotalMatched)
	case TabChangedMsg:
		a.resultList.SetLoading(true)
		cmds = append(cmds, a.resultList.SpinnerTick(), a.triggerSearch())
	case QueryChangedMsg:
		a.resultList.SetLoading(true)
		cmds = append(cmds, a.resultList.SpinnerTick(), a.triggerSearch())
	case ScopeChangedMsg:
		a.resultList.SetLoading(true)
		cmds = append(cmds, a.resultList.SpinnerTick(), a.triggerSearch())
	case FilterChangedMsg:
		badge := a.filterMenu.BadgeText()
		a.input.SetFilter(badge)
		a.resultList.SetLoading(true)
		cmds = append(cmds, a.resultList.SpinnerTick(), a.triggerSearch())
	}

	var cmd tea.Cmd

	// When filter menu is visible, route keys there instead of other components.
	if a.filterMenu.Visible() {
		a.filterMenu, cmd = a.filterMenu.Update(msg)
		cmds = append(cmds, cmd)
	} else {
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
	}

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

// triggerSearch returns a cmd that queries the index and produces a ResultsMsg.
func (a App) triggerSearch() tea.Cmd {
	a.searchCancel.Cancel()

	if a.index == nil {
		return func() tea.Msg { return ResultsMsg{} }
	}

	tab := a.tabBar.Active()
	switch tab {
	case model.TabText:
		return a.triggerGrepSearch()
	case model.TabSymbols:
		return a.triggerSymbolSearch()
	default:
		return a.triggerFileSearch()
	}
}

func (a App) triggerFileSearch() tea.Cmd {
	idx := a.index
	query := a.input.Value()
	tab := a.tabBar.Active()
	extFilters := a.filterMenu.SelectedExtensions()
	includeHidden := !a.statusBar.ProjectOnly()

	return func() tea.Msg {
		rs := idx.Search(search.SearchOptions{
			Query:         query,
			Tab:           tab,
			ExtFilters:    extFilters,
			MaxResults:    100,
			IncludeHidden: includeHidden,
		})
		return ResultsMsg{Items: rs.Items, TotalMatched: rs.TotalMatched}
	}
}

func (a App) triggerSymbolSearch() tea.Cmd {
	idx := a.index
	query := a.input.Value()
	includeHidden := !a.statusBar.ProjectOnly()

	return func() tea.Msg {
		rs := idx.SearchSymbols(query, 100, includeHidden)
		return ResultsMsg{Items: rs.Items, TotalMatched: rs.TotalMatched}
	}
}

func (a App) triggerGrepSearch() tea.Cmd {
	idx := a.index
	query := a.input.Value()
	includeHidden := !a.statusBar.ProjectOnly()
	sc := a.searchCancel

	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		sc.Set(cancel)

		ch := search.Grep(ctx, idx, query, search.GrepOptions{
			IncludeHidden: includeHidden,
			MaxResults:    200,
		})

		var results []model.SearchResult
		for m := range ch {
			results = append(results, search.GrepMatchToResult(m))
		}

		return ResultsMsg{Items: results, TotalMatched: len(results)}
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

	rendered := container.Render(content)

	// Overlay filter menu if visible.
	if a.filterMenu.Visible() {
		overlay := a.filterMenu.View()
		rendered = lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, overlay)
	}

	return rendered
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

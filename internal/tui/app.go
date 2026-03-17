package tui

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/atotto/clipboard"
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
	width          int
	height         int
	tabBar         TabBar
	input          SearchInput
	resultList     ResultList
	statusBar      StatusBar
	filterMenu     FilterMenu
	index          *search.Index
	searchCancel   *searchCanceler
	chosen         *model.SearchResult // set when user presses Enter
	actionRegistry *ActionRegistry
	showHidden     bool
}

// NewApp returns an initialized App with the given file index.
func NewApp(idx *search.Index) App {
	fm := NewFilterMenu()
	if idx != nil {
		fm.SetExtensions(idx.Extensions())
	}
	return App{
		tabBar:         NewTabBar(),
		input:          NewSearchInput(),
		resultList:     NewResultList(),
		statusBar:      NewStatusBar(),
		filterMenu:     fm,
		index:          idx,
		searchCancel:   &searchCanceler{},
		actionRegistry: NewActionRegistry(),
		showHidden:     false,
	}
}

// Chosen returns the result selected by the user, or nil if they quit without selecting.
func (a App) Chosen() *model.SearchResult {
	return a.chosen
}

// LineNum returns the line number parsed from the input, if any.
func (a App) LineNum() int {
	return a.input.LineNum()
}

func (a App) Init() tea.Cmd {
	return tea.Batch(a.input.Focus(), a.triggerSearch())
}

func isNavigationKey(msg tea.Msg) bool {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return false
	}
	switch km.String() {
	case "up", "down", "left", "right", "pgup", "pgdown", "home", "end", "ctrl+n", "ctrl+p", "ctrl+up", "ctrl+down":
		return true
	}
	return false
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case IndexUpdatedMsg:
		if a.index != nil {
			a.filterMenu.SetExtensions(a.index.Extensions())
		}
		a.resultList.SetLoading(true)
		cmds = append(cmds, a.resultList.SpinnerTick(), a.triggerSearch())
		cmds = append(cmds, a.statusBar.SetMessage("Index refreshed", 2*time.Second))
	case tea.KeyMsg:
		// Global keys and shortcuts.
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
		case "ctrl+r":
			// Refresh index.
			cmds = append(cmds, a.statusBar.SetMessage("Refreshing index...", 2*time.Second))
			cmds = append(cmds, a.refreshIndex())
			return a, tea.Batch(cmds...)
		case "ctrl+h":
			// Toggle hidden files.
			a.showHidden = !a.showHidden
			status := "Hidden files: ON"
			if !a.showHidden {
				status = "Hidden files: OFF"
			}
			cmds = append(cmds, a.statusBar.SetMessage(status, 2*time.Second))
			cmds = append(cmds, a.triggerSearch())
			return a, tea.Batch(cmds...)
		case "?":
			if a.input.Value() == "" {
				// Show help.
				cmds = append(cmds, a.statusBar.SetMessage("Help overlay coming soon...", 2*time.Second))
				return a, tea.Batch(cmds...)
			}
		case "1", "2", "3", "4", "5", "6":
			if a.input.Value() == "" {
				// Only switch tabs via numbers if input is empty.
				var cmd tea.Cmd
				a.tabBar, cmd = a.tabBar.Update(msg)
				return a, cmd
			}
		case "ctrl+y":
			if r, ok := a.resultList.Selected(); ok && r.FilePath != "" {
				if err := clipboard.WriteAll(r.FilePath); err == nil {
					cmds = append(cmds, a.statusBar.SetMessage("Copied absolute path!", 2*time.Second))
				}
			}
			return a, tea.Batch(cmds...)
		case "ctrl+shift+y", "ctrl+Y": // Bubble Tea often sends Ctrl+Y for Ctrl+Shift+Y
			if r, ok := a.resultList.Selected(); ok && r.FilePath != "" {
				rel := r.FilePath
				if a.index != nil {
					if rPath, err := filepath.Rel(a.index.Root(), r.FilePath); err == nil {
						rel = rPath
					}
				}
				if err := clipboard.WriteAll(rel); err == nil {
					cmds = append(cmds, a.statusBar.SetMessage("Copied relative path!", 2*time.Second))
				}
			}
			return a, tea.Batch(cmds...)
		case "ctrl+o":
			if r, ok := a.resultList.Selected(); ok && r.FilePath != "" {
				if err := revealInFileManager(r.FilePath); err == nil {
					cmds = append(cmds, a.statusBar.SetMessage("Revealed in file manager", 2*time.Second))
				}
			}
			return a, tea.Batch(cmds...)
		case "enter":
			if a.filterMenu.Visible() {
				// Let filter menu handle it.
				break
			}
			if r, ok := a.resultList.Selected(); ok {
				// Special case: if it's a "more..." or header, it might switch tabs.
				// ResultList.Update handles emitting TabChangedMsg.
				// But we also need to allow opening real results.
				if !r.IsHeader && !strings.HasPrefix(r.Name, " … more") {
					a.chosen = &r
					return a, tea.Quit
				}
			}
		}

		// Routing logic for focus behavior in Story 6.1.
		// If it's a printable character and NOT a navigation key, and filter menu is NOT visible,
		// always route it to the input field.
		if !a.filterMenu.Visible() && isPrintable(msg) && !isNavigationKey(msg) {
			var cmd tea.Cmd
			a.input, cmd = a.input.Update(msg)
			cmds = append(cmds, cmd)
			// Also ensure input is focused (though it should already be).
			cmds = append(cmds, a.input.Focus())
		}
	case tea.WindowSizeMsg:
		a.layout(msg.Width, msg.Height)
	case ResultsMsg:
		// Persist cursor if results are just being updated during typing.
		a.resultList.SetItems(msg.Items, false)
		a.resultList.SetTotalMatched(msg.TotalMatched)
	case TabChangedMsg:
		a.tabBar.SetActive(msg.Tab)
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
		// Route messages to components if they haven't been handled by printable char logic.
		// Navigation keys (up/down/etc) go to ResultList.
		// Tab keys go to TabBar.
		
		if isTabKey(msg) {
			a.tabBar, cmd = a.tabBar.Update(msg)
			cmds = append(cmds, cmd)
		}

		// Only update input if it wasn't already updated by printable char logic above.
		if !isPrintable(msg) || isNavigationKey(msg) {
			a.input, cmd = a.input.Update(msg)
			cmds = append(cmds, cmd)
		}

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

// isPrintable returns true if the key message is a printable character.
func isPrintable(msg tea.Msg) bool {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return false
	}
	if km.Type == tea.KeyRunes {
		return true
	}
	if km.String() == "space" {
		return true
	}
	return false
}

// layout recalculates component sizes on terminal resize.
// IndexUpdatedMsg is emitted when the index has been rebuilt.
type IndexUpdatedMsg struct{}

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

func (a App) refreshIndex() tea.Cmd {
	if a.index == nil {
		return nil
	}
	root := a.index.Root()
	return func() tea.Msg {
		rules := search.LoadIgnoreFiles(root)
		a.index.RebuildFrom(context.Background(), root, rules, search.WalkOptions{})
		return IndexUpdatedMsg{}
	}
}

func revealInFileManager(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", "-R", path)
	case "windows":
		cmd = exec.Command("explorer", "/select,", path)
	default: // linux, etc.
		// xdg-open doesn't have a direct "select" or "reveal" flag in many cases,
		// but opening the directory is a common fallback.
		cmd = exec.Command("xdg-open", filepath.Dir(path))
	}
	return cmd.Run()
}

// triggerSearch returns a cmd that queries the index and produces a ResultsMsg.
func (a App) triggerSearch() tea.Cmd {
	a.searchCancel.Cancel()

	if a.index == nil {
		return func() tea.Msg { return ResultsMsg{} }
	}

	tab := a.tabBar.Active()
	switch tab {
	case model.TabAll:
		return a.triggerAllSearch()
	case model.TabText:
		return a.triggerGrepSearch()
	case model.TabSymbols:
		return a.triggerSymbolSearch()
	case model.TabClasses:
		return a.triggerClassSearch()
	case model.TabActions:
		return a.triggerActionSearch()
	default:
		return a.triggerFileSearch()
	}
}

func (a App) triggerAllSearch() tea.Cmd {
	query := a.input.Value()
	includeHidden := a.showHidden || !a.statusBar.ProjectOnly()
	projectOnly := a.statusBar.ProjectOnly()
	idx := a.index
	extFilters := a.filterMenu.SelectedExtensions()

	return func() tea.Msg {
		var allItems []model.SearchResult

		// Files
		filesRs := idx.Search(search.SearchOptions{
			Query:         query,
			Tab:           model.TabFiles,
			ExtFilters:    extFilters,
			MaxResults:    5,
			IncludeHidden: includeHidden,
			ProjectOnly:   projectOnly,
		})
		if len(filesRs.Items) > 0 {
			allItems = append(allItems, model.SearchResult{
				Name:       "Files",
				IsHeader:   true,
				SectionTab: model.TabFiles,
			})
			allItems = append(allItems, filesRs.Items...)
			if filesRs.TotalMatched > 5 {
				allItems = append(allItems, model.SearchResult{
					Name:       fmt.Sprintf("  … more Files (%d total)", filesRs.TotalMatched),
					SectionTab: model.TabFiles,
				})
			}
		}

		// Classes
		classesRs := idx.SearchClasses(query, 5, includeHidden, projectOnly)
		if len(classesRs.Items) > 0 {
			allItems = append(allItems, model.SearchResult{
				Name:       "Classes",
				IsHeader:   true,
				SectionTab: model.TabClasses,
			})
			allItems = append(allItems, classesRs.Items...)
			if classesRs.TotalMatched > 5 {
				allItems = append(allItems, model.SearchResult{
					Name:       fmt.Sprintf("  … more Classes (%d total)", classesRs.TotalMatched),
					SectionTab: model.TabClasses,
				})
			}
		}

		// Symbols
		symbolsRs := idx.SearchSymbols(query, 5, includeHidden, projectOnly)
		if len(symbolsRs.Items) > 0 {
			allItems = append(allItems, model.SearchResult{
				Name:       "Symbols",
				IsHeader:   true,
				SectionTab: model.TabSymbols,
			})
			allItems = append(allItems, symbolsRs.Items...)
			if symbolsRs.TotalMatched > 5 {
				allItems = append(allItems, model.SearchResult{
					Name:       fmt.Sprintf("  … more Symbols (%d total)", symbolsRs.TotalMatched),
					SectionTab: model.TabSymbols,
				})
			}
		}

		// Actions
		actions := a.searchActions(query)
		if len(actions) > 0 {
			totalActions := len(actions)
			limit := 5
			if limit > totalActions {
				limit = totalActions
			}
			allItems = append(allItems, model.SearchResult{
				Name:       "Actions",
				IsHeader:   true,
				SectionTab: model.TabActions,
			})
			allItems = append(allItems, actions[:limit]...)
			if totalActions > 5 {
				allItems = append(allItems, model.SearchResult{
					Name:       fmt.Sprintf("  … more Actions (%d total)", totalActions),
					SectionTab: model.TabActions,
				})
			}
		}

		return ResultsMsg{Items: allItems, TotalMatched: len(allItems)}
	}
}

func (a App) triggerFileSearch() tea.Cmd {
	idx := a.index
	query := a.input.Value()
	tab := a.tabBar.Active()
	extFilters := a.filterMenu.SelectedExtensions()
	includeHidden := a.showHidden || !a.statusBar.ProjectOnly()
	projectOnly := a.statusBar.ProjectOnly()

	return func() tea.Msg {
		rs := idx.Search(search.SearchOptions{
			Query:         query,
			Tab:           tab,
			ExtFilters:    extFilters,
			MaxResults:    100,
			IncludeHidden: includeHidden,
			ProjectOnly:   projectOnly,
		})
		return ResultsMsg{Items: rs.Items, TotalMatched: rs.TotalMatched}
	}
}

func (a App) triggerSymbolSearch() tea.Cmd {
	idx := a.index
	query := a.input.Value()
	includeHidden := a.showHidden || !a.statusBar.ProjectOnly()
	projectOnly := a.statusBar.ProjectOnly()

	return func() tea.Msg {
		rs := idx.SearchSymbols(query, 100, includeHidden, projectOnly)
		return ResultsMsg{Items: rs.Items, TotalMatched: rs.TotalMatched}
	}
}

func (a App) triggerClassSearch() tea.Cmd {
	idx := a.index
	query := a.input.Value()
	includeHidden := a.showHidden || !a.statusBar.ProjectOnly()
	projectOnly := a.statusBar.ProjectOnly()

	return func() tea.Msg {
		rs := idx.SearchClasses(query, 100, includeHidden, projectOnly)
		return ResultsMsg{Items: rs.Items, TotalMatched: rs.TotalMatched}
	}
}

func (a App) triggerActionSearch() tea.Cmd {
	query := a.input.Value()

	return func() tea.Msg {
		results := a.searchActions(query)
		return ResultsMsg{Items: results, TotalMatched: len(results)}
	}
}

func (a App) searchActions(query string) []model.SearchResult {
	query = strings.TrimSpace(query)
	
	var results []model.SearchResult
	for _, action := range a.actionRegistry.Actions() {
		if query == "" {
			results = append(results, ActionToSearchResult(action))
			continue
		}

		// Fuzzy match against action name and description
		nameMatch := search.FuzzyMatch(query, action.Name)
		if nameMatch.Matched {
			r := ActionToSearchResult(action)
			r.MatchRanges = nameMatch.Ranges
			r.Score = 1000 // High score for name matches
			results = append(results, r)
			continue
		}

		descMatch := search.FuzzyMatch(query, action.Description)
		if descMatch.Matched {
			r := ActionToSearchResult(action)
			r.MatchRanges = descMatch.Ranges
			r.Score = 500 // Lower score for description matches
			results = append(results, r)
		}
	}

	// Sort by score (descending), then by name (ascending)
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return strings.ToLower(results[i].Name) < strings.ToLower(results[j].Name)
	})

	if len(results) > 100 {
		results = results[:100]
	}
	return results
}

func (a App) triggerGrepSearch() tea.Cmd {
	idx := a.index
	query := a.input.Value()
	includeHidden := a.showHidden || !a.statusBar.ProjectOnly()
	projectOnly := a.statusBar.ProjectOnly()
	sc := a.searchCancel

	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		sc.Set(cancel)

		ch := search.Grep(ctx, idx, query, search.GrepOptions{
			IncludeHidden: includeHidden,
			ProjectOnly:   projectOnly,
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

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

	"github.com/bob/boomerang/internal/config"
	"github.com/bob/boomerang/internal/model"
	"github.com/bob/boomerang/internal/search"
)

// SearchID tracks search sessions to avoid stale results.
type SearchID uint64

// ResultsMsg delivers search results to the TUI.
type ResultsMsg struct {
	ID           SearchID
	Items        []model.SearchResult
	TotalMatched int
	Append       bool
	Done         bool
	Ch           chan ResultsMsg
}

// IndexProgressMsg is emitted during indexing to show progress.
type IndexProgressMsg struct {
	Count int
	Done  bool
	Ch    chan IndexProgressMsg
}

// OpenResultMsg is emitted when the user presses Enter on a selected result.
type OpenResultMsg struct {
	Result model.SearchResult
}

// searchCanceler provides safe cancellation of in-flight searches and tracks search IDs.
// It is a pointer-based shared state that survives Bubble Tea's value copies.
type searchCanceler struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	id     SearchID
}

func (sc *searchCanceler) Cancel() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.cancel != nil {
		sc.cancel()
		sc.cancel = nil
	}
}

func (sc *searchCanceler) NextID() SearchID {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.id++
	return sc.id
}

func (sc *searchCanceler) CurrentID() SearchID {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.id
}

func (sc *searchCanceler) Set(id SearchID, cancel context.CancelFunc) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.cancel != nil {
		sc.cancel()
	}
	sc.id = id
	sc.cancel = cancel
}

// InitialState defines the initial UI state passed from CLI flags.
type InitialState struct {
	Query       string
	Tab         model.Tab
	Extensions  []string
	ProjectOnly *bool // nil means use config default
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
	helpOverlay    HelpOverlay
	index          *search.Index
	searchCancel   *searchCanceler
	chosen         *model.SearchResult // set when user presses Enter
	actionRegistry *ActionRegistry
	showHidden     bool
	cfg            *config.Config
	theme          Theme
	history        *config.History
	queryCursor    int // -1 means not navigating history
}

// NewApp returns an initialized App with the given file index and config.
func NewApp(idx *search.Index, cfg *config.Config, init InitialState) App {
	if cfg == nil {
		cfg = config.NewDefaultConfig()
	}

	hist, _ := config.LoadHistory()

	themeName := cfg.Theme
	if themeName == "auto" {
		if lipgloss.HasDarkBackground() {
			themeName = "dark"
		} else {
			themeName = "light"
		}
	}
	theme := GetTheme(themeName)

	fm := NewFilterMenu(theme)
	if idx != nil {
		fm.SetExtensions(idx.Extensions())
	}
	if len(init.Extensions) > 0 {
		fm.SetSelected(init.Extensions)
	}

	sb := NewStatusBar(theme)
	if idx != nil {
		sb.SetRoot(idx.Root())
	}
	if cfg.DefaultScope == "all" {
		sb.SetProjectOnly(false)
	}
	if init.ProjectOnly != nil {
		sb.SetProjectOnly(*init.ProjectOnly)
	}

	tb := NewTabBar(theme)
	tb.SetActive(init.Tab)

	input := NewSearchInput(theme)
	if init.Tab == model.TabText {
		input.SetDebounce(250 * time.Millisecond)
	} else {
		input.SetDebounce(100 * time.Millisecond)
	}
	if init.Query != "" {
		input.SetValue(init.Query)
	}
	
	// Apply filter badge to input if filters are pre-applied.
	if len(init.Extensions) > 0 {
		input.SetFilter(fm.BadgeText())
	}

	return App{
		tabBar:         tb,
		input:          input,
		resultList:     NewResultList(theme),
		statusBar:      sb,
		filterMenu:     fm,
		helpOverlay:    NewHelpOverlay(theme),
		index:          idx,
		searchCancel:   &searchCanceler{},
		actionRegistry: NewActionRegistry(),
		showHidden:     cfg.ShowHidden,
		cfg:            cfg,
		theme:          theme,
		history:        hist,
		queryCursor:    -1,
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
	return tea.Batch(
		a.input.Focus(),
		a.refreshIndex(), // Non-blocking index build
		a.triggerSearch(),
	)
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
	case IndexProgressMsg:
		if msg.Done {
			if a.index != nil {
				a.filterMenu.SetExtensions(a.index.Extensions())
			}
			a.resultList.SetLoading(true)
			cmds = append(cmds, a.resultList.SpinnerTick(), a.triggerSearch())
			cmds = append(cmds, a.statusBar.SetMessage("Index refreshed", 2*time.Second))
		} else {
			status := fmt.Sprintf("Indexing... %d files", msg.Count)
			cmds = append(cmds, a.statusBar.SetMessage(status, 1*time.Second))
			if msg.Ch != nil {
				cmds = append(cmds, a.waitForIndexProgress(msg.Ch))
			}
		}
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
				a.helpOverlay.Toggle()
				return a, nil
			}
		case "1", "2", "3", "4", "5", "6":
			if a.input.Value() == "" {
				// Only switch tabs via numbers if input is empty.
				var cmd tea.Cmd
				a.tabBar, cmd = a.tabBar.Update(msg)
				return a, cmd
			}
		case "ctrl+up":
			if a.history != nil {
				tabName := a.tabBar.Active().String()
				queries := a.history.GetQueries(tabName)
				if len(queries) > 0 {
					a.queryCursor++
					if a.queryCursor >= len(queries) {
						a.queryCursor = len(queries) - 1
					}
					a.input.SetValue(queries[a.queryCursor])
					cmds = append(cmds, a.triggerSearch())
				}
			}
			return a, tea.Batch(cmds...)
		case "ctrl+down":
			if a.history != nil {
				tabName := a.tabBar.Active().String()
				queries := a.history.GetQueries(tabName)
				a.queryCursor--
				if a.queryCursor < -1 {
					a.queryCursor = -1
				}
				if a.queryCursor == -1 {
					a.input.SetValue("")
				} else {
					a.input.SetValue(queries[a.queryCursor])
				}
				cmds = append(cmds, a.triggerSearch())
			}
			return a, tea.Batch(cmds...)
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
					if r.FilePath != "" && a.history != nil {
						a.history.Add(r.FilePath)
						_ = a.history.Save()
					}
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
		if msg.ID != a.searchCancel.CurrentID() {
			return a, nil
		}
		if msg.Append {
			a.resultList.AppendItems(msg.Items)
		} else {
			a.resultList.SetItems(msg.Items, false)
		}
		a.resultList.SetTotalMatched(msg.TotalMatched)
		if !msg.Done && msg.Ch != nil {
			cmds = append(cmds, a.waitForResults(msg.ID, msg.Ch))
		}
	case TabChangedMsg:
		a.tabBar.SetActive(msg.Tab)
		if msg.Tab == model.TabText {
			a.input.SetDebounce(250 * time.Millisecond)
		} else {
			a.input.SetDebounce(100 * time.Millisecond)
		}
		a.queryCursor = -1
		a.resultList.SetLoading(true)
		cmds = append(cmds, a.resultList.SpinnerTick(), a.triggerSearch())
	case QueryChangedMsg:
		if a.history != nil && msg.Query != "" {
			a.history.AddQuery(a.tabBar.Active().String(), msg.Query)
			_ = a.history.Save()
		}
		// Reset history cursor when typing a new query.
		a.queryCursor = -1
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
	case RemoveFromHistoryMsg:
		if a.history != nil {
			a.history.Remove(msg.FilePath)
			_ = a.history.Save()
			cmds = append(cmds, a.triggerSearch())
		}
	}

	var cmd tea.Cmd

	// When help overlay is visible, handle it.
	if a.helpOverlay.Visible() {
		if km, ok := msg.(tea.KeyMsg); ok {
			s := km.String()
			if s == "esc" || s == "?" {
				a.helpOverlay.Toggle()
				return a, nil
			}
			// Any other key also closes help, but let's allow scrolling first.
			if s == "up" || s == "down" || s == "pgup" || s == "pgdown" {
				a.helpOverlay, cmd = a.helpOverlay.Update(msg)
				return a, cmd
			}
			a.helpOverlay.Toggle()
			return a, nil
		}
		a.helpOverlay, cmd = a.helpOverlay.Update(msg)
		return a, cmd
	}

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

// waitForResults waits for the next message on the results channel.
func (a App) waitForResults(id SearchID, ch chan ResultsMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

// waitForIndexProgress waits for the next message on the index progress channel.
func (a App) waitForIndexProgress(ch chan IndexProgressMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

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
	a.helpOverlay.SetSize(w-10, h-4)
}

func (a App) refreshIndex() tea.Cmd {
	if a.index == nil {
		return nil
	}
	root := a.index.Root()
	ch := make(chan IndexProgressMsg, 10)

	return tea.Batch(
		func() tea.Msg {
			go func() {
				defer close(ch)
				rules := search.LoadIgnoreFiles(root)
				if len(a.cfg.IgnorePatterns) > 0 {
					rules.LoadPatterns(a.cfg.IgnorePatterns)
				}
				a.index.RebuildFrom(context.Background(), root, rules, search.WalkOptions{}, func(count int) {
					ch <- IndexProgressMsg{Count: count, Done: false, Ch: ch}
				})
				ch <- IndexProgressMsg{Done: true}
			}()
			return nil
		},
		a.waitForIndexProgress(ch),
	)
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
	id := a.searchCancel.NextID()

	if a.index == nil {
		return func() tea.Msg { return ResultsMsg{ID: id, Done: true} }
	}

	tab := a.tabBar.Active()
	switch tab {
	case model.TabAll:
		return a.triggerAllSearch(id)
	case model.TabText:
		return a.triggerGrepSearch(id)
	case model.TabSymbols:
		return a.triggerSymbolSearch(id)
	case model.TabClasses:
		return a.triggerClassSearch(id)
	case model.TabActions:
		return a.triggerActionSearch(id)
	default:
		return a.triggerFileSearch(id)
	}
}

func (a App) triggerAllSearch(id SearchID) tea.Cmd {
	query := a.input.Value()
	includeHidden := a.showHidden || !a.statusBar.ProjectOnly()
	projectOnly := a.statusBar.ProjectOnly()
	idx := a.index
	extFilters := a.filterMenu.SelectedExtensions()
	limit := a.cfg.MaxResults / 4 // split across 4 sections
	if limit < 5 {
		limit = 5
	}
	sc := a.searchCancel

	ctx, cancel := context.WithCancel(context.Background())
	sc.Set(id, cancel)

	ch := make(chan ResultsMsg, 100)

	go func() {
		defer cancel()
		defer close(ch)

		first := true
		totalMatched := 0

		// Special case: empty query shows Recent Files only.
		if strings.TrimSpace(query) == "" && a.history != nil {
			recents := a.history.RecentFiles
			if len(recents) > 0 {
				// idx.Search with empty query already boosts and sorts by mod time.
				// We'll let it handle the heavy lifting and just use the header.
			}
		}

		// Files
		filesRs := idx.Search(ctx, search.SearchOptions{
			Query:         query,
			Tab:           model.TabFiles,
			ExtFilters:    extFilters,
			MaxResults:    limit,
			IncludeHidden: includeHidden,
			ProjectOnly:   projectOnly,
			History:       a.history,
		})
		if ctx.Err() != nil {
			return
		}

		var filesItems []model.SearchResult
		if len(filesRs.Items) > 0 {
			header := "Files"
			if strings.TrimSpace(query) == "" {
				header = "Recent Files"
			}
			filesItems = append(filesItems, model.SearchResult{
				Name:       header,
				IsHeader:   true,
				SectionTab: model.TabFiles,
			})
			filesItems = append(filesItems, filesRs.Items...)
			if filesRs.TotalMatched > limit {
				filesItems = append(filesItems, model.SearchResult{
					Name:       fmt.Sprintf("  … more Files (%d total)", filesRs.TotalMatched),
					SectionTab: model.TabFiles,
				})
			}
			totalMatched += len(filesItems)
			ch <- ResultsMsg{ID: id, Items: filesItems, TotalMatched: totalMatched, Append: false, Done: false, Ch: ch}
			first = false
		} else {
			// Clear the list if this is the first (and only) section so far.
			ch <- ResultsMsg{ID: id, Items: nil, TotalMatched: 0, Append: false, Done: false, Ch: ch}
			first = false
		}

		// If query is empty, we only show Recent Files (Story 5.6).
		if strings.TrimSpace(query) == "" {
			ch <- ResultsMsg{ID: id, Append: true, Done: true}
			return
		}

		// Classes
		classesRs := idx.SearchClasses(ctx, query, limit, includeHidden, projectOnly, a.history)
		if ctx.Err() != nil {
			return
		}
		var classesItems []model.SearchResult
		if len(classesRs.Items) > 0 {
			classesItems = append(classesItems, model.SearchResult{
				Name:       "Classes",
				IsHeader:   true,
				SectionTab: model.TabClasses,
			})
			classesItems = append(classesItems, classesRs.Items...)
			if classesRs.TotalMatched > limit {
				classesItems = append(classesItems, model.SearchResult{
					Name:       fmt.Sprintf("  … more Classes (%d total)", classesRs.TotalMatched),
					SectionTab: model.TabClasses,
				})
			}
			totalMatched += len(classesItems)
			ch <- ResultsMsg{ID: id, Items: classesItems, TotalMatched: totalMatched, Append: true, Done: false, Ch: ch}
			first = false
		}

		// Symbols
		symbolsRs := idx.SearchSymbols(ctx, query, limit, includeHidden, projectOnly, a.history)
		if ctx.Err() != nil {
			return
		}
		var symbolsItems []model.SearchResult
		if len(symbolsRs.Items) > 0 {
			symbolsItems = append(symbolsItems, model.SearchResult{
				Name:       "Symbols",
				IsHeader:   true,
				SectionTab: model.TabSymbols,
			})
			symbolsItems = append(symbolsItems, symbolsRs.Items...)
			if symbolsRs.TotalMatched > limit {
				symbolsItems = append(symbolsItems, model.SearchResult{
					Name:       fmt.Sprintf("  … more Symbols (%d total)", symbolsRs.TotalMatched),
					SectionTab: model.TabSymbols,
				})
			}
			totalMatched += len(symbolsItems)
			ch <- ResultsMsg{ID: id, Items: symbolsItems, TotalMatched: totalMatched, Append: true, Done: false, Ch: ch}
			first = false
		}

		// Actions
		actions := a.searchActions(query)
		if len(actions) > 0 {
			var actionsItems []model.SearchResult
			totalActions := len(actions)
			aLimit := limit
			if aLimit > totalActions {
				aLimit = totalActions
			}
			actionsItems = append(actionsItems, model.SearchResult{
				Name:       "Actions",
				IsHeader:   true,
				SectionTab: model.TabActions,
			})
			actionsItems = append(actionsItems, actions[:aLimit]...)
			if totalActions > limit {
				actionsItems = append(actionsItems, model.SearchResult{
					Name:       fmt.Sprintf("  … more Actions (%d total)", totalActions),
					SectionTab: model.TabActions,
				})
			}
			totalMatched += len(actionsItems)
			ch <- ResultsMsg{ID: id, Items: actionsItems, TotalMatched: totalMatched, Append: true, Done: false, Ch: ch}
			first = false
		}

		if first {
			ch <- ResultsMsg{ID: id, Items: nil, TotalMatched: 0, Append: false, Done: true}
		} else {
			// Final signal, use Append: true to avoid clearing results already sent.
			ch <- ResultsMsg{ID: id, Append: true, Done: true}
		}
	}()

	return a.waitForResults(id, ch)
}

func (a App) triggerFileSearch(id SearchID) tea.Cmd {
	idx := a.index
	query := a.input.Value()
	tab := a.tabBar.Active()
	extFilters := a.filterMenu.SelectedExtensions()
	includeHidden := a.showHidden || !a.statusBar.ProjectOnly()
	projectOnly := a.statusBar.ProjectOnly()
	sc := a.searchCancel

	ctx, cancel := context.WithCancel(context.Background())
	sc.Set(id, cancel)

	ch := make(chan ResultsMsg, 100)

	go func() {
		defer cancel()
		defer close(ch)

		rs := idx.Search(ctx, search.SearchOptions{
			Query:         query,
			Tab:           tab,
			ExtFilters:    extFilters,
			MaxResults:    a.cfg.MaxResults,
			IncludeHidden: includeHidden,
			ProjectOnly:   projectOnly,
			History:       a.history,
		})
		if ctx.Err() != nil {
			return
		}

		if len(rs.Items) == 0 {
			ch <- ResultsMsg{ID: id, Items: nil, TotalMatched: 0, Append: false, Done: true}
			return
		}

		// Batch results (50 at a time)
		batchSize := 50
		for i := 0; i < len(rs.Items); i += batchSize {
			end := i + batchSize
			if end > len(rs.Items) {
				end = len(rs.Items)
			}

			ch <- ResultsMsg{
				ID:           id,
				Items:        rs.Items[i:end],
				TotalMatched: rs.TotalMatched,
				Append:       i > 0,
				Done:         end == len(rs.Items),
				Ch:           ch,
			}

			if end == len(rs.Items) {
				return
			}

			// Check for cancellation between batches.
			if ctx.Err() != nil {
				return
			}
		}
	}()

	return a.waitForResults(id, ch)
}

func (a App) triggerSymbolSearch(id SearchID) tea.Cmd {
	idx := a.index
	query := a.input.Value()
	includeHidden := a.showHidden || !a.statusBar.ProjectOnly()
	projectOnly := a.statusBar.ProjectOnly()
	sc := a.searchCancel

	ctx, cancel := context.WithCancel(context.Background())
	sc.Set(id, cancel)

	ch := make(chan ResultsMsg, 100)

	go func() {
		defer cancel()
		defer close(ch)

		rs := idx.SearchSymbols(ctx, query, a.cfg.MaxResults, includeHidden, projectOnly, a.history)
		if ctx.Err() != nil {
			return
		}

		if len(rs.Items) == 0 {
			ch <- ResultsMsg{ID: id, Items: nil, TotalMatched: 0, Append: false, Done: true}
			return
		}

		// Batch results
		batchSize := 50
		for i := 0; i < len(rs.Items); i += batchSize {
			end := i + batchSize
			if end > len(rs.Items) {
				end = len(rs.Items)
			}

			ch <- ResultsMsg{
				ID:           id,
				Items:        rs.Items[i:end],
				TotalMatched: rs.TotalMatched,
				Append:       i > 0,
				Done:         end == len(rs.Items),
				Ch:           ch,
			}

			if end == len(rs.Items) {
				return
			}
			if ctx.Err() != nil {
				return
			}
		}
	}()

	return a.waitForResults(id, ch)
}

func (a App) triggerClassSearch(id SearchID) tea.Cmd {
	idx := a.index
	query := a.input.Value()
	includeHidden := a.showHidden || !a.statusBar.ProjectOnly()
	projectOnly := a.statusBar.ProjectOnly()
	sc := a.searchCancel

	ctx, cancel := context.WithCancel(context.Background())
	sc.Set(id, cancel)

	ch := make(chan ResultsMsg, 100)

	go func() {
		defer cancel()
		defer close(ch)

		rs := idx.SearchClasses(ctx, query, a.cfg.MaxResults, includeHidden, projectOnly, a.history)
		if ctx.Err() != nil {
			return
		}

		if len(rs.Items) == 0 {
			ch <- ResultsMsg{ID: id, Items: nil, TotalMatched: 0, Append: false, Done: true}
			return
		}

		// Batch results
		batchSize := 50
		for i := 0; i < len(rs.Items); i += batchSize {
			end := i + batchSize
			if end > len(rs.Items) {
				end = len(rs.Items)
			}

			ch <- ResultsMsg{
				ID:           id,
				Items:        rs.Items[i:end],
				TotalMatched: rs.TotalMatched,
				Append:       i > 0,
				Done:         end == len(rs.Items),
				Ch:           ch,
			}

			if end == len(rs.Items) {
				return
			}
			if ctx.Err() != nil {
				return
			}
		}
	}()

	return a.waitForResults(id, ch)
}

func (a App) triggerActionSearch(id SearchID) tea.Cmd {
	query := a.input.Value()
	sc := a.searchCancel

	ctx, cancel := context.WithCancel(context.Background())
	sc.Set(id, cancel)

	ch := make(chan ResultsMsg, 100)

	go func() {
		defer cancel()
		defer close(ch)

		results := a.searchActions(query)
		if ctx.Err() != nil {
			return
		}

		if len(results) == 0 {
			ch <- ResultsMsg{ID: id, Items: nil, TotalMatched: 0, Append: false, Done: true}
			return
		}

		// Batch results
		batchSize := 50
		for i := 0; i < len(results); i += batchSize {
			end := i + batchSize
			if end > len(results) {
				end = len(results)
			}

			ch <- ResultsMsg{
				ID:           id,
				Items:        results[i:end],
				TotalMatched: len(results),
				Append:       i > 0,
				Done:         end == len(results),
				Ch:           ch,
			}

			if end == len(results) {
				return
			}
			if ctx.Err() != nil {
				return
			}
		}
	}()

	return a.waitForResults(id, ch)
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

	if len(results) > a.cfg.MaxResults {
		results = results[:a.cfg.MaxResults]
	}
	return results
}

func (a App) triggerGrepSearch(id SearchID) tea.Cmd {
	idx := a.index
	query := a.input.Value()
	includeHidden := a.showHidden || !a.statusBar.ProjectOnly()
	projectOnly := a.statusBar.ProjectOnly()
	sc := a.searchCancel

	ctx, cancel := context.WithCancel(context.Background())
	sc.Set(id, cancel)

	ch := make(chan ResultsMsg, 100)

	go func() {
		defer cancel()
		defer close(ch)

		grepCh := search.Grep(ctx, idx, query, search.GrepOptions{
			IncludeHidden: includeHidden,
			ProjectOnly:   projectOnly,
			MaxResults:    a.cfg.MaxResults,
		})

		var results []model.SearchResult
		count := 0
		first := true
		for {
			select {
			case m, ok := <-grepCh:
				if !ok {
					// Final message.
					if first && count == 0 {
						ch <- ResultsMsg{ID: id, Items: nil, TotalMatched: 0, Append: false, Done: true}
					} else {
						ch <- ResultsMsg{ID: id, Items: results, TotalMatched: count, Append: !first, Done: true}
					}
					return
				}
				count++
				results = append(results, search.GrepMatchToResult(m))
				if len(results) >= 50 {
					ch <- ResultsMsg{ID: id, Items: results, TotalMatched: count, Append: !first, Done: false, Ch: ch}
					results = nil
					first = false
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return a.waitForResults(id, ch)
}

func (a App) View() string {
	if a.width == 0 || a.height == 0 {
		return ""
	}

	if a.width < 40 || a.height < 10 {
		return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, "Terminal too small")
	}

	divider := lipgloss.NewStyle().
		Foreground(a.theme.Divider).
		Render(repeatChar("─", a.width-4))

	container := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(a.theme.Border).
		Foreground(a.theme.Foreground).
		Background(a.theme.Background).
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

	// Overlay help if visible.
	if a.helpOverlay.Visible() {
		overlay := a.helpOverlay.View()
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

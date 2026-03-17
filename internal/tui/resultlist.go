package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bob/boomerang/internal/model"
)

// ResultList is a scrollable, selectable list of search results.
type ResultList struct {
	items        []model.SearchResult
	cursor       int // selected index
	offset       int // first visible row
	height       int // visible rows
	width        int
	loading      bool
	spinner      spinner.Model
	totalMatched int // total before MaxResults truncation
	theme        Theme
}

// NewResultList returns an initialized ResultList.
func NewResultList(theme Theme) ResultList {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(theme.MatchHighlight)
	return ResultList{
		spinner: sp,
		theme:   theme,
	}
}

// SetSize sets the viewport dimensions.
func (r *ResultList) SetSize(w, h int) {
	r.width = w
	r.height = h
}

// SetItems replaces the result list. If resetCursor is true, it resets the cursor to 0.
func (r *ResultList) SetItems(items []model.SearchResult, resetCursor bool) {
	r.items = items
	r.loading = false
	if resetCursor {
		r.cursor = 0
		r.offset = 0
	} else {
		// Ensure cursor is still in bounds.
		if len(r.items) == 0 {
			r.cursor = 0
			r.offset = 0
		} else if r.cursor >= len(r.items) {
			r.cursor = len(r.items) - 1
		}
		r.ensureVisible()
	}
}

// SetTotalMatched stores the total match count (before truncation).
func (r *ResultList) SetTotalMatched(n int) {
	r.totalMatched = n
}

// SetLoading enables or disables the loading spinner.
func (r *ResultList) SetLoading(v bool) {
	r.loading = v
}

// Selected returns the currently selected result, if any.
func (r ResultList) Selected() (model.SearchResult, bool) {
	if len(r.items) == 0 {
		return model.SearchResult{}, false
	}
	return r.items[r.cursor], true
}

// Cursor returns the current selection index.
func (r ResultList) Cursor() int {
	return r.cursor
}

// Len returns the total number of items.
func (r ResultList) Len() int {
	return len(r.items)
}
// RemoveFromHistoryMsg is emitted when the user wants to remove a result from history.
type RemoveFromHistoryMsg struct {
	FilePath string
}

func (r ResultList) Update(msg tea.Msg) (ResultList, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k", "ctrl+p":
			r.moveUp(1)
		case "delete":
			if r.cursor < len(r.items) {
				item := r.items[r.cursor]
				if item.FilePath != "" {
					return r, func() tea.Msg {
						return RemoveFromHistoryMsg{FilePath: item.FilePath}
					}
				}
			}
		case "down", "j", "ctrl+n":
			r.moveDown(1)
		case "ctrl+up":
			r.scrollUp(1)
		case "ctrl+down":
			r.scrollDown(1)
		case "pgup":
			r.moveUp(r.pageSize())
		case "pgdown":
			r.moveDown(r.pageSize())
		case "home":
			r.cursor = 0
			r.skipHeaderDown()
			r.offset = 0
		case "end":
			if len(r.items) > 0 {
				r.cursor = len(r.items) - 1
				r.skipHeaderUp()
				r.ensureVisible()
			}
		case "enter":
			if r.cursor < len(r.items) {
				item := r.items[r.cursor]
				if item.IsHeader && item.SectionTab != model.TabAll {
					return r, func() tea.Msg {
						return TabChangedMsg{Tab: item.SectionTab}
					}
				}
				if strings.HasPrefix(item.Name, " … more") {
					return r, func() tea.Msg {
						return TabChangedMsg{Tab: item.SectionTab}
					}
				}
			}
		}
	case spinner.TickMsg:
		if r.loading {
			var cmd tea.Cmd
			r.spinner, cmd = r.spinner.Update(msg)
			return r, cmd
		}
	}
	return r, nil
}

func (r *ResultList) moveUp(n int) {
	if len(r.items) == 0 {
		return
	}

	for i := 0; i < n; i++ {
		prev := r.cursor
		r.cursor--
		if r.cursor < 0 {
			r.cursor = len(r.items) - 1
		}
		
		// Skip headers.
		if r.items[r.cursor].IsHeader {
			if r.cursor == 0 {
				r.cursor = len(r.items) - 1
			} else {
				r.cursor--
			}
		}

		// Fallback
		if r.items[r.cursor].IsHeader {
			r.cursor = prev
		}
	}
	r.ensureVisible()
}

func (r *ResultList) moveDown(n int) {
	if len(r.items) == 0 {
		return
	}

	max := len(r.items) - 1
	for i := 0; i < n; i++ {
		prev := r.cursor
		r.cursor++
		if r.cursor > max {
			r.cursor = 0
		}
		
		// Skip headers.
		if r.items[r.cursor].IsHeader {
			if r.cursor == max {
				r.cursor = 0
			} else {
				r.cursor++
			}
		}

		// Fallback
		if r.items[r.cursor].IsHeader {
			r.cursor = prev
		}
	}
	r.ensureVisible()
}

func (r *ResultList) scrollUp(n int) {
	r.offset -= n
	if r.offset < 0 {
		r.offset = 0
	}
}

func (r *ResultList) scrollDown(n int) {
	maxOffset := len(r.items) - r.height
	if maxOffset < 0 {
		maxOffset = 0
	}
	r.offset += n
	if r.offset > maxOffset {
		r.offset = maxOffset
	}
}

func (r *ResultList) skipHeaderDown() {
	if r.cursor < len(r.items) && r.items[r.cursor].IsHeader {
		if r.cursor < len(r.items)-1 {
			r.cursor++
		}
	}
}

func (r *ResultList) skipHeaderUp() {
	if r.cursor >= 0 && r.cursor < len(r.items) && r.items[r.cursor].IsHeader {
		if r.cursor > 0 {
			r.cursor--
		}
	}
}

func (r *ResultList) ensureVisible() {
	if r.cursor < r.offset {
		r.offset = r.cursor
	}
	if r.height > 0 && r.cursor >= r.offset+r.height {
		r.offset = r.cursor - r.height + 1
	}
}

func (r ResultList) pageSize() int {
	if r.height > 1 {
		return r.height - 1
	}
	return 1
}

// SpinnerTick returns the cmd to start the spinner.
func (r ResultList) SpinnerTick() tea.Cmd {
	return r.spinner.Tick
}

// View renders the result list.
func (r ResultList) View() string {
	if r.loading {
		msg := r.spinner.View() + " Searching…"
		return r.centeredMessage(msg)
	}

	if len(r.items) == 0 {
		return r.centeredMessage(
			lipgloss.NewStyle().Foreground(r.theme.DimForeground).Render("No results found"),
		)
	}

	end := r.offset + r.height
	if end > len(r.items) {
		end = len(r.items)
	}
	visible := r.items[r.offset:end]

	var rows []string
	for i, item := range visible {
		idx := r.offset + i
		rows = append(rows, r.renderRow(item, idx == r.cursor))
	}

	// Pad remaining height with empty lines.
	for len(rows) < r.height {
		rows = append(rows, "")
	}

	// Scroll indicator column.
	indicators := r.scrollIndicators(len(rows))
	var lines []string
	for i, row := range rows {
		ind := " "
		if i < len(indicators) {
			ind = indicators[i]
		}
		lines = append(lines, row+ind)
	}

	return strings.Join(lines, "\n")
}

func (r ResultList) renderRow(item model.SearchResult, selected bool) string {
	if item.IsHeader {
		return r.renderHeader(item)
	}

	contentWidth := r.width - 5 // icon(3) + gap(1) + scroll indicator(1)
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Icon.
	iconColor := r.getIconColor(item)
	iconStyle := lipgloss.NewStyle().Foreground(iconColor)
	icon := item.Icon
	if icon == "" {
		icon = "·"
	}
	iconStr := iconStyle.Render(fmt.Sprintf(" %s ", icon))

	// Name with match highlighting.
	name := r.highlightName(item.Name, item.MatchRanges, selected)

	// Detail (right-aligned, dimmed).
	detail := lipgloss.NewStyle().Foreground(r.theme.ResultDetail).Render(item.Detail)

	nameWidth := contentWidth - lipgloss.Width(detail)
	if nameWidth < 4 {
		nameWidth = 4
	}
	namePadded := lipgloss.NewStyle().Width(nameWidth).Render(name)

	row := iconStr + namePadded + detail

	if selected {
		row = lipgloss.NewStyle().
			Background(r.theme.ResultSelected).
			Bold(true).
			Width(r.width - 1). // minus scroll indicator
			Render(row)
	}

	return row
}

func (r ResultList) getIconColor(item model.SearchResult) lipgloss.Color {
	switch item.ResultType {
	case model.ResultFile:
		return r.theme.IconFile
	case model.ResultSymbol:
		return r.theme.IconSymbol
	case model.ResultClass:
		return r.theme.IconClass
	case model.ResultAction:
		return r.theme.IconAction
	case model.ResultText:
		return r.theme.IconText
	default:
		return r.theme.MatchHighlight
	}
}

func (r ResultList) renderHeader(item model.SearchResult) string {
	headerStyle := lipgloss.NewStyle().
		Foreground(r.theme.SectionHeader).
		Bold(true)

	ruleStyle := lipgloss.NewStyle().
		Foreground(r.theme.SectionRule)

	text := " ── " + item.Name + " "
	ruleLen := r.width - lipgloss.Width(text) - 1
	if ruleLen < 0 {
		ruleLen = 0
	}
	rule := repeatChar("─", ruleLen)

	return headerStyle.Render(text) + ruleStyle.Render(rule)
}

func (r ResultList) highlightName(name string, ranges []model.MatchRange, selected bool) string {
	if len(ranges) == 0 {
		style := lipgloss.NewStyle().Foreground(r.theme.ResultName)
		if selected {
			style = style.Bold(true)
		}
		return style.Render(name)
	}

	matchStyle := lipgloss.NewStyle().
		Foreground(r.theme.MatchHighlight).
		Bold(true)
	plainStyle := lipgloss.NewStyle().Foreground(r.theme.ResultName)
	if selected {
		plainStyle = plainStyle.Bold(true)
	}

	runes := []rune(name)
	matched := make([]bool, len(runes))
	for _, mr := range ranges {
		for i := mr.Start; i < mr.End && i < len(runes); i++ {
			matched[i] = true
		}
	}

	var out strings.Builder
	i := 0
	for i < len(runes) {
		if matched[i] {
			j := i
			for j < len(runes) && matched[j] {
				j++
			}
			out.WriteString(matchStyle.Render(string(runes[i:j])))
			i = j
		} else {
			j := i
			for j < len(runes) && !matched[j] {
				j++
			}
			out.WriteString(plainStyle.Render(string(runes[i:j])))
			i = j
		}
	}
	return out.String()
}

func (r ResultList) scrollIndicators(visibleRows int) []string {
	total := len(r.items)
	if total <= r.height || r.height <= 0 {
		out := make([]string, visibleRows)
		for i := range out {
			out[i] = " "
		}
		return out
	}

	out := make([]string, visibleRows)
	style := lipgloss.NewStyle().Foreground(r.theme.DimForeground)

	// Compute thumb position and size.
	thumbSize := r.height * r.height / total
	if thumbSize < 1 {
		thumbSize = 1
	}
	maxOffset := total - r.height
	thumbPos := 0
	if maxOffset > 0 {
		thumbPos = r.offset * (r.height - thumbSize) / maxOffset
	}

	for i := 0; i < visibleRows; i++ {
		if i >= thumbPos && i < thumbPos+thumbSize {
			out[i] = style.Render("┃")
		} else {
			out[i] = style.Render("│")
		}
	}

	// Top/bottom arrows.
	if r.offset > 0 {
		out[0] = style.Render("▲")
	}
	if r.offset+r.height < total {
		out[visibleRows-1] = style.Render("▼")
	}

	return out
}

func (r ResultList) centeredMessage(msg string) string {
	if r.width <= 0 || r.height <= 0 {
		return msg
	}
	return lipgloss.Place(r.width, r.height, lipgloss.Center, lipgloss.Center, msg)
}

package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bob/boomerang/internal/model"
)

var (
	accentColor = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	dimColor    = lipgloss.AdaptiveColor{Light: "#999999", Dark: "#666666"}
	selectBg    = lipgloss.AdaptiveColor{Light: "#E8DFFB", Dark: "#2D2150"}
	normalFg    = lipgloss.AdaptiveColor{Light: "#333333", Dark: "#DDDDDD"}
)

// ResultList is a scrollable, selectable list of search results.
type ResultList struct {
	items    []model.SearchResult
	cursor   int // selected index
	offset   int // first visible row
	height   int // visible rows
	width    int
	loading  bool
	spinner  spinner.Model
}

// NewResultList returns an initialized ResultList.
func NewResultList() ResultList {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(accentColor)
	return ResultList{spinner: sp}
}

// SetSize sets the viewport dimensions.
func (r *ResultList) SetSize(w, h int) {
	r.width = w
	r.height = h
}

// SetItems replaces the result list and resets the cursor.
func (r *ResultList) SetItems(items []model.SearchResult) {
	r.items = items
	r.cursor = 0
	r.offset = 0
	r.loading = false
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

func (r ResultList) Update(msg tea.Msg) (ResultList, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			r.moveUp(1)
		case "down", "j":
			r.moveDown(1)
		case "pgup":
			r.moveUp(r.pageSize())
		case "pgdown":
			r.moveDown(r.pageSize())
		case "home":
			r.cursor = 0
			r.offset = 0
		case "end":
			if len(r.items) > 0 {
				r.cursor = len(r.items) - 1
				r.ensureVisible()
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
	r.cursor -= n
	if r.cursor < 0 {
		r.cursor = 0
	}
	r.ensureVisible()
}

func (r *ResultList) moveDown(n int) {
	r.cursor += n
	max := len(r.items) - 1
	if max < 0 {
		max = 0
	}
	if r.cursor > max {
		r.cursor = max
	}
	r.ensureVisible()
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
			lipgloss.NewStyle().Foreground(dimColor).Render("No results found"),
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
	contentWidth := r.width - 5 // icon(3) + gap(1) + scroll indicator(1)
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Icon.
	iconStyle := lipgloss.NewStyle()
	if item.IconColor != "" {
		iconStyle = iconStyle.Foreground(lipgloss.Color(item.IconColor))
	} else {
		iconStyle = iconStyle.Foreground(accentColor)
	}
	icon := item.Icon
	if icon == "" {
		icon = "·"
	}
	iconStr := iconStyle.Render(fmt.Sprintf(" %s ", icon))

	// Name with match highlighting.
	name := r.highlightName(item.Name, item.MatchRanges, selected)

	// Detail (right-aligned, dimmed).
	detail := lipgloss.NewStyle().Foreground(dimColor).Render(item.Detail)

	nameWidth := contentWidth - lipgloss.Width(detail)
	if nameWidth < 4 {
		nameWidth = 4
	}
	namePadded := lipgloss.NewStyle().Width(nameWidth).Render(name)

	row := iconStr + namePadded + detail

	if selected {
		row = lipgloss.NewStyle().
			Background(selectBg).
			Bold(true).
			Width(r.width - 1). // minus scroll indicator
			Render(row)
	}

	return row
}

func (r ResultList) highlightName(name string, ranges []model.MatchRange, selected bool) string {
	if len(ranges) == 0 {
		style := lipgloss.NewStyle().Foreground(normalFg)
		if selected {
			style = style.Bold(true)
		}
		return style.Render(name)
	}

	matchStyle := lipgloss.NewStyle().
		Foreground(accentColor).
		Bold(true)
	plainStyle := lipgloss.NewStyle().Foreground(normalFg)
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
	style := lipgloss.NewStyle().Foreground(dimColor)

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

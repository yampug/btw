package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yampug/btw/internal/search"
)

// FilterChangedMsg is emitted when the extension filter selection changes.
type FilterChangedMsg struct {
	Extensions []string // selected extensions (e.g. [".go", ".rs"])
}

// FilterMenu is an overlay that shows project file extensions as a checklist.
type FilterMenu struct {
	visible    bool
	extensions []search.ExtensionStats
	selected   map[string]bool
	cursor     int
	width      int
	height     int
	filterText string // for search-within-filter
	theme      Theme
}

// NewFilterMenu returns an initialized FilterMenu with the given theme.
func NewFilterMenu(theme Theme) FilterMenu {
	return FilterMenu{
		selected: make(map[string]bool),
		theme:    theme,
	}
}

// SetExtensions updates the available extensions list.
func (f *FilterMenu) SetExtensions(exts []search.ExtensionStats) {
	f.extensions = exts
}

// SetSize sets the overlay dimensions.
func (f *FilterMenu) SetSize(w, h int) {
	f.width = w
	f.height = h
}

// Visible reports whether the filter menu is open.
func (f FilterMenu) Visible() bool {
	return f.visible
}

// Toggle opens or closes the filter menu.
func (f *FilterMenu) Toggle() {
	f.visible = !f.visible
	if f.visible {
		f.cursor = 0
		f.filterText = ""
	}
}

// SetSelected explicitly sets the selected extensions.
func (f *FilterMenu) SetSelected(exts []string) {
	f.selected = make(map[string]bool)
	for _, ext := range exts {
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		f.selected[ext] = true
	}
}

// ClearSelected clears all selected extensions.
func (f *FilterMenu) ClearSelected() {
	f.selected = make(map[string]bool)
}

// SelectedExtensions returns the list of checked extensions.
func (f FilterMenu) SelectedExtensions() []string {
	var exts []string
	for ext, on := range f.selected {
		if on {
			exts = append(exts, ext)
		}
	}
	return exts
}

// BadgeText returns a short string for the input badge, e.g. ".go" or ".go,.rs".
func (f FilterMenu) BadgeText() string {
	exts := f.SelectedExtensions()
	if len(exts) == 0 {
		return ""
	}
	return strings.Join(exts, ",")
}

func (f FilterMenu) Update(msg tea.Msg) (FilterMenu, tea.Cmd) {
	if !f.visible {
		return f, nil
	}

	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return f, nil
	}

	filtered := f.filteredExtensions()
	maxIdx := len(filtered) - 1

	switch km.String() {
	case "up", "k":
		f.cursor--
		if f.cursor < 0 {
			f.cursor = 0
		}
	case "down", "j":
		f.cursor++
		if f.cursor > maxIdx {
			f.cursor = maxIdx
		}
	case " ", "enter":
		if maxIdx >= 0 {
			ext := filtered[f.cursor].Ext
			f.selected[ext] = !f.selected[ext]
			if !f.selected[ext] {
				delete(f.selected, ext)
			}
			return f, func() tea.Msg {
				return FilterChangedMsg{Extensions: f.SelectedExtensions()}
			}
		}
	case "a": // Select all visible
		for _, es := range filtered {
			f.selected[es.Ext] = true
		}
		return f, func() tea.Msg {
			return FilterChangedMsg{Extensions: f.SelectedExtensions()}
		}
	case "n": // Deselect all visible
		for _, es := range filtered {
			delete(f.selected, es.Ext)
		}
		return f, func() tea.Msg {
			return FilterChangedMsg{Extensions: f.SelectedExtensions()}
		}
	case "backspace":
		if len(f.filterText) > 0 {
			f.filterText = f.filterText[:len(f.filterText)-1]
			f.cursor = 0
		}
	case "esc", "ctrl+f":
		f.visible = false
	default:
		if len(km.String()) == 1 && km.Type == tea.KeyRunes {
			f.filterText += km.String()
			f.cursor = 0
		}
	}

	return f, nil
}

func (f FilterMenu) filteredExtensions() []search.ExtensionStats {
	if f.filterText == "" {
		return f.extensions
	}
	var res []search.ExtensionStats
	for _, es := range f.extensions {
		if strings.Contains(strings.ToLower(es.Ext), strings.ToLower(f.filterText)) {
			res = append(res, es)
		}
	}
	return res
}

// View renders the filter menu overlay.
func (f FilterMenu) View() string {
	if !f.visible {
		return ""
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(f.theme.SectionHeader)

	selStyle := lipgloss.NewStyle().
		Background(f.theme.ResultSelected).
		Bold(true)

	dimStyle := lipgloss.NewStyle().
		Foreground(f.theme.DimForeground)

	var lines []string
	header := titleStyle.Render(" Filter by extension")
	if f.filterText != "" {
		header += dimStyle.Render(" searching: ") + lipgloss.NewStyle().Foreground(f.theme.MatchHighlight).Render(f.filterText)
	}
	lines = append(lines, header)
	lines = append(lines, dimStyle.Render(" (a: all, n: none, space: toggle, esc: close)"))
	lines = append(lines, "")

	filtered := f.filteredExtensions()
	if len(filtered) == 0 {
		lines = append(lines, dimStyle.Render("  No matching extensions"))
	} else {
		maxVisible := f.height - 6
		if maxVisible < 3 {
			maxVisible = 3
		}
		if maxVisible > len(filtered) {
			maxVisible = len(filtered)
		}

		// Scroll window around cursor.
		start := 0
		if f.cursor >= maxVisible {
			start = f.cursor - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(filtered) {
			end = len(filtered)
			start = end - maxVisible
			if start < 0 {
				start = 0
			}
		}

		for i := start; i < end; i++ {
			es := filtered[i]
			check := "[ ]"
			if f.selected[es.Ext] {
				check = "[✓]"
			}
			line := fmt.Sprintf(" %s %-10s (%d files)", check, es.Ext, es.Count)
			if i == f.cursor {
				line = selStyle.Render(line)
			}
			lines = append(lines, line)
		}
	}

	content := strings.Join(lines, "\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(f.theme.Border).
		Background(f.theme.Background).
		Foreground(f.theme.Foreground).
		Padding(0, 1).
		Width(45)

	return boxStyle.Render(content)
}

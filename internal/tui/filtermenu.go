package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bob/boomerang/internal/search"
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
}

// NewFilterMenu returns an initialized FilterMenu.
func NewFilterMenu() FilterMenu {
	return FilterMenu{
		selected: make(map[string]bool),
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
	}
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

	maxIdx := len(f.extensions) - 1
	if maxIdx < 0 {
		return f, nil
	}

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
		ext := f.extensions[f.cursor].Ext
		f.selected[ext] = !f.selected[ext]
		if !f.selected[ext] {
			delete(f.selected, ext)
		}
		return f, func() tea.Msg {
			return FilterChangedMsg{Extensions: f.SelectedExtensions()}
		}
	case "esc", "ctrl+f":
		f.visible = false
	}

	return f, nil
}

// View renders the filter menu overlay.
func (f FilterMenu) View() string {
	if !f.visible || len(f.extensions) == 0 {
		return ""
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"})

	selStyle := lipgloss.NewStyle().
		Background(lipgloss.AdaptiveColor{Light: "#E8DFFB", Dark: "#2D2150"}).
		Bold(true)

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#666666"})

	var lines []string
	lines = append(lines, titleStyle.Render(" Filter by extension")+" "+dimStyle.Render("(space to toggle, esc to close)"))
	lines = append(lines, "")

	maxVisible := f.height - 4
	if maxVisible < 3 {
		maxVisible = 3
	}
	if maxVisible > len(f.extensions) {
		maxVisible = len(f.extensions)
	}

	// Scroll window around cursor.
	start := 0
	if f.cursor >= maxVisible {
		start = f.cursor - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(f.extensions) {
		end = len(f.extensions)
		start = end - maxVisible
		if start < 0 {
			start = 0
		}
	}

	for i := start; i < end; i++ {
		es := f.extensions[i]
		check := "[ ]"
		if f.selected[es.Ext] {
			check = "[x]"
		}
		line := fmt.Sprintf(" %s %-8s %d files", check, es.Ext, es.Count)
		if i == f.cursor {
			line = selStyle.Render(line)
		}
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}).
		Padding(0, 1).
		Width(40)

	return boxStyle.Render(content)
}

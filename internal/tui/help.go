package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// HelpOverlay is a component that shows available keybindings.
type HelpOverlay struct {
	visible bool
	theme   Theme
	width   int
	height  int
}

// NewHelpOverlay returns an initialized HelpOverlay.
func NewHelpOverlay(theme Theme) HelpOverlay {
	return HelpOverlay{
		theme: theme,
	}
}

// SetSize sets the overlay dimensions.
func (h *HelpOverlay) SetSize(w, hSize int) {
	h.width = w
	h.height = hSize
}

// Toggle opens or closes the help overlay.
func (h *HelpOverlay) Toggle() {
	h.visible = !h.visible
}

// Visible reports whether the overlay is open.
func (h HelpOverlay) Visible() bool {
	return h.visible
}

// View renders the help overlay.
func (h HelpOverlay) View() string {
	if !h.visible {
		return ""
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(h.theme.SectionHeader)

	keyStyle := lipgloss.NewStyle().
		Foreground(h.theme.InputIcon).
		Bold(true).
		Width(12)

	descStyle := lipgloss.NewStyle().
		Foreground(h.theme.Foreground)

	dimStyle := lipgloss.NewStyle().
		Foreground(h.theme.DimForeground)

	var sections []string

	// Navigation
	nav := []string{
		titleStyle.Render("Navigation"),
		h.entry(keyStyle, descStyle, "Up / Down", "Select result (wrap around)"),
		h.entry(keyStyle, descStyle, "Ctrl+N / P", "Next / Previous result"),
		h.entry(keyStyle, descStyle, "PgUp / Dn", "Page up / down"),
		h.entry(keyStyle, descStyle, "Tab / S-Tab", "Next / Previous tab"),
		h.entry(keyStyle, descStyle, "1 - 6", "Jump to tab (input empty)"),
	}
	sections = append(sections, strings.Join(nav, "\n"))

	// Actions
	actions := []string{
		titleStyle.Render("Actions"),
		h.entry(keyStyle, descStyle, "Enter", "Open selected result in editor"),
		h.entry(keyStyle, descStyle, "Ctrl+Y", "Copy absolute path"),
		h.entry(keyStyle, descStyle, "Ctrl+Shift+Y", "Copy relative path"),
		h.entry(keyStyle, descStyle, "Ctrl+O", "Reveal in File Manager"),
	}
	sections = append(sections, strings.Join(actions, "\n"))

	// Toggles & Global
	toggles := []string{
		titleStyle.Render("Toggles & Global"),
		h.entry(keyStyle, descStyle, "Ctrl+F", "Toggle extension filters"),
		h.entry(keyStyle, descStyle, "Ctrl+P", "Toggle scope (Project / All)"),
		h.entry(keyStyle, descStyle, "Ctrl+H", "Toggle hidden files"),
		h.entry(keyStyle, descStyle, "Ctrl+R", "Refresh search index"),
		h.entry(keyStyle, descStyle, "Esc / Ctrl+C", "Close overlay / Exit"),
		h.entry(keyStyle, descStyle, "?", "Toggle this help menu"),
	}
	sections = append(sections, strings.Join(toggles, "\n"))

	content := strings.Join(sections, "\n\n")
	content += "\n\n" + dimStyle.Render("Press any key to close help")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(h.theme.Border).
		Background(h.theme.Background).
		Padding(1, 2).
		Width(50)

	return boxStyle.Render(content)
}

func (h HelpOverlay) entry(keyS, descS lipgloss.Style, key, desc string) string {
	return keyS.Render(key) + " " + descS.Render(desc)
}

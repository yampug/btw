package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// HelpOverlay is a component that shows available keybindings.
type HelpOverlay struct {
	visible  bool
	theme    Theme
	width    int
	height   int
	viewport viewport.Model
}

// NewHelpOverlay returns an initialized HelpOverlay.
func NewHelpOverlay(theme Theme) HelpOverlay {
	vp := viewport.New(0, 0)
	return HelpOverlay{
		theme:    theme,
		viewport: vp,
	}
}

// SetSize sets the overlay dimensions.
func (h *HelpOverlay) SetSize(w, hSize int) {
	h.width = w
	h.height = hSize
	
	// Inner dimensions for viewport.
	innerW := w - 6 // minus borders and padding
	innerH := hSize - 4 // minus borders and title/footer
	if innerW < 0 { innerW = 0 }
	if innerH < 0 { innerH = 0 }
	
	h.viewport.Width = innerW
	h.viewport.Height = innerH
}

// Toggle opens or closes the help overlay.
func (h *HelpOverlay) Toggle() {
	h.visible = !h.visible
	if h.visible {
		h.viewport.SetContent(h.renderContent())
		h.viewport.YOffset = 0
	}
}

// Visible reports whether the overlay is open.
func (h HelpOverlay) Visible() bool {
	return h.visible
}

func (h HelpOverlay) Update(msg tea.Msg) (HelpOverlay, tea.Cmd) {
	if !h.visible {
		return h, nil
	}
	var cmd tea.Cmd
	h.viewport, cmd = h.viewport.Update(msg)
	return h, cmd
}

func (h HelpOverlay) renderContent() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(h.theme.SectionHeader).
		Underline(true)

	keyStyle := lipgloss.NewStyle().
		Foreground(h.theme.InputIcon).
		Bold(true).
		Width(12)

	descStyle := lipgloss.NewStyle().
		Foreground(h.theme.Foreground)

	var sections []string

	// Navigation
	nav := []string{
		titleStyle.Render("Navigation"),
		h.entry(keyStyle, descStyle, "Up / Down", "Select result (wrap around)"),
		h.entry(keyStyle, descStyle, "Ctrl+N / P", "Next / Previous result"),
		h.entry(keyStyle, descStyle, "Ctrl+↑ / ↓", "Cycle query history"),
		h.entry(keyStyle, descStyle, "PgUp / Dn", "Page up / down"),
		h.entry(keyStyle, descStyle, "Home / End", "Jump to first / last result"),
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

	// Search & Toggles
	toggles := []string{
		titleStyle.Render("Search & Toggles"),
		h.entry(keyStyle, descStyle, "Ctrl+F", "Toggle extension filters"),
		h.entry(keyStyle, descStyle, "Ctrl+P", "Toggle scope (Project / All)"),
		h.entry(keyStyle, descStyle, "Ctrl+H", "Toggle hidden files"),
		h.entry(keyStyle, descStyle, "Ctrl+R", "Refresh search index"),
		h.entry(keyStyle, descStyle, "Esc", "Close overlay"),
		h.entry(keyStyle, descStyle, "?", "Toggle this help menu"),
	}
	sections = append(sections, strings.Join(toggles, "\n"))

	return strings.Join(sections, "\n\n")
}

// View renders the help overlay.
func (h HelpOverlay) View() string {
	if !h.visible {
		return ""
	}

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(h.theme.TabActive).
		Padding(0, 1).
		Render("Boomerang Help")

	footer := lipgloss.NewStyle().
		Foreground(h.theme.DimForeground).
		Padding(0, 1).
		Render("Press any key to close · ↑/↓ to scroll")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(h.theme.Border).
		Background(h.theme.Background).
		Padding(0, 1)

	// Adjust width if not set.
	w := h.width
	if w == 0 {
		w = 60
	}
	boxStyle = boxStyle.Width(w - 2)

	content := header + "\n\n" + h.viewport.View() + "\n\n" + footer
	return boxStyle.Render(content)
}

func (h HelpOverlay) entry(keyS, descS lipgloss.Style, key, desc string) string {
	return keyS.Render(key) + " " + descS.Render(desc)
}

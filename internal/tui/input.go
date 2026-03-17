package tui

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var lineNumSuffix = regexp.MustCompile(`:(\d+)$`)

// QueryChangedMsg is emitted (debounced) when the search query changes.
type QueryChangedMsg struct {
	Query string
	Line  int // parsed from `:N` suffix; 0 if absent
}

// debounceTickMsg is an internal message for the debounce timer.
type debounceTickMsg struct {
	query string
}

// SearchInput wraps a Bubbles textinput with search-specific behaviour.
type SearchInput struct {
	input     textinput.Model
	filter    string // active extension filter (e.g. ".go")
	query     string // raw query text minus `:N`
	lineNum   int    // parsed line number from `:N`
	width     int
	debounce  time.Duration
	lastValue string // value when debounce timer started
}

// NewSearchInput returns an initialized, focused SearchInput.
func NewSearchInput() SearchInput {
	ti := textinput.New()
	ti.Placeholder = "Search everywhere..."
	ti.Focus()
	ti.CharLimit = 256
	ti.Prompt = ""

	return SearchInput{
		input:    ti,
		debounce: 150 * time.Millisecond,
	}
}

// SetWidth sets the available rendering width.
func (s *SearchInput) SetWidth(w int) {
	s.width = w
	// Reserve space for icon (4) and filter badge (up to 10).
	inputWidth := w - 4
	if s.filter != "" {
		inputWidth -= len(s.filter) + 3 // " [.go]"
	}
	if inputWidth < 10 {
		inputWidth = 10
	}
	s.input.Width = inputWidth
}

// Value returns the current query text (without `:N` suffix).
func (s SearchInput) Value() string {
	return s.query
}

// LineNum returns the parsed line number, or 0.
func (s SearchInput) LineNum() int {
	return s.lineNum
}

// Filter returns the active extension filter.
func (s SearchInput) Filter() string {
	return s.filter
}

// SetFilter sets an extension filter (e.g. ".go").
func (s *SearchInput) SetFilter(f string) {
	s.filter = f
	s.SetWidth(s.width)
}

// Focus gives keyboard focus to the input.
func (s *SearchInput) Focus() tea.Cmd {
	return s.input.Focus()
}

// Focused reports whether the input is focused.
func (s SearchInput) Focused() bool {
	return s.input.Focused()
}

func (s SearchInput) Update(msg tea.Msg) (SearchInput, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+u":
			s.input.SetValue("")
			s.parseValue()
			return s, s.startDebounce()
		case "ctrl+a":
			v := s.input.Value()
			s.input.SetCursor(len(v))
			// bubbles textinput doesn't have native select-all;
			// position cursor at end so next typing replaces via normal flow.
		}
	case debounceTickMsg:
		// Only emit if the value hasn't changed since the timer started.
		if msg.query == s.input.Value() {
			return s, func() tea.Msg {
				return QueryChangedMsg{Query: s.query, Line: s.lineNum}
			}
		}
		return s, nil
	}

	prev := s.input.Value()
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)

	if s.input.Value() != prev {
		s.parseValue()
		return s, tea.Batch(cmd, s.startDebounce())
	}
	return s, cmd
}

func (s *SearchInput) parseValue() {
	raw := s.input.Value()
	if m := lineNumSuffix.FindStringSubmatch(raw); m != nil {
		s.query = raw[:strings.LastIndex(raw, ":")]
		s.lineNum, _ = strconv.Atoi(m[1])
	} else {
		s.query = raw
		s.lineNum = 0
	}
}

func (s *SearchInput) startDebounce() tea.Cmd {
	val := s.input.Value()
	s.lastValue = val
	d := s.debounce
	return tea.Tick(d, func(time.Time) tea.Msg {
		return debounceTickMsg{query: val}
	})
}

// View renders the search input row.
func (s SearchInput) View() string {
	iconStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}).
		Bold(true)

	icon := iconStyle.Render(" 🔍 ")

	inputView := s.input.View()

	var badge string
	if s.filter != "" {
		badgeStyle := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFFFF"}).
			Background(lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}).
			Padding(0, 1)
		badge = " " + badgeStyle.Render(s.filter)
	}

	row := icon + inputView + badge

	if s.width > 0 {
		row = lipgloss.NewStyle().MaxWidth(s.width).Render(row)
	}
	return row
}

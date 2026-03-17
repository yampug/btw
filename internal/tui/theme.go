package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Theme defines the color palette for the TUI.
type Theme struct {
	Name string

	// Chrome
	Background    lipgloss.Color
	Foreground    lipgloss.Color
	DimForeground lipgloss.Color
	Border        lipgloss.Color
	Divider       lipgloss.Color

	// Tabs
	TabActive      lipgloss.Color
	TabInactive    lipgloss.Color
	TabHint        lipgloss.Color
	TabBackground  lipgloss.Color

	// Input
	InputText        lipgloss.Color
	InputPlaceholder lipgloss.Color
	InputIcon        lipgloss.Color
	InputBadge       lipgloss.Color
	InputBadgeText   lipgloss.Color

	// Results
	ResultName     lipgloss.Color
	ResultDetail   lipgloss.Color
	ResultSelected lipgloss.Color // Background for selected row
	MatchHighlight lipgloss.Color // Bold+color for matched chars

	// Icons (per result type)
	IconFile   lipgloss.Color
	IconSymbol lipgloss.Color
	IconClass  lipgloss.Color
	IconAction lipgloss.Color
	IconText   lipgloss.Color

	// Status bar
	StatusBg      lipgloss.Color
	StatusFg      lipgloss.Color
	StatusMessage lipgloss.Color

	// Section headers
	SectionHeader lipgloss.Color
	SectionRule   lipgloss.Color
}

// DarkTheme is the default Darcula-inspired theme.
var DarkTheme = Theme{
	Name: "dark",

	Background:    lipgloss.Color("#2B2B2B"),
	Foreground:    lipgloss.Color("#A9B7C6"),
	DimForeground: lipgloss.Color("#666666"),
	Border:        lipgloss.Color("#7D56F4"),
	Divider:       lipgloss.Color("#444444"),

	TabActive:     lipgloss.Color("#FFFFFF"),
	TabInactive:   lipgloss.Color("#888888"),
	TabHint:       lipgloss.Color("#555555"),
	TabBackground: lipgloss.Color("#7D56F4"),

	InputText:        lipgloss.Color("#FFFFFF"),
	InputPlaceholder: lipgloss.Color("#666666"),
	InputIcon:        lipgloss.Color("#7D56F4"),
	InputBadge:       lipgloss.Color("#7D56F4"),
	InputBadgeText:   lipgloss.Color("#FFFFFF"),

	ResultName:     lipgloss.Color("#DDDDDD"),
	ResultDetail:   lipgloss.Color("#666666"),
	ResultSelected: lipgloss.Color("#2D2150"),
	MatchHighlight: lipgloss.Color("#7D56F4"),

	IconFile:   lipgloss.Color("#00ADD8"),
	IconSymbol: lipgloss.Color("#A855F7"),
	IconClass:  lipgloss.Color("#3B82F6"),
	IconAction: lipgloss.Color("#FACC15"),
	IconText:   lipgloss.Color("#6C8EBF"),

	StatusBg:      lipgloss.Color("#1E1E1E"),
	StatusFg:      lipgloss.Color("#888888"),
	StatusMessage: lipgloss.Color("#28A745"),

	SectionHeader: lipgloss.Color("#7D56F4"),
	SectionRule:   lipgloss.Color("#444444"),
}

// LightTheme is the IntelliJ Light-inspired theme.
var LightTheme = Theme{
	Name: "light",

	Background:    lipgloss.Color("#FFFFFF"),
	Foreground:    lipgloss.Color("#000000"),
	DimForeground: lipgloss.Color("#999999"),
	Border:        lipgloss.Color("#874BFD"),
	Divider:       lipgloss.Color("#CCCCCC"),

	TabActive:     lipgloss.Color("#FFFFFF"),
	TabInactive:   lipgloss.Color("#666666"),
	TabHint:       lipgloss.Color("#999999"),
	TabBackground: lipgloss.Color("#874BFD"),

	InputText:        lipgloss.Color("#333333"),
	InputPlaceholder: lipgloss.Color("#999999"),
	InputIcon:        lipgloss.Color("#874BFD"),
	InputBadge:       lipgloss.Color("#874BFD"),
	InputBadgeText:   lipgloss.Color("#FFFFFF"),

	ResultName:     lipgloss.Color("#333333"),
	ResultDetail:   lipgloss.Color("#999999"),
	ResultSelected: lipgloss.Color("#E8DFFB"),
	MatchHighlight: lipgloss.Color("#874BFD"),

	IconFile:   lipgloss.Color("#00ADD8"),
	IconSymbol: lipgloss.Color("#A855F7"),
	IconClass:  lipgloss.Color("#3B82F6"),
	IconAction: lipgloss.Color("#F59E0B"),
	IconText:   lipgloss.Color("#6C8EBF"),

	StatusBg:      lipgloss.Color("#F2F2F2"),
	StatusFg:      lipgloss.Color("#666666"),
	StatusMessage: lipgloss.Color("#218838"),

	SectionHeader: lipgloss.Color("#874BFD"),
	SectionRule:   lipgloss.Color("#CCCCCC"),
}

// GetTheme returns the requested theme or falls back to dark.
func GetTheme(name string) Theme {
	switch name {
	case "light":
		return LightTheme
	default:
		return DarkTheme
	}
}

package ui

import "github.com/charmbracelet/lipgloss"

var (
	Accent    = lipgloss.Color("#7C3AED")
	AccentDim = lipgloss.Color("#A78BFA")
	Muted     = lipgloss.Color("#6B7280")
	Fg        = lipgloss.Color("#E5E7EB")
	BgDark    = lipgloss.Color("#111827")

	TabActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(Accent).
			Padding(0, 2)

	TabInactive = lipgloss.NewStyle().
			Foreground(Muted).
			Padding(0, 2)

	StatusBarStyle = lipgloss.NewStyle().
			Foreground(Fg).
			Background(lipgloss.Color("#1F2937")).
			Padding(0, 1)

	HelpKey = lipgloss.NewStyle().
		Foreground(AccentDim).
		Bold(true)

	HelpDesc = lipgloss.NewStyle().
			Foreground(Muted)

	PaneTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Accent).
			MarginBottom(1)

	Subtle = lipgloss.NewStyle().
		Foreground(Muted)
)

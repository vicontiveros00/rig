package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func RenderTabs(names []string, activeIdx, width int) string {
	var tabs []string
	for i, name := range names {
		if i == activeIdx {
			tabs = append(tabs, TabActive.Render(name))
		} else {
			tabs = append(tabs, TabInactive.Render(name))
		}
	}

	row := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
	border := lipgloss.NewStyle().
		Width(width).
		BorderBottom(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(Muted)

	gap := width - lipgloss.Width(row)
	if gap > 0 {
		row += strings.Repeat(" ", gap)
	}

	return border.Render(row)
}

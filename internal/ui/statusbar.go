package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

func RenderStatusBar(model, provider string, width int) string {
	left := fmt.Sprintf(" %s  %s/%s",
		HelpKey.Render("rig"),
		Subtle.Render(provider),
		lipgloss.NewStyle().Foreground(Fg).Render(model),
	)

	right := fmt.Sprintf("%s %s  %s %s ",
		HelpKey.Render("tab/shift+tab"),
		HelpDesc.Render("switch tab"),
		HelpKey.Render("ctrl+c"),
		HelpDesc.Render("quit"),
	)

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := width - leftW - rightW
	if gap < 0 {
		gap = 0
	}

	bar := StatusBarStyle.Width(width).Render(
		fmt.Sprintf("%s%*s%s", left, gap, "", right),
	)
	return bar
}

package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/vicontiveros00/rig/internal/version"
)

func RenderStatusBar(model, provider string, width int) string {
	left := fmt.Sprintf("%s %s  %s/%s",
		HelpKey.Render("rig"),
		Subtle.Render(version.String()),
		Subtle.Render(provider),
		lipgloss.NewStyle().Foreground(Fg).Render(model),
	)

	right := fmt.Sprintf("%s %s  %s %s",
		HelpKey.Render("tab/shift+tab"),
		HelpDesc.Render("switch tab"),
		HelpKey.Render("ctrl+c"),
		HelpDesc.Render("quit"),
	)

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	// StatusBarStyle has Padding(0,1) which adds 2 chars total
	innerWidth := width - 2
	gap := innerWidth - leftW - rightW
	if gap < 0 {
		gap = 0
	}

	content := fmt.Sprintf("%s%*s%s", left, gap, "", right)

	return StatusBarStyle.
		Width(width).
		MaxWidth(width).
		Render(content)
}

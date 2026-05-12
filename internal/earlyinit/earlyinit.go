package earlyinit

import "os"

func init() {
	// Set COLORFGBG before termenv/lipgloss/bubbletea init() functions run.
	// This prevents termenv from sending OSC 11 terminal background color
	// queries whose responses leak into textareas as garbage text.
	// "15;0" = white foreground on black background (dark theme).
	if os.Getenv("COLORFGBG") == "" {
		os.Setenv("COLORFGBG", "15;0")
	}
}

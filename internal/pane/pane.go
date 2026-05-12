package pane

import tea "github.com/charmbracelet/bubbletea"

type Pane interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (Pane, tea.Cmd)
	View() string
	Name() string
	ShortHelp() string
	SetSize(width, height int)
}

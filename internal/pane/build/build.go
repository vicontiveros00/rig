package build

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vicontiveros00/rig/internal/messages"
	"github.com/vicontiveros00/rig/internal/pane"
	"fmt"
)

type Pane struct {
	activePlanTitle string
	activePlanTasks string
	width           int
	height          int
}

func New() pane.Pane {
	return &Pane{}
}

func (p *Pane) Name() string      { return "build" }
func (p *Pane) ShortHelp() string { return "run builds and commands" }
func (p *Pane) SetSize(w, h int)  { p.width = w; p.height = h }
func (p *Pane) Init() tea.Cmd     { return nil }

func (p *Pane) Update(msg tea.Msg) (pane.Pane, tea.Cmd) {
	switch msg := msg.(type) {
	case messages.ActivePlanChangedMsg:
		p.activePlanTitle = msg.PlanTitle
		p.activePlanTasks = msg.PlanTasks
	}
	return p, nil
}

func (p *Pane) View() string {
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	msg := dim.Render("[ build — coming soon ]")

	if p.activePlanTitle != "" {
		msg += "\n\n" + dim.Render(fmt.Sprintf("active plan: %s", p.activePlanTitle))
	}

	return lipgloss.Place(p.width, p.height, lipgloss.Center, lipgloss.Center, msg)
}

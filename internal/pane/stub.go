package pane

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Stub struct {
	name      string
	shortHelp string
	width     int
	height    int
}

func NewStub(name, shortHelp string) *Stub {
	return &Stub{name: name, shortHelp: shortHelp}
}

func (s *Stub) Init() tea.Cmd                         { return nil }
func (s *Stub) Update(tea.Msg) (Pane, tea.Cmd)        { return s, nil }
func (s *Stub) Name() string                          { return s.name }
func (s *Stub) ShortHelp() string                     { return s.shortHelp }
func (s *Stub) SetSize(width, height int)              { s.width = width; s.height = height }

func (s *Stub) View() string {
	msg := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6B7280")).
		Bold(true).
		Render(fmt.Sprintf("[ %s — coming soon ]", s.name))

	return lipgloss.Place(s.width, s.height, lipgloss.Center, lipgloss.Center, msg)
}

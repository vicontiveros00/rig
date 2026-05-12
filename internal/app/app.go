package app

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vicontiveros00/rig/internal/messages"
	"github.com/vicontiveros00/rig/internal/pane"
	"github.com/vicontiveros00/rig/internal/ui"
)

type Model struct {
	panes     []pane.Pane
	activeIdx int
	width     int
	height    int
	model     string
	provider  string
}

func New(panes []pane.Pane, provider, model string) Model {
	return Model{
		panes:    panes,
		model:    model,
		provider: provider,
	}
}

func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	for _, p := range m.panes {
		if cmd := p.Init(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizePanes()
		active := m.panes[m.activeIdx]
		updated, cmd := active.Update(msg)
		m.panes[m.activeIdx] = updated
		return m, cmd

	case messages.ModelSelectedMsg:
		m.provider = msg.ProviderName
		m.model = msg.Model
		// Forward to all panes so chat can pick it up
		var cmds []tea.Cmd
		for i, p := range m.panes {
			updated, cmd := p.Update(msg)
			m.panes[i] = updated
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.NextTab):
			m.activeIdx = (m.activeIdx + 1) % len(m.panes)
			return m, nil
		case key.Matches(msg, keys.PrevTab):
			m.activeIdx = (m.activeIdx - 1 + len(m.panes)) % len(m.panes)
			return m, nil
		}
	}

	active := m.panes[m.activeIdx]
	updated, cmd := active.Update(msg)
	m.panes[m.activeIdx] = updated
	return m, cmd
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	names := make([]string, len(m.panes))
	for i, p := range m.panes {
		names[i] = p.Name()
	}

	tabBar := ui.RenderTabs(names, m.activeIdx, m.width)
	statusBar := ui.RenderStatusBar(m.model, m.provider, m.width)

	tabH := lipgloss.Height(tabBar)
	statusH := lipgloss.Height(statusBar)
	contentH := m.height - tabH - statusH
	if contentH < 1 {
		contentH = 1
	}

	content := m.panes[m.activeIdx].View()
	content = lipgloss.NewStyle().
		Width(m.width).
		Height(contentH).
		MaxHeight(contentH).
		Render(content)

	return lipgloss.JoinVertical(lipgloss.Left, tabBar, content, statusBar)
}

func (m *Model) resizePanes() {
	// 2 lines for tab bar (content + border), 1 for status bar, 2 for newlines
	paneHeight := m.height - 5
	if paneHeight < 1 {
		paneHeight = 1
	}
	for _, p := range m.panes {
		p.SetSize(m.width, paneHeight)
	}
}

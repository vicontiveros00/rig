package scratch

import (
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vicontiveros00/rig/internal/pane"
)

var (
	savedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981"))
	hintStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
)

type Pane struct {
	editor textarea.Model
	width  int
	height int
	dirty  bool
	saved  bool
	path   string
}

func scratchPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".rig", "scratch.md")
}

func New() pane.Pane {
	ta := textarea.New()
	ta.Placeholder = "start typing notes here..."
	ta.ShowLineNumbers = true
	ta.CharLimit = 0
	ta.Focus()

	p := &Pane{
		editor: ta,
		path:   scratchPath(),
	}

	if data, err := os.ReadFile(p.path); err == nil {
		ta.SetValue(string(data))
		p.editor = ta
	}

	return p
}

func (p *Pane) Name() string      { return "scratch" }
func (p *Pane) ShortHelp() string { return "persistent notepad" }

func (p *Pane) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.editor.SetWidth(width)
	p.editor.SetHeight(height - 2)
}

func (p *Pane) Init() tea.Cmd {
	return textarea.Blink
}

func (p *Pane) Update(msg tea.Msg) (pane.Pane, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+s":
			p.save()
			return p, nil
		}
	}

	var cmd tea.Cmd
	p.editor, cmd = p.editor.Update(msg)

	p.dirty = true
	p.saved = false

	return p, cmd
}

func (p *Pane) View() string {
	hint := hintStyle.Render("ctrl+s save")
	if p.saved {
		hint = savedStyle.Render("saved ✓")
	}

	return p.editor.View() + "\n" + hint
}

func (p *Pane) save() {
	dir := filepath.Dir(p.path)
	os.MkdirAll(dir, 0o755)
	os.WriteFile(p.path, []byte(p.editor.Value()), 0o644)
	p.dirty = false
	p.saved = true
}

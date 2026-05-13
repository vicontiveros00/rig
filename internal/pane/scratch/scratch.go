package scratch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vicontiveros00/rig/internal/history"
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

	pickerOpen  bool
	pickerItems []history.ScratchMeta
	pickerIdx   int
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

type scratchListLoadedMsg struct {
	metas []history.ScratchMeta
}

type scratchLoadedMsg struct {
	content string
	err     error
}

func (p *Pane) Update(msg tea.Msg) (pane.Pane, tea.Cmd) {
	switch msg := msg.(type) {
	case scratchListLoadedMsg:
		p.pickerItems = msg.metas
		p.pickerIdx = 0
		p.pickerOpen = true
		return p, nil

	case scratchLoadedMsg:
		if msg.err == nil {
			p.editor.SetValue(msg.content)
			p.dirty = true
			p.saved = false
		}
		return p, nil

	case tea.KeyMsg:
		if p.pickerOpen {
			return p.updatePicker(msg)
		}

		switch msg.String() {
		case "ctrl+s":
			p.save()
			return p, nil
		case "ctrl+n":
			p.archiveAndReset()
			return p, nil
		case "ctrl+o":
			return p, p.openPicker()
		}
	}

	var cmd tea.Cmd
	p.editor, cmd = p.editor.Update(msg)

	p.dirty = true
	p.saved = false

	return p, cmd
}

func (p *Pane) View() string {
	if p.pickerOpen {
		return p.viewPicker()
	}

	hint := hintStyle.Render("ctrl+s save  ctrl+n new  ctrl+o history")
	if p.saved {
		hint = savedStyle.Render("saved ✓") + "  " + hintStyle.Render("ctrl+n new  ctrl+o history")
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

func (p *Pane) archiveAndReset() {
	content := strings.TrimSpace(p.editor.Value())
	if content != "" {
		_ = history.ArchiveScratch(content)
	}
	p.editor.Reset()
	p.save()
	p.dirty = false
	p.saved = false
}

func (p *Pane) openPicker() tea.Cmd {
	return func() tea.Msg {
		metas, _ := history.ListScratches()
		return scratchListLoadedMsg{metas: metas}
	}
}

func (p *Pane) updatePicker(msg tea.KeyMsg) (pane.Pane, tea.Cmd) {
	switch msg.String() {
	case "esc":
		p.pickerOpen = false
		return p, nil
	case "up", "k":
		if p.pickerIdx > 0 {
			p.pickerIdx--
		}
	case "down", "j":
		if p.pickerIdx < len(p.pickerItems)-1 {
			p.pickerIdx++
		}
	case "enter":
		if len(p.pickerItems) > 0 {
			meta := p.pickerItems[p.pickerIdx]
			p.pickerOpen = false
			return p, func() tea.Msg {
				content, err := history.LoadScratch(meta.Filename)
				return scratchLoadedMsg{content: content, err: err}
			}
		}
	}
	return p, nil
}

func (p *Pane) viewPicker() string {
	var b strings.Builder

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8B5CF6"))
	b.WriteString(headerStyle.Render("  scratch history"))
	b.WriteString("\n\n")

	if len(p.pickerItems) == 0 {
		dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
		b.WriteString(dim.Render("  no archived scratches"))
		b.WriteString("\n")
	}

	for i, meta := range p.pickerItems {
		ts := meta.Timestamp.Format("2006-01-02 15:04")
		previewStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))

		line := fmt.Sprintf("  %s", ts)
		preview := fmt.Sprintf("    %s", previewStyle.Render(meta.Preview))

		if i == p.pickerIdx {
			line = lipgloss.NewStyle().
				Background(lipgloss.Color("#1E1B4B")).
				Width(p.width - 2).
				Render("> " + line[2:])
			preview = lipgloss.NewStyle().
				Background(lipgloss.Color("#1E1B4B")).
				Width(p.width - 2).
				Render(preview)
		}

		b.WriteString(line)
		b.WriteString("\n")
		b.WriteString(preview)
		b.WriteString("\n\n")
	}

	help := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	b.WriteString(help.Render("  ↑/↓ navigate  enter = load  esc = cancel"))

	return b.String()
}

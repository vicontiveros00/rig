package chat

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/vicontiveros00/rig/internal/llm"
	"github.com/vicontiveros00/rig/internal/messages"
	"github.com/vicontiveros00/rig/internal/pane"
)

type streamChunkMsg struct{ chunk llm.StreamChunk }

var (
	userStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7C3AED"))

	assistantStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#10B981"))

	errStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EF4444")).
			Bold(true)
)

type Pane struct {
	messages  []Message
	input     textarea.Model
	viewport  viewport.Model
	spinner   spinner.Model
	provider  llm.Provider
	model     string
	streaming bool
	width     int
	height    int
	renderer  *glamour.TermRenderer
	cancel    context.CancelFunc
	streamCh  <-chan llm.StreamChunk
	err       error
}

func New(provider llm.Provider, model string) pane.Pane {
	ta := textarea.New()
	ta.Placeholder = "type a message... (enter to send)"
	ta.ShowLineNumbers = false
	ta.SetHeight(3)
	ta.Focus()
	ta.CharLimit = 0

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED"))

	r, _ := glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(80),
	)

	return &Pane{
		input:    ta,
		spinner:  sp,
		provider: provider,
		model:    model,
		renderer: r,
	}
}

func (p *Pane) Name() string      { return "chat" }
func (p *Pane) ShortHelp() string { return "talk to an llm" }

func (p *Pane) SetSize(width, height int) {
	p.width = width
	p.height = height

	inputHeight := 3
	p.input.SetWidth(width - 2)
	p.input.SetHeight(inputHeight)

	vpHeight := height - inputHeight - 2
	if vpHeight < 1 {
		vpHeight = 1
	}
	p.viewport.Width = width
	p.viewport.Height = vpHeight

	wrapWidth := width - 4
	if wrapWidth < 40 {
		wrapWidth = 40
	}
	p.renderer, _ = glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(wrapWidth),
	)

	p.updateViewportContent()
}

func (p *Pane) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, p.spinner.Tick)
}

func (p *Pane) Update(msg tea.Msg) (pane.Pane, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case messages.ModelSelectedMsg:
		p.provider = msg.Provider
		p.model = msg.Model
		return p, nil

	case tea.KeyMsg:
		if p.streaming {
			if msg.String() == "esc" {
				if p.cancel != nil {
					p.cancel()
				}
				p.streaming = false
				return p, nil
			}
			return p, nil
		}

		switch msg.Type {
		case tea.KeyEnter:
			if msg.Alt {
				// Alt+Enter inserts newline
				break
			}
			text := strings.TrimSpace(p.input.Value())
			if text == "" {
				return p, nil
			}
			p.input.Reset()
			p.messages = append(p.messages, userMsg(text))
			p.messages = append(p.messages, assistantMsg())
			p.streaming = true
			p.err = nil
			p.updateViewportContent()
			return p, tea.Batch(p.startStream(), p.spinner.Tick)
		}

	case streamChunkMsg:
		if msg.chunk.Error != nil {
			p.err = msg.chunk.Error
			p.streaming = false
			p.updateViewportContent()
			return p, nil
		}
		if msg.chunk.Done {
			p.streaming = false
			p.updateViewportContent()
			return p, nil
		}
		if len(p.messages) > 0 {
			last := &p.messages[len(p.messages)-1]
			last.Content += msg.chunk.Content
		}
		p.updateViewportContent()
		return p, p.waitForChunk()

	case spinner.TickMsg:
		if p.streaming {
			var cmd tea.Cmd
			p.spinner, cmd = p.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	if !p.streaming {
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(msg)
		cmds = append(cmds, cmd)
	}

	var vpCmd tea.Cmd
	p.viewport, vpCmd = p.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return p, tea.Batch(cmds...)
}

func (p *Pane) View() string {
	vpView := p.viewport.View()
	inputView := p.input.View()

	var status string
	if p.streaming {
		status = p.spinner.View() + " streaming..."
	}
	if p.err != nil {
		status = errStyle.Render(fmt.Sprintf("error: %v", p.err))
	}

	var parts []string
	parts = append(parts, vpView)
	if status != "" {
		parts = append(parts, status)
	}
	parts = append(parts, inputView)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (p *Pane) updateViewportContent() {
	var sb strings.Builder
	for _, m := range p.messages {
		switch m.Role {
		case llm.RoleUser:
			sb.WriteString(userStyle.Render("you") + "\n")
			sb.WriteString(m.Content + "\n\n")
		case llm.RoleAssistant:
			sb.WriteString(assistantStyle.Render("rigby") + "\n")
			content := m.Content
			if content == "" && p.streaming {
				content = "..."
			}
			if p.renderer != nil && content != "" {
				rendered, err := p.renderer.Render(content)
				if err == nil {
					content = rendered
				}
			}
			sb.WriteString(content + "\n")
		}
	}
	p.viewport.SetContent(sb.String())
	p.viewport.GotoBottom()
}

func (p *Pane) startStream() tea.Cmd {
	msgs := make([]llm.Message, len(p.messages)-1)
	for i := 0; i < len(p.messages)-1; i++ {
		msgs[i] = p.messages[i].ToLLM()
	}

	provider := p.provider
	model := p.model

	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		p.cancel = cancel

		ch, err := provider.StreamChat(ctx, model, msgs)
		if err != nil {
			return streamChunkMsg{chunk: llm.StreamChunk{Error: err, Done: true}}
		}
		p.streamCh = ch
		chunk, ok := <-ch
		if !ok {
			return streamChunkMsg{chunk: llm.StreamChunk{Done: true}}
		}
		return streamChunkMsg{chunk: chunk}
	}
}

func (p *Pane) waitForChunk() tea.Cmd {
	ch := p.streamCh
	return func() tea.Msg {
		if ch == nil {
			return streamChunkMsg{chunk: llm.StreamChunk{Done: true}}
		}
		chunk, ok := <-ch
		if !ok {
			return streamChunkMsg{chunk: llm.StreamChunk{Done: true}}
		}
		return streamChunkMsg{chunk: chunk}
	}
}


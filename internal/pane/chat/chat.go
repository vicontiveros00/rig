package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/vicontiveros00/rig/internal/history"
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

	hintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280"))
)

type Pane struct {
	messages  []Message
	input     textarea.Model
	viewport  viewport.Model
	spinner   spinner.Model
	provider  llm.Provider
	model     string
	providerName string
	streaming bool
	width     int
	height    int
	renderer  *glamour.TermRenderer
	cancel    context.CancelFunc
	streamCh  <-chan llm.StreamChunk
	err       error

	sessionID  string
	createdAt  time.Time

	activePlanTitle string
	activePlanTasks string

	pickerOpen  bool
	pickerItems []history.ChatMeta
	pickerIdx   int
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
		input:     ta,
		spinner:   sp,
		provider:  provider,
		model:     model,
		renderer:  r,
		sessionID: history.GenerateChatID(model),
		createdAt: time.Now(),
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
		p.providerName = msg.ProviderName
		return p, nil

	case messages.ActivePlanChangedMsg:
		p.activePlanTitle = msg.PlanTitle
		p.activePlanTasks = msg.PlanTasks
		return p, nil

	case chatListLoadedMsg:
		p.pickerItems = msg.metas
		p.pickerIdx = 0
		p.pickerOpen = true
		return p, nil

	case chatSessionLoadedMsg:
		if msg.err == nil {
			p.sessionID = msg.session.ID
			p.createdAt = msg.session.CreatedAt
			p.messages = nil
			for _, r := range msg.session.Messages {
				p.messages = append(p.messages, fromRecord(r))
			}
			p.updateViewportContent()
		}
		return p, nil

	case tea.KeyMsg:
		if p.pickerOpen {
			return p.updatePicker(msg)
		}

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

		switch msg.String() {
		case "ctrl+n":
			p.newSession()
			return p, nil
		case "ctrl+o":
			return p, p.openPicker()
		}

		switch msg.Type {
		case tea.KeyEnter:
			if msg.Alt {
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
			p.autoSave()
			return p, nil
		}
		if msg.chunk.Done {
			p.streaming = false
			p.updateViewportContent()
			p.autoSave()
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

	if !p.streaming && !p.pickerOpen {
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
	if p.pickerOpen {
		return p.viewPicker()
	}

	vpView := p.viewport.View()
	inputView := p.input.View()

	var status string
	if p.streaming {
		status = p.spinner.View() + " streaming..."
	}
	if p.err != nil {
		status = errStyle.Render(fmt.Sprintf("error: %v", p.err))
	}

	help := hintStyle.Render("ctrl+n new chat  ctrl+o history")

	var parts []string
	parts = append(parts, vpView)
	if status != "" {
		parts = append(parts, status)
	}
	parts = append(parts, inputView)
	parts = append(parts, help)

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

var systemPrompt = `You are Rigby, a helpful assistant running inside rig — a multi-pane terminal UI crafted by Vic. You live in the chat pane alongside scratch, plan, build, git, mcp, models, and servers panes.

Keep responses concise and well-formatted in markdown. You can use code blocks, lists, and headings. The user sees your output rendered with glamour in a terminal viewport.

When the user asks about rig itself, you know it is a Go TUI built on Charm's Bubble Tea framework, configured via ~/.rig/config.yaml, and supports multiple LLM providers (OpenAI-compatible, Ollama, Anthropic) and MCP servers.`

func (p *Pane) buildSystemPrompt() string {
	prompt := systemPrompt
	if p.activePlanTasks != "" {
		prompt += fmt.Sprintf("\n\n## Active Plan\nThe user is currently working on: %q\n\n%s", p.activePlanTitle, p.activePlanTasks)
	}
	return prompt
}

func (p *Pane) startStream() tea.Cmd {
	msgs := make([]llm.Message, 0, len(p.messages))
	msgs = append(msgs, llm.Message{Role: llm.RoleSystem, Content: p.buildSystemPrompt()})
	for i := 0; i < len(p.messages)-1; i++ {
		msgs = append(msgs, p.messages[i].ToLLM())
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

func (p *Pane) autoSave() {
	if len(p.messages) == 0 {
		return
	}

	records := make([]history.MessageRecord, len(p.messages))
	for i, m := range p.messages {
		records[i] = m.ToRecord()
	}

	session := history.ChatSession{
		ID:        p.sessionID,
		Provider:  p.providerName,
		Model:     p.model,
		CreatedAt: p.createdAt,
		UpdatedAt: time.Now(),
		Messages:  records,
	}
	_ = history.SaveChat(session)
}

func (p *Pane) newSession() {
	p.autoSave()
	p.messages = nil
	p.sessionID = history.GenerateChatID(p.model)
	p.createdAt = time.Now()
	p.err = nil
	p.updateViewportContent()
}

func (p *Pane) openPicker() tea.Cmd {
	return func() tea.Msg {
		metas, _ := history.ListChats()
		return chatListLoadedMsg{metas: metas}
	}
}

type chatListLoadedMsg struct {
	metas []history.ChatMeta
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
			return p, p.loadSession(meta.ID)
		}
	}
	return p, nil
}

type chatSessionLoadedMsg struct {
	session history.ChatSession
	err     error
}

func (p *Pane) loadSession(id string) tea.Cmd {
	return func() tea.Msg {
		session, err := history.LoadChat(id)
		return chatSessionLoadedMsg{session: session, err: err}
	}
}

func (p *Pane) viewPicker() string {
	var b strings.Builder

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8B5CF6"))
	b.WriteString(headerStyle.Render("  recent conversations"))
	b.WriteString("\n\n")

	if len(p.pickerItems) == 0 {
		dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
		b.WriteString(dim.Render("  no saved conversations"))
		b.WriteString("\n")
	}

	for i, meta := range p.pickerItems {
		ts := meta.CreatedAt.Format("2006-01-02 15:04")
		modelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981"))
		previewStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))

		line := fmt.Sprintf("  %s  %s", ts, modelStyle.Render(meta.Model))
		preview := fmt.Sprintf("    %s", previewStyle.Render(fmt.Sprintf("%q", meta.Preview)))

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

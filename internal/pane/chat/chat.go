package chat

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/vicontiveros00/rig/internal/chatcore"
	"github.com/vicontiveros00/rig/internal/history"
	"github.com/vicontiveros00/rig/internal/llm"
	"github.com/vicontiveros00/rig/internal/messages"
	"github.com/vicontiveros00/rig/internal/pane"
)

var (
	errStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EF4444")).
			Bold(true)

	hintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280"))
)

type Pane struct {
	engine   chatcore.Engine
	input    textarea.Model
	viewport viewport.Model
	spinner  spinner.Model
	width    int
	height   int
	renderer *glamour.TermRenderer
	err      error

	providerName string
	sessionID    string
	createdAt    time.Time

	activePlanTitle string
	activePlanTasks string

	pickerOpen  bool
	pickerItems []history.ChatMeta
	pickerIdx   int
	pickerVP            viewport.Model
	pickerConfirmDelete bool
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
		engine:    chatcore.Engine{Provider: provider, Model: model},
		input:     ta,
		spinner:   sp,
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

	pickerH := height - 4
	if pickerH < 1 {
		pickerH = 1
	}
	p.pickerVP.Width = width
	p.pickerVP.Height = pickerH

	p.updateViewportContent()
}

func (p *Pane) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, p.spinner.Tick)
}

func (p *Pane) Update(msg tea.Msg) (pane.Pane, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case messages.ModelSelectedMsg:
		p.engine.SetProvider(msg.Provider, msg.Model)
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
			p.engine.Messages = nil
			for _, r := range msg.session.Messages {
				m := fromRecord(r)
				p.engine.Messages = append(p.engine.Messages, chatcore.Message{Role: m.Role, Content: m.Content})
			}
			p.updateViewportContent()
		}
		return p, nil

	case tea.KeyMsg:
		if p.pickerOpen {
			return p.updatePicker(msg)
		}

		if p.engine.Streaming {
			if msg.String() == "esc" {
				p.engine.CancelStream()
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
			p.engine.SendUser(text)
			p.err = nil
			p.updateViewportContent()
			return p, tea.Batch(p.engine.StartStream(p.buildSystemPrompt()), p.spinner.Tick)
		}

	case chatcore.StreamReadyMsg:
		cmd, err := p.engine.HandleReady(msg)
		if err != nil {
			p.err = err
			p.updateViewportContent()
			return p, nil
		}
		return p, cmd

	case chatcore.ChunkMsg:
		done := p.engine.HandleChunk(msg.Chunk)
		if msg.Chunk.Error != nil {
			p.err = msg.Chunk.Error
		}
		p.updateViewportContent()
		if done {
			p.autoSave()
			return p, nil
		}
		return p, p.engine.WaitForChunk()

	case spinner.TickMsg:
		var cmd tea.Cmd
		p.spinner, cmd = p.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	if !p.engine.Streaming && !p.pickerOpen {
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
	if p.engine.Streaming {
		last := ""
		if len(p.engine.Messages) > 0 {
			last = p.engine.Messages[len(p.engine.Messages)-1].Content
		}
		if last == "" {
			status = p.spinner.View() + " waiting for model..."
		} else {
			status = p.spinner.View() + " streaming..."
		}
	}
	if p.err != nil {
		status = errStyle.Render(fmt.Sprintf("error: %v", p.err))
	}

	ctx := chatcore.ContextStatus(&p.engine)
	help := hintStyle.Render("ctrl+n new  ctrl+o history") + "  " + ctx

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
	for _, m := range p.engine.Messages {
		switch m.Role {
		case llm.RoleUser:
			sb.WriteString(chatcore.UserStyle.Render("you") + "\n")
			sb.WriteString(m.Content + "\n\n")
		case llm.RoleAssistant:
			sb.WriteString(chatcore.AssistantStyle.Render("rigby") + "\n")
			content := m.Content
			if content == "" && p.engine.Streaming {
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

var systemPrompt = `You are Rigby, a friendly and versatile assistant running inside rig — a terminal UI crafted by Vic.

This is the chat pane — your space to talk about anything. The user might ask about code, work through a problem, chat about their day, ask general knowledge questions, test how you handle different topics, or just have a conversation. Be natural, helpful, and adapt to whatever they bring up.

Keep responses concise and well-formatted in markdown when it helps (code blocks, lists, headings), but don't over-format casual conversation. The output is rendered in a terminal viewport.

You have awareness of the user's active plan (if one exists) for context, but plan modifications happen in the plan pane — if they ask to edit the plan, point them there (tab to switch panes).`

func (p *Pane) buildSystemPrompt() string {
	prompt := systemPrompt
	if p.activePlanTasks != "" {
		prompt += fmt.Sprintf("\n\n## Active Plan\nThe user is currently working on: %q\n\n%s", p.activePlanTitle, p.activePlanTasks)
	}
	return prompt
}

func (p *Pane) autoSave() {
	if len(p.engine.Messages) == 0 {
		return
	}

	records := make([]history.MessageRecord, len(p.engine.Messages))
	for i, m := range p.engine.Messages {
		records[i] = history.MessageRecord{
			Role:    string(m.Role),
			Content: m.Content,
		}
	}

	session := history.ChatSession{
		ID:        p.sessionID,
		Provider:  p.providerName,
		Model:     p.engine.Model,
		CreatedAt: p.createdAt,
		UpdatedAt: time.Now(),
		Messages:  records,
	}
	_ = history.SaveChat(session)
}

func (p *Pane) newSession() {
	p.autoSave()
	p.engine.Messages = nil
	p.sessionID = history.GenerateChatID(p.engine.Model)
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
	if p.pickerConfirmDelete {
		switch msg.String() {
		case "y":
			if p.pickerIdx < len(p.pickerItems) {
				_ = history.DeleteChat(p.pickerItems[p.pickerIdx].ID)
				p.pickerItems = append(p.pickerItems[:p.pickerIdx], p.pickerItems[p.pickerIdx+1:]...)
				if p.pickerIdx >= len(p.pickerItems) && p.pickerIdx > 0 {
					p.pickerIdx--
				}
			}
			p.pickerConfirmDelete = false
		case "n", "esc":
			p.pickerConfirmDelete = false
		}
		return p, nil
	}

	switch msg.String() {
	case "esc":
		p.pickerOpen = false
		return p, nil
	case "up", "k":
		if p.pickerIdx > 0 {
			p.pickerIdx--
			p.scrollPickerToCursor()
		}
	case "down", "j":
		if p.pickerIdx < len(p.pickerItems)-1 {
			p.pickerIdx++
			p.scrollPickerToCursor()
		}
	case "enter":
		if len(p.pickerItems) > 0 {
			meta := p.pickerItems[p.pickerIdx]
			p.pickerOpen = false
			return p, p.loadSession(meta.ID)
		}
	case "d":
		if len(p.pickerItems) > 0 {
			p.pickerConfirmDelete = true
		}
	}
	return p, nil
}

func (p *Pane) scrollPickerToCursor() {
	// Each item takes ~3 lines (line + preview + blank)
	row := p.pickerIdx * 3
	if row < p.pickerVP.YOffset {
		p.pickerVP.SetYOffset(row)
	} else if row >= p.pickerVP.YOffset+p.pickerVP.Height {
		p.pickerVP.SetYOffset(row - p.pickerVP.Height + 3)
	}
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
	var content strings.Builder

	if len(p.pickerItems) == 0 {
		dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
		content.WriteString(dim.Render("  no saved conversations"))
		content.WriteString("\n")
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

		content.WriteString(line)
		content.WriteString("\n")
		content.WriteString(preview)
		content.WriteString("\n\n")
	}

	if p.pickerConfirmDelete && p.pickerIdx < len(p.pickerItems) {
		warn := lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Bold(true)
		preview := p.pickerItems[p.pickerIdx].Preview
		if len(preview) > 40 {
			preview = preview[:37] + "..."
		}
		content.WriteString(warn.Render(fmt.Sprintf("  delete %q? (y/n)", preview)))
		content.WriteString("\n")
	}

	p.pickerVP.SetContent(content.String())

	var b strings.Builder
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8B5CF6"))
	b.WriteString(headerStyle.Render("  recent conversations"))
	b.WriteString("\n\n")
	b.WriteString(p.pickerVP.View())
	b.WriteString("\n")
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	if p.pickerConfirmDelete {
		b.WriteString(help.Render("  y = delete  n = cancel"))
	} else {
		b.WriteString(help.Render("  ↑/↓ navigate  enter = load  d = delete  esc = cancel"))
	}

	return b.String()
}

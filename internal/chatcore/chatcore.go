package chatcore

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vicontiveros00/rig/internal/llm"
)

var (
	UserStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7C3AED"))

	AssistantStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#10B981"))
)

type ChunkMsg struct {
	Chunk llm.StreamChunk
}

type Message struct {
	Role    llm.Role
	Content string
}

type Engine struct {
	Provider  llm.Provider
	Model     string
	Messages  []Message
	Streaming bool
	streamCh  <-chan llm.StreamChunk
	cancel    context.CancelFunc
}

func (e *Engine) SetProvider(p llm.Provider, model string) {
	e.Provider = p
	e.Model = model
}

func (e *Engine) SendUser(text string) {
	e.Messages = append(e.Messages, Message{Role: llm.RoleUser, Content: text})
	e.Messages = append(e.Messages, Message{Role: llm.RoleAssistant, Content: ""})
	e.Streaming = true
}

func (e *Engine) StartStream(systemPrompt string) tea.Cmd {
	msgs := make([]llm.Message, 0, len(e.Messages)+1)
	msgs = append(msgs, llm.Message{Role: llm.RoleSystem, Content: systemPrompt})
	for i := 0; i < len(e.Messages)-1; i++ {
		msgs = append(msgs, llm.Message{Role: e.Messages[i].Role, Content: e.Messages[i].Content})
	}

	provider := e.Provider
	model := e.Model

	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		e.cancel = cancel

		ch, err := provider.StreamChat(ctx, model, msgs)
		if err != nil {
			return ChunkMsg{Chunk: llm.StreamChunk{Error: err, Done: true}}
		}
		e.streamCh = ch
		chunk, ok := <-ch
		if !ok {
			return ChunkMsg{Chunk: llm.StreamChunk{Done: true}}
		}
		return ChunkMsg{Chunk: chunk}
	}
}

func (e *Engine) WaitForChunk() tea.Cmd {
	ch := e.streamCh
	return func() tea.Msg {
		if ch == nil {
			return ChunkMsg{Chunk: llm.StreamChunk{Done: true}}
		}
		chunk, ok := <-ch
		if !ok {
			return ChunkMsg{Chunk: llm.StreamChunk{Done: true}}
		}
		return ChunkMsg{Chunk: chunk}
	}
}

// HandleChunk appends content to the last assistant message.
// Returns true when the stream is done.
func (e *Engine) HandleChunk(chunk llm.StreamChunk) bool {
	if chunk.Error != nil {
		e.Streaming = false
		return true
	}
	if chunk.Done {
		e.Streaming = false
		return true
	}
	if len(e.Messages) > 0 {
		last := &e.Messages[len(e.Messages)-1]
		last.Content += chunk.Content
	}
	return false
}

func (e *Engine) CancelStream() {
	if e.cancel != nil {
		e.cancel()
	}
	e.Streaming = false
}

func (e *Engine) LastAssistantContent() string {
	for i := len(e.Messages) - 1; i >= 0; i-- {
		if e.Messages[i].Role == llm.RoleAssistant {
			return e.Messages[i].Content
		}
	}
	return ""
}

func (e *Engine) RenderMessages() string {
	var sb strings.Builder
	for _, m := range e.Messages {
		switch m.Role {
		case llm.RoleUser:
			sb.WriteString(UserStyle.Render("you") + "\n")
			sb.WriteString(m.Content + "\n\n")
		case llm.RoleAssistant:
			sb.WriteString(AssistantStyle.Render("rigby") + "\n")
			content := m.Content
			if content == "" && e.Streaming {
				content = "..."
			}
			sb.WriteString(content + "\n\n")
		}
	}
	return sb.String()
}

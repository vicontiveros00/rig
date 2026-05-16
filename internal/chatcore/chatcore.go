package chatcore

import (
	"context"
	"fmt"
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
	Provider     llm.Provider
	Model        string
	Messages     []Message
	Streaming    bool
	PromptTokens int
	TotalTokens  int
	streamCh     <-chan llm.StreamChunk
	cancel       context.CancelFunc
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

type StreamReadyMsg struct {
	Ch  <-chan llm.StreamChunk
	Err error
}

func (e *Engine) StartStream(systemPrompt string) tea.Cmd {
	msgs := make([]llm.Message, 0, len(e.Messages)+1)
	msgs = append(msgs, llm.Message{Role: llm.RoleSystem, Content: systemPrompt})
	for i := 0; i < len(e.Messages)-1; i++ {
		msgs = append(msgs, llm.Message{Role: e.Messages[i].Role, Content: e.Messages[i].Content})
	}

	provider := e.Provider
	model := e.Model

	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel

	return func() tea.Msg {
		ch, err := provider.StreamChat(ctx, model, msgs)
		return StreamReadyMsg{Ch: ch, Err: err}
	}
}

// HandleReady processes a StreamReadyMsg. Call this when you receive one.
func (e *Engine) HandleReady(msg StreamReadyMsg) (tea.Cmd, error) {
	if msg.Err != nil {
		e.Streaming = false
		return nil, msg.Err
	}
	e.streamCh = msg.Ch
	return e.WaitForChunk(), nil
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
		if chunk.PromptTokens > 0 {
			e.PromptTokens = chunk.PromptTokens
		}
		if chunk.TotalTokens > 0 {
			e.TotalTokens = chunk.TotalTokens
		}
		if len(e.Messages) > 0 {
			last := &e.Messages[len(e.Messages)-1]
			if last.Role == llm.RoleAssistant && strings.TrimSpace(last.Content) == "" {
				last.Content = "(no response from model)"
			}
		}
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

// ContextStatus returns a formatted string showing token usage from API data only.
func ContextStatus(e *Engine) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))

	if e.TotalTokens == 0 {
		return style.Render("tokens: waiting for response")
	}

	return style.Render(fmt.Sprintf("%s tokens used", formatTokenCount(e.TotalTokens)))
}

func formatTokenCount(n int) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fm", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
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

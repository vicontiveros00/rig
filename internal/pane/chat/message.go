package chat

import (
	"time"

	"github.com/vicontiveros00/rig/internal/llm"
)

type Message struct {
	Role      llm.Role
	Content   string
	Timestamp time.Time
}

func userMsg(content string) Message {
	return Message{Role: llm.RoleUser, Content: content, Timestamp: time.Now()}
}

func assistantMsg() Message {
	return Message{Role: llm.RoleAssistant, Content: "", Timestamp: time.Now()}
}

func (m Message) ToLLM() llm.Message {
	return llm.Message{Role: m.Role, Content: m.Content}
}

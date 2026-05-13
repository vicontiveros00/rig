package chat

import (
	"time"

	"github.com/vicontiveros00/rig/internal/history"
	"github.com/vicontiveros00/rig/internal/llm"
)

type Message struct {
	Role      llm.Role  `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
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

func (m Message) ToRecord() history.MessageRecord {
	return history.MessageRecord{
		Role:      string(m.Role),
		Content:   m.Content,
		Timestamp: m.Timestamp,
	}
}

func fromRecord(r history.MessageRecord) Message {
	return Message{
		Role:      llm.Role(r.Role),
		Content:   r.Content,
		Timestamp: r.Timestamp,
	}
}

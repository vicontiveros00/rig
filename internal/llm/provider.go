package llm

import (
	"context"

	"github.com/vicontiveros00/rig/internal/config"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Message struct {
	Role    Role
	Content string
}

type StreamChunk struct {
	Content      string
	Reasoning    string // separate stream of reasoning/thinking tokens (e.g. DeepSeek R1, Claude extended thinking)
	Done         bool
	Error        error
	PromptTokens int
	TotalTokens  int
}

type Provider interface {
	StreamChat(ctx context.Context, model string, msgs []Message) (<-chan StreamChunk, error)
	ListModels(ctx context.Context) ([]string, error)
	Name() string
}

func NewProvider(name string, cfg config.ProviderConfig) (Provider, error) {
	switch name {
	case "ollama":
		return NewOllama(cfg), nil
	default:
		// All providers use the OpenAI-compatible API (openai, litellm, etc.)
		return NewOpenAI(cfg), nil
	}
}

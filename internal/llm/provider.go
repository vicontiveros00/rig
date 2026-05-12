package llm

import (
	"context"
	"fmt"

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
	Content string
	Done    bool
	Error   error
}

type Provider interface {
	StreamChat(ctx context.Context, model string, msgs []Message) (<-chan StreamChunk, error)
	ListModels(ctx context.Context) ([]string, error)
	Name() string
}

func NewProvider(name string, cfg config.ProviderConfig) (Provider, error) {
	switch name {
	case "openai":
		return NewOpenAI(cfg), nil
	case "ollama":
		return NewOllama(cfg), nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
}

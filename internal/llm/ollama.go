package llm

import (
	"github.com/vicontiveros00/rig/internal/config"
)

type OllamaProvider struct {
	*OpenAIProvider
}

func NewOllama(cfg config.ProviderConfig) *OllamaProvider {
	if cfg.Endpoint == "" {
		cfg.Endpoint = "http://localhost:11434/v1"
	}
	if cfg.APIKey == "" {
		cfg.APIKey = "ollama"
	}
	return &OllamaProvider{OpenAIProvider: NewOpenAI(cfg)}
}

func (o *OllamaProvider) Name() string { return "ollama" }

package llm

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/vicontiveros00/rig/internal/config"
)

type OpenAIProvider struct {
	client *openai.Client
}

func NewOpenAI(cfg config.ProviderConfig) *OpenAIProvider {
	ocfg := openai.DefaultConfig(cfg.APIKey)
	if cfg.Endpoint != "" {
		ocfg.BaseURL = cfg.Endpoint
	}
	ocfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	return &OpenAIProvider{client: openai.NewClientWithConfig(ocfg)}
}

func (o *OpenAIProvider) Name() string { return "openai" }

func (o *OpenAIProvider) StreamChat(ctx context.Context, model string, msgs []Message) (<-chan StreamChunk, error) {
	oaiMsgs := make([]openai.ChatCompletionMessage, len(msgs))
	for i, m := range msgs {
		oaiMsgs[i] = openai.ChatCompletionMessage{
			Role:    string(m.Role),
			Content: m.Content,
		}
	}

	stream, err := o.client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:         model,
		Messages:      oaiMsgs,
		Stream:        true,
		StreamOptions: &openai.StreamOptions{IncludeUsage: true},
	})
	if err != nil {
		return nil, err
	}

	ch := make(chan StreamChunk)
	go func() {
		defer close(ch)
		defer stream.Close()
		var promptTokens, totalTokens int
		for {
			resp, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				ch <- StreamChunk{Done: true, PromptTokens: promptTokens, TotalTokens: totalTokens}
				return
			}
			if err != nil {
				ch <- StreamChunk{Error: err, Done: true}
				return
			}
			if resp.Usage != nil {
				promptTokens = resp.Usage.PromptTokens
				totalTokens = resp.Usage.TotalTokens
			}
			if len(resp.Choices) > 0 {
				delta := resp.Choices[0].Delta
				ch <- StreamChunk{
					Content:   delta.Content,
					Reasoning: delta.ReasoningContent,
				}
			}
		}
	}()

	return ch, nil
}

func (o *OpenAIProvider) ListModels(ctx context.Context) ([]string, error) {
	resp, err := o.client.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(resp.Models))
	for i, m := range resp.Models {
		names[i] = m.ID
	}
	return names, nil
}

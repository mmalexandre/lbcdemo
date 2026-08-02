package main

import (
	"context"
	"fmt"

	openai "github.com/sashabaranov/go-openai"
)

type LLMClient struct {
	client *openai.Client
	model  string
}

type LLMResponse struct {
	Content      string
	Model        string
	InputTokens  int
	OutputTokens int
}

func NewLLMClient() *LLMClient {
	apiKey := getEnv("OPENAI_API_KEY", "")
	model := getEnv("OPENAI_MODEL", "gpt-4o-mini")
	return &LLMClient{
		client: openai.NewClient(apiKey),
		model:  model,
	}
}

func (l *LLMClient) Chat(ctx context.Context, systemPrompt, userMessage string) (*LLMResponse, error) {
	if l.client == nil {
		return nil, fmt.Errorf("LLM client not configured: OPENAI_API_KEY is missing")
	}

	req := openai.ChatCompletionRequest{
		Model: l.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: userMessage},
		},
	}

	resp, err := l.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("openai chat completion: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai returned no choices")
	}

	return &LLMResponse{
		Content:      resp.Choices[0].Message.Content,
		Model:        resp.Model,
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
	}, nil
}

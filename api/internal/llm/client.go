package llm

import (
	"context"
	"fmt"
	"os"

	openai "github.com/sashabaranov/go-openai"
)

type Client struct {
	client *openai.Client
	model  string
}

type Response struct {
	Content      string
	Model        string
	InputTokens  int
	OutputTokens int
}

func NewClient() *Client {
	apiKey := os.Getenv("OPENAI_API_KEY")
	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &Client{
		client: openai.NewClient(apiKey),
		model:  model,
	}
}

func (l *Client) Chat(ctx context.Context, systemPrompt, userMessage string) (*Response, error) {
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

	return &Response{
		Content:      resp.Choices[0].Message.Content,
		Model:        resp.Model,
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
	}, nil
}

package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/ersinkoc/WindowsTaskManager/internal/config"
)

// openaiRequest is the OpenAI /v1/chat/completions request body. The same
// schema is accepted by every "OpenAI compatible" provider: OpenAI itself,
// OpenRouter, Groq, DeepSeek, Together, Mistral, Fireworks, Ollama (>=0.1.14),
// LM Studio, vLLM, llama.cpp's server, etc.
type openaiRequest struct {
	Model       string          `json:"model"`
	Messages    []openaiMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	Stream      bool            `json:"stream"`
}

type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiResponse struct {
	Choices []struct {
		Message      openaiMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// callOpenAI invokes any OpenAI-compatible /v1/chat/completions endpoint.
// The default endpoint is OpenAI itself; the user can override it via
// cfg.AI.Endpoint to talk to OpenRouter / Groq / DeepSeek / Together /
// Mistral / Ollama / LM Studio / etc.
func (a *Advisor) callOpenAI(ctx context.Context, cfg *config.Config, prompt string) (string, *TokenUsage, error) {
	reqBody := openaiRequest{
		Model:     cfg.AI.Model,
		MaxTokens: cfg.AI.MaxTokens,
		Messages: []openaiMessage{
			{Role: "system", Content: SystemPrompt(cfg.AI.Language)},
			{Role: "user", Content: prompt},
		},
	}
	buf, err := marshalProviderBody(reqBody)
	if err != nil {
		return "", nil, err
	}

	endpoint := cfg.AI.Endpoint
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/chat/completions"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buf))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.AI.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.AI.APIKey)
	}
	for k, v := range cfg.AI.ExtraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("openai call: %w", err)
	}
	defer resp.Body.Close()

	body, err := readProviderBody(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return "", nil, fmt.Errorf("openai %d: %s", resp.StatusCode, truncateForError(string(body)))
	}

	var parsed openaiResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", nil, fmt.Errorf("parse response: %w", err)
	}
	if parsed.Error != nil {
		return "", nil, fmt.Errorf("openai %s: %s", parsed.Error.Type, parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", nil, errors.New("openai: empty response")
	}

	var usage *TokenUsage
	if parsed.Usage != nil {
		usage = &TokenUsage{
			Prompt:     uint64(parsed.Usage.PromptTokens),
			Completion: uint64(parsed.Usage.CompletionTokens),
			Total:      uint64(parsed.Usage.TotalTokens),
		}
	}

	return strings.TrimSpace(parsed.Choices[0].Message.Content), usage, nil
}

func truncateForError(s string) string {
	const max = 512
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

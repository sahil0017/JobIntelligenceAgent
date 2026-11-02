package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (provider *OpenAICompatible) Generate(ctx context.Context, request Request) (Response, error) {
	if strings.TrimSpace(request.UserPrompt) == "" {
		return Response{}, fmt.Errorf("user prompt is required")
	}
	payload := struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Temperature float64 `json:"temperature,omitempty"`
		MaxTokens   int     `json:"max_tokens,omitempty"`
	}{Model: provider.model, Temperature: request.Temperature, MaxTokens: request.MaxTokens}
	if request.SystemPrompt != "" {
		payload.Messages = append(payload.Messages, struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{Role: "system", Content: request.SystemPrompt})
	}
	payload.Messages = append(payload.Messages, struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{Role: "user", Content: request.UserPrompt})

	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, fmt.Errorf("encode LLM request: %w", err)
	}
	endpoint := *provider.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/chat/completions"
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("create LLM request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	if provider.apiKey != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+provider.apiKey)
	}
	response, err := provider.httpClient.Do(httpRequest)
	if err != nil {
		return Response{}, fmt.Errorf("LLM request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Response{}, fmt.Errorf("LLM provider returned HTTP %d", response.StatusCode)
	}
	var result struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&result); err != nil {
		return Response{}, fmt.Errorf("decode LLM response: %w", err)
	}
	if len(result.Choices) == 0 {
		return Response{}, fmt.Errorf("LLM provider returned no choices")
	}
	return Response{
		Text:        result.Choices[0].Message.Content,
		Model:       result.Model,
		UsageInput:  result.Usage.PromptTokens,
		UsageOutput: result.Usage.CompletionTokens,
	}, nil
}

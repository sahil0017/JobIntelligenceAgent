package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Request struct {
	SystemPrompt string
	UserPrompt   string
	Temperature  float64
	MaxTokens    int
}

type Response struct {
	Text        string
	Model       string
	UsageInput  int
	UsageOutput int
}

type Provider interface {
	Name() string
	Generate(context.Context, Request) (Response, error)
}

type OpenAICompatible struct {
	baseURL    *url.URL
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewOpenAICompatible(baseURL, apiKey, model string, client *http.Client) (*OpenAICompatible, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, fmt.Errorf("LLM base URL must be an absolute HTTP(S) URL")
	}
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("LLM model name is required")
	}
	if client == nil {
		client = &http.Client{Timeout: 45 * time.Second}
	}
	return &OpenAICompatible{baseURL: parsed, apiKey: apiKey, model: model, httpClient: client}, nil
}

func (provider *OpenAICompatible) Name() string {
	return "openai-compatible"
}

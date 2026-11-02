package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAICompatibleProviderUsesConfiguredEndpointAndReturnsText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-secret" {
			t.Errorf("authorization header was not forwarded")
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["model"] != "local-model" {
			t.Errorf("unexpected model: %v", payload["model"])
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"model":"local-model","choices":[{"message":{"content":"Hello"}}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatible(server.URL+"/v1", "test-secret", "local-model", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	response, err := provider.Generate(context.Background(), Request{UserPrompt: "Say hello"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "Hello" || response.UsageInput != 3 || response.UsageOutput != 2 {
		t.Fatalf("unexpected LLM response: %#v", response)
	}
}

func TestOpenAICompatibleProviderRejectsInvalidBaseURL(t *testing.T) {
	if _, err := NewOpenAICompatible("file:///etc/passwd", "", "model", nil); err == nil {
		t.Fatal("expected invalid base URL to be rejected")
	}
}

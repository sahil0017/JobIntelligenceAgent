package runs

import (
	"bytes"
	"careerpilot/contracts/domain"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxReviewBody = 1 << 20

// OllamaReviewer summarizes matches with a local Ollama model. It sends only the
// candidate's title and skills plus job summaries; resume text is never sent.
type OllamaReviewer struct {
	httpClient *http.Client
}

func NewOllamaReviewer() *OllamaReviewer {
	return &OllamaReviewer{httpClient: &http.Client{Timeout: 90 * time.Second}}
}

func ValidateOllamaURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return fmt.Errorf("Ollama URL must be an absolute HTTP(S) URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("Ollama URL must not contain credentials, query parameters, or a fragment")
	}
	return nil
}

func (reviewer *OllamaReviewer) Review(ctx context.Context, config domain.AgentConfig, profile domain.SearchProfile, jobs []domain.SearchJob) (string, error) {
	if err := ValidateOllamaURL(config.OllamaURL); err != nil {
		return "", err
	}
	if strings.TrimSpace(config.Model) == "" {
		return "", fmt.Errorf("Ollama model name is required")
	}
	var listing strings.Builder
	for i, job := range jobs {
		fmt.Fprintf(&listing, "%d. %s at %s (%s), match %d%%, skills: %s\n", i+1, job.Role, job.Company, job.Location, job.Score, strings.Join(job.Tags, ", "))
	}
	body, err := json.Marshal(map[string]any{
		"model":  config.Model,
		"stream": false,
		"messages": []map[string]string{
			{"role": "system", "content": "You are CareerPilot's job-match reviewer. Use only the jobs provided; never invent listings or claim any application was sent. Reply in under 120 words: name the best two matches and say why, then note one gap to check."},
			{"role": "user", "content": fmt.Sprintf("Candidate role: %s\nCandidate skills: %s\n\nVerified jobs:\n%s", profile.Title, profile.Skills, listing.String())},
		},
	})
	if err != nil {
		return "", fmt.Errorf("encode review request: %w", err)
	}
	endpoint := strings.TrimRight(strings.TrimSpace(config.OllamaURL), "/") + "/api/chat"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create review request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := reviewer.httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("reach Ollama: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxReviewBody+1))
	if err != nil {
		return "", fmt.Errorf("read Ollama response: %w", err)
	}
	if len(data) > maxReviewBody {
		return "", fmt.Errorf("Ollama response is too large")
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Ollama returned HTTP %d", response.StatusCode)
	}
	var decoded struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return "", fmt.Errorf("decode Ollama response: %w", err)
	}
	text := strings.TrimSpace(decoded.Message.Content)
	if text == "" {
		return "", fmt.Errorf("Ollama returned an empty review")
	}
	return text, nil
}

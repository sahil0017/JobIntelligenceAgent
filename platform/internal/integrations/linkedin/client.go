package linkedin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	apiBaseURL       = "https://api.linkedin.com"
	maxResponseBytes = 2 << 20
)

type Client struct {
	httpClient  *http.Client
	accessToken string
	apiVersion  string
}

func NewClient(accessToken, apiVersion string, httpClient *http.Client) (*Client, error) {
	if strings.TrimSpace(accessToken) == "" {
		return nil, fmt.Errorf("LinkedIn access token is required")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{
		httpClient:  httpClient,
		accessToken: accessToken,
		apiVersion:  strings.TrimSpace(apiVersion),
	}, nil
}

func (client *Client) getJSON(ctx context.Context, endpoint string, query url.Values, target any) error {
	if !strings.HasPrefix(endpoint, "/") || strings.HasPrefix(endpoint, "//") {
		return fmt.Errorf("LinkedIn API endpoint must be a relative path")
	}
	parsed, err := url.Parse(apiBaseURL + endpoint)
	if err != nil || parsed.Host != "api.linkedin.com" {
		return fmt.Errorf("invalid LinkedIn API endpoint")
	}
	if query != nil {
		parsed.RawQuery = query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return fmt.Errorf("create LinkedIn request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+client.accessToken)
	request.Header.Set("Accept", "application/json")
	if client.apiVersion != "" {
		request.Header.Set("LinkedIn-Version", client.apiVersion)
	}

	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("LinkedIn API request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("LinkedIn API returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read LinkedIn API response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return fmt.Errorf("LinkedIn API response exceeds the %d MiB size limit", maxResponseBytes>>20)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode LinkedIn API response: %w", err)
	}
	return nil
}

package gmail

import (
	"context"
	"fmt"
	"sync"

	"careerpilot/platform/internal/storage"
)

type Integration struct {
	mu    sync.RWMutex
	oauth *OAuth
}

func New(config OAuthConfig) (*Integration, error) {
	oauth, err := NewOAuth(config)
	if err != nil {
		return nil, err
	}
	return &Integration{oauth: oauth}, nil
}

func NewUnconfigured() *Integration {
	return &Integration{}
}

func (integration *Integration) Configure(config storage.IntegrationConfig) error {
	if config.ClientID == "" && config.ClientSecret == "" && config.RedirectURL == "" {
		integration.mu.Lock()
		integration.oauth = nil
		integration.mu.Unlock()
		return nil
	}
	oauth, err := NewOAuth(OAuthConfig{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RedirectURL:  config.RedirectURL,
	})
	if err != nil {
		return err
	}
	integration.mu.Lock()
	integration.oauth = oauth
	integration.mu.Unlock()
	return nil
}

func (integration *Integration) Configured() bool {
	integration.mu.RLock()
	defer integration.mu.RUnlock()
	return integration.oauth != nil
}

func (integration *Integration) StartAuthorization() (string, error) {
	oauth := integration.currentOAuth()
	if oauth == nil {
		return "", fmt.Errorf("Gmail OAuth is not configured")
	}
	return oauth.Begin()
}

func (integration *Integration) CompleteAuthorization(ctx context.Context, state, code string) error {
	oauth := integration.currentOAuth()
	if oauth == nil {
		return fmt.Errorf("Gmail OAuth is not configured")
	}
	return oauth.Complete(ctx, state, code)
}

func (integration *Integration) Connected() bool {
	oauth := integration.currentOAuth()
	if oauth == nil {
		return false
	}
	_, connected := oauth.TokenSource()
	return connected
}

func (integration *Integration) Disconnect() {
	if oauth := integration.currentOAuth(); oauth != nil {
		oauth.Disconnect()
	}
}

func (integration *Integration) SearchThreads(ctx context.Context, query string, limit int) ([]ThreadSummary, error) {
	oauth := integration.currentOAuth()
	if oauth == nil {
		return nil, fmt.Errorf("Gmail OAuth is not configured")
	}
	client, err := oauth.AuthenticatedClient(ctx)
	if err != nil {
		return nil, err
	}
	return SearchThreads(ctx, client, query, limit)
}

func (integration *Integration) SendMessage(ctx context.Context, recipient, subject, body string) (string, error) {
	oauth := integration.currentOAuth()
	if oauth == nil {
		return "", fmt.Errorf("Gmail OAuth is not configured")
	}
	client, err := oauth.AuthenticatedClient(ctx)
	if err != nil {
		return "", err
	}
	return SendMessage(ctx, client, recipient, subject, body)
}

func (integration *Integration) EmailAddress(ctx context.Context) (string, error) {
	oauth := integration.currentOAuth()
	if oauth == nil {
		return "", fmt.Errorf("Gmail OAuth is not configured")
	}
	client, err := oauth.AuthenticatedClient(ctx)
	if err != nil {
		return "", err
	}
	return GmailAddress(ctx, client)
}

func (integration *Integration) currentOAuth() *OAuth {
	integration.mu.RLock()
	defer integration.mu.RUnlock()
	return integration.oauth
}

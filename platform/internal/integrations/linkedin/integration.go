package linkedin

import (
	"context"
	"fmt"
	"sync"
	"time"

	"careerpilot/platform/internal/storage"
)

const authorizationStateTTL = 10 * time.Minute

type Manager struct {
	mu      sync.Mutex
	oauth   *OAuth
	token   Token
	profile Profile
	pending map[string]time.Time
}

func NewManager(config storage.IntegrationConfig) (*Manager, error) {
	manager := &Manager{pending: make(map[string]time.Time)}
	if config.ClientID != "" || config.ClientSecret != "" || config.RedirectURL != "" {
		if err := manager.Configure(config); err != nil {
			return nil, err
		}
	}
	return manager, nil
}

func (manager *Manager) Configure(config storage.IntegrationConfig) error {
	if config.ClientID == "" && config.ClientSecret == "" && config.RedirectURL == "" {
		manager.mu.Lock()
		manager.oauth = nil
		manager.token = Token{}
		manager.profile = Profile{}
		manager.pending = make(map[string]time.Time)
		manager.mu.Unlock()
		return nil
	}
	oauth, err := NewOAuth(OAuthConfig{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RedirectURL:  config.RedirectURL,
		Scopes:       []string{"openid", "profile", "email"},
	})
	if err != nil {
		return err
	}
	manager.mu.Lock()
	manager.oauth = oauth
	manager.token = Token{}
	manager.profile = Profile{}
	manager.pending = make(map[string]time.Time)
	manager.mu.Unlock()
	return nil
}

func (manager *Manager) Configured() bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.oauth != nil
}

func (manager *Manager) StartAuthorization() (string, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.oauth == nil {
		return "", fmt.Errorf("LinkedIn OAuth is not configured")
	}
	now := time.Now()
	for state, expiresAt := range manager.pending {
		if !expiresAt.After(now) {
			delete(manager.pending, state)
		}
	}
	state, err := NewState()
	if err != nil {
		return "", err
	}
	manager.pending[state] = now.Add(authorizationStateTTL)
	return manager.oauth.AuthorizationURL(state)
}

func (manager *Manager) CompleteAuthorization(ctx context.Context, state, code string) (Profile, error) {
	manager.mu.Lock()
	expiresAt, ok := manager.pending[state]
	delete(manager.pending, state)
	oauth := manager.oauth
	manager.mu.Unlock()
	if !ok || !expiresAt.After(time.Now()) {
		return Profile{}, fmt.Errorf("LinkedIn OAuth state is invalid, expired, or already used")
	}
	if oauth == nil {
		return Profile{}, fmt.Errorf("LinkedIn OAuth is not configured")
	}
	token, err := oauth.ExchangeCode(ctx, code)
	if err != nil {
		return Profile{}, err
	}
	client, err := NewClient(token.AccessToken, "", nil)
	if err != nil {
		return Profile{}, err
	}
	profile, err := client.GetProfile(ctx)
	if err != nil {
		return Profile{}, err
	}
	manager.mu.Lock()
	manager.token = token
	manager.profile = profile
	manager.mu.Unlock()
	return profile, nil
}

func (manager *Manager) Connected() bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.token.AccessToken != "" &&
		(manager.token.ExpiresAt.IsZero() || manager.token.ExpiresAt.After(time.Now()))
}

func (manager *Manager) Profile() (Profile, bool) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	connected := manager.token.AccessToken != "" &&
		(manager.token.ExpiresAt.IsZero() || manager.token.ExpiresAt.After(time.Now()))
	return manager.profile, connected
}

func (manager *Manager) Disconnect() {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.token = Token{}
	manager.profile = Profile{}
}

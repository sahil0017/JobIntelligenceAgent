package gmail

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	maxPendingAuthorizations = 20
	authorizationTTL         = 10 * time.Minute
)

type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	HTTPClient   *http.Client
	Endpoint     oauth2.Endpoint
}

type pendingAuthorization struct {
	verifier  string
	expiresAt time.Time
}

type OAuth struct {
	config *oauth2.Config
	client *http.Client
	now    func() time.Time

	mu      sync.Mutex
	pending map[string]pendingAuthorization
	token   oauth2.TokenSource
}

func NewOAuth(config OAuthConfig) (*OAuth, error) {
	config.ClientID = strings.TrimSpace(config.ClientID)
	config.ClientSecret = strings.TrimSpace(config.ClientSecret)
	config.RedirectURL = strings.TrimSpace(config.RedirectURL)
	if config.ClientID == "" || config.ClientSecret == "" {
		return nil, fmt.Errorf("Gmail OAuth client ID and client secret are required")
	}
	if err := validateRedirectURL(config.RedirectURL); err != nil {
		return nil, err
	}
	if config.Endpoint.AuthURL == "" && config.Endpoint.TokenURL == "" {
		config.Endpoint = google.Endpoint
	} else if config.Endpoint.AuthURL == "" || config.Endpoint.TokenURL == "" {
		return nil, fmt.Errorf("Gmail OAuth authorization and token endpoints are both required")
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &OAuth{
		config: &oauth2.Config{
			ClientID:     config.ClientID,
			ClientSecret: config.ClientSecret,
			RedirectURL:  config.RedirectURL,
			Scopes: []string{
				"https://www.googleapis.com/auth/gmail.readonly",
				"https://www.googleapis.com/auth/gmail.send",
			},
			Endpoint: config.Endpoint,
		},
		client:  config.HTTPClient,
		now:     time.Now,
		pending: make(map[string]pendingAuthorization),
	}, nil
}

func validateRedirectURL(raw string) error {
	redirectURL, err := url.ParseRequestURI(raw)
	if err != nil || redirectURL.Host == "" || redirectURL.User != nil || redirectURL.Fragment != "" {
		return fmt.Errorf("Gmail OAuth redirect URL must be absolute and contain no credentials or fragment")
	}
	if redirectURL.Scheme == "https" {
		return nil
	}
	if redirectURL.Scheme != "http" {
		return fmt.Errorf("Gmail OAuth redirect URL must use HTTPS or localhost HTTP")
	}
	host := redirectURL.Hostname()
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("Gmail OAuth redirect URL must use HTTPS or localhost HTTP")
	}
	return nil
}

func (oauth *OAuth) Begin() (string, error) {
	state, err := randomURLSafe(32)
	if err != nil {
		return "", fmt.Errorf("generate Gmail OAuth state: %w", err)
	}
	verifier, err := randomURLSafe(32)
	if err != nil {
		return "", fmt.Errorf("generate Gmail OAuth PKCE verifier: %w", err)
	}

	oauth.mu.Lock()
	defer oauth.mu.Unlock()
	now := oauth.now()
	for key, pending := range oauth.pending {
		if !pending.expiresAt.After(now) {
			delete(oauth.pending, key)
		}
	}
	if len(oauth.pending) >= maxPendingAuthorizations {
		return "", fmt.Errorf("too many pending Gmail authorizations; try again shortly")
	}
	oauth.pending[state] = pendingAuthorization{
		verifier:  verifier,
		expiresAt: now.Add(authorizationTTL),
	}
	return oauth.config.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("include_granted_scopes", "true"),
		oauth2.SetAuthURLParam("prompt", "consent"),
	), nil
}

func (oauth *OAuth) Complete(ctx context.Context, state, code string) error {
	state = strings.TrimSpace(state)
	code = strings.TrimSpace(code)
	if state == "" || code == "" {
		return fmt.Errorf("Gmail OAuth state and authorization code are required")
	}
	pending, ok := oauth.consumeState(state)
	if !ok {
		return fmt.Errorf("Gmail OAuth state is invalid, expired, or already used")
	}

	exchangeContext := context.WithValue(ctx, oauth2.HTTPClient, oauth.client)
	token, err := oauth.config.Exchange(exchangeContext, code, oauth2.VerifierOption(pending.verifier))
	if err != nil {
		return fmt.Errorf("exchange Gmail authorization code: %w", err)
	}
	if token.AccessToken == "" {
		return fmt.Errorf("Gmail OAuth response did not contain an access token")
	}
	tokenContext := context.WithValue(context.Background(), oauth2.HTTPClient, oauth.client)
	tokenSource := oauth2.ReuseTokenSource(token, oauth.config.TokenSource(tokenContext, token))
	oauth.mu.Lock()
	oauth.token = tokenSource
	oauth.mu.Unlock()
	return nil
}

func (oauth *OAuth) consumeState(state string) (pendingAuthorization, bool) {
	oauth.mu.Lock()
	defer oauth.mu.Unlock()
	pending, ok := oauth.pending[state]
	delete(oauth.pending, state)
	if !ok || !pending.expiresAt.After(oauth.now()) {
		return pendingAuthorization{}, false
	}
	return pending, true
}

func (oauth *OAuth) TokenSource() (oauth2.TokenSource, bool) {
	oauth.mu.Lock()
	defer oauth.mu.Unlock()
	return oauth.token, oauth.token != nil
}

func (oauth *OAuth) AuthenticatedClient(ctx context.Context) (*http.Client, error) {
	tokenSource, connected := oauth.TokenSource()
	if !connected {
		return nil, fmt.Errorf("Gmail is not connected")
	}
	client := *oauth.client
	baseTransport := client.Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}
	client.Transport = &oauth2.Transport{Source: tokenSource, Base: baseTransport}
	return &client, nil
}

func (oauth *OAuth) Disconnect() {
	oauth.mu.Lock()
	defer oauth.mu.Unlock()
	oauth.token = nil
}

func randomURLSafe(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

package linkedin

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	authorizationURL = "https://www.linkedin.com/oauth/v2/authorization"
	tokenURL         = "https://www.linkedin.com/oauth/v2/accessToken"
)

type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
	HTTPClient   *http.Client
}

type OAuth struct {
	config     OAuthConfig
	httpClient *http.Client
}

type Token struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiresAt    time.Time
}

func NewOAuth(config OAuthConfig) (*OAuth, error) {
	config.ClientID = strings.TrimSpace(config.ClientID)
	config.ClientSecret = strings.TrimSpace(config.ClientSecret)
	config.RedirectURL = strings.TrimSpace(config.RedirectURL)
	if config.ClientID == "" || config.ClientSecret == "" {
		return nil, fmt.Errorf("LinkedIn client ID and client secret are required")
	}
	if len(config.Scopes) == 0 {
		config.Scopes = []string{"openid", "profile", "email"}
	}
	redirectURL, err := url.ParseRequestURI(config.RedirectURL)
	if err != nil || redirectURL.Host == "" ||
		(redirectURL.Scheme != "https" && !(redirectURL.Scheme == "http" && isLoopbackHost(redirectURL.Hostname()))) {
		return nil, fmt.Errorf("redirect URL must use HTTPS or localhost HTTP")
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &OAuth{config: config, httpClient: config.HTTPClient}, nil
}

func (oauth *OAuth) AuthorizationURL(state string) (string, error) {
	if len(strings.TrimSpace(state)) < 32 {
		return "", fmt.Errorf("OAuth state must contain at least 32 characters")
	}
	values := url.Values{
		"response_type": {"code"},
		"client_id":     {oauth.config.ClientID},
		"redirect_uri":  {oauth.config.RedirectURL},
		"state":         {state},
		"scope":         {strings.Join(oauth.config.Scopes, " ")},
	}
	return authorizationURL + "?" + values.Encode(), nil
}

func NewState() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate OAuth state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (oauth *OAuth) ExchangeCode(ctx context.Context, code string) (Token, error) {
	if strings.TrimSpace(code) == "" {
		return Token{}, fmt.Errorf("OAuth authorization code is required")
	}
	return oauth.requestToken(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {oauth.config.RedirectURL},
		"client_id":     {oauth.config.ClientID},
		"client_secret": {oauth.config.ClientSecret},
	})
}

func (oauth *OAuth) RefreshToken(ctx context.Context, refreshToken string) (Token, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return Token{}, fmt.Errorf("OAuth refresh token is required")
	}
	return oauth.requestToken(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {oauth.config.ClientID},
		"client_secret": {oauth.config.ClientSecret},
	})
}

func (oauth *OAuth) requestToken(ctx context.Context, values url.Values) (Token, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return Token{}, fmt.Errorf("create LinkedIn token request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response, err := oauth.httpClient.Do(request)
	if err != nil {
		return Token{}, fmt.Errorf("LinkedIn token request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return Token{}, fmt.Errorf("LinkedIn token endpoint returned HTTP %d", response.StatusCode)
	}
	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return Token{}, fmt.Errorf("decode LinkedIn token response: %w", err)
	}
	if result.AccessToken == "" {
		return Token{}, fmt.Errorf("LinkedIn token response did not contain an access token")
	}
	return Token{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		TokenType:    result.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(result.ExpiresIn) * time.Second),
	}, nil
}

func isLoopbackHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

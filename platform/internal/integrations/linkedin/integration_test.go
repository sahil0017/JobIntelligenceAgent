package linkedin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"careerpilot/contracts/domain"
)

func TestAuthorizationURLRequiresStateAndEncodesRequestedScopes(t *testing.T) {
	oauth, err := NewOAuth(OAuthConfig{
		ClientID: "client-id", ClientSecret: "client-secret",
		RedirectURL: "http://localhost:3000/callback",
		Scopes:      []string{"openid", "profile", "email"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := oauth.AuthorizationURL("short"); err == nil {
		t.Fatal("expected short OAuth state to fail")
	}
	authURL, err := oauth.AuthorizationURL(strings.Repeat("x", 32))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(authURL, "scope=openid+profile+email") {
		t.Fatalf("OAuth URL did not include requested scopes: %s", authURL)
	}
	state, err := NewState()
	if err != nil || len(state) < 32 {
		t.Fatalf("generated OAuth state is invalid: %q, %v", state, err)
	}
}

func TestLinkedInProfileUsesOfficialUserInfoEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v2/userinfo" || request.Header.Get("Authorization") != "Bearer access-token" {
			t.Errorf("unexpected LinkedIn request: %s %s", request.Method, request.URL)
		}
		_, _ = writer.Write([]byte(`{"sub":"member-1","given_name":"Sahil","family_name":"Kanungo","email":"sahil@example.com"}`))
	}))
	defer server.Close()

	client, err := NewClient("access-token", "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.httpClient.Transport = rewriteHostTransport{base: server.URL}
	profile, err := client.GetProfile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if profile.ID != "member-1" || profile.Name != "Sahil Kanungo" {
		t.Fatalf("unexpected profile: %#v", profile)
	}
}

func TestOAuthExchangesAuthorizationCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/oauth/v2/accessToken" {
			t.Errorf("unexpected LinkedIn token request: %s %s", request.Method, request.URL)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if request.Form.Get("grant_type") != "authorization_code" || request.Form.Get("code") != "auth-code" {
			t.Errorf("authorization code was not exchanged: %v", request.Form)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"access_token":"issued-token","token_type":"Bearer","expires_in":3600}`))
	}))
	defer server.Close()

	oauth, err := NewOAuth(OAuthConfig{
		ClientID: "client-id", ClientSecret: "client-secret",
		RedirectURL: "http://localhost:3000/callback",
		HTTPClient:  &http.Client{Transport: rewriteHostTransport{base: server.URL}},
	})
	if err != nil {
		t.Fatal(err)
	}
	token, err := oauth.ExchangeCode(context.Background(), "auth-code")
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "issued-token" || token.ExpiresAt.IsZero() {
		t.Fatalf("unexpected OAuth token: %#v", token)
	}
}

func TestJobsRequireApprovedEndpointAndMapper(t *testing.T) {
	client, err := NewClient("access-token", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewJobsClient(client, "", testJobMapper{}); !errors.Is(err, ErrJobsProductAccessRequired) {
		t.Fatalf("expected unconfigured jobs API to remain unavailable, got %v", err)
	}
	if _, err := NewJobsClient(client, "https://example.com/jobs", testJobMapper{}); err == nil {
		t.Fatal("absolute API URL should be rejected")
	}
}

type testJobMapper struct{}

func (testJobMapper) MapJobs(payload json.RawMessage) ([]domain.Job, error) {
	return []domain.Job{}, nil
}

type rewriteHostTransport struct {
	base string
}

func (transport rewriteHostTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	cloned := request.Clone(request.Context())
	target, err := request.URL.Parse(transport.base + request.URL.RequestURI())
	if err != nil {
		return nil, err
	}
	cloned.URL = target
	cloned.Host = target.Host
	return http.DefaultTransport.RoundTrip(cloned)
}

package gmail

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestOAuthAndThreadSearchUseReadOnlyScopeAndPKCE(t *testing.T) {
	var authorizationHeader string
	var verifier string
	var sentMessage string
	plainBody := base64.RawURLEncoding.EncodeToString([]byte("We are hiring a Backend Engineer at Example Corp."))
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch {
		case request.URL.Path == "/token":
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}
			verifier = request.Form.Get("code_verifier")
			return testResponse(http.StatusOK, `{"access_token":"test-access","token_type":"Bearer","expires_in":3600}`), nil
		case strings.HasSuffix(request.URL.Path, "/threads"):
			authorizationHeader = request.Header.Get("Authorization")
			if request.URL.Query().Get("q") != "from:recruiter@example.com" {
				t.Errorf("unexpected Gmail query: %q", request.URL.Query().Get("q"))
			}
			if request.URL.Query().Get("maxResults") != "2" {
				t.Errorf("unexpected result limit: %q", request.URL.Query().Get("maxResults"))
			}
			return testResponse(http.StatusOK, `{"threads":[{"id":"thread-1"}]}`), nil
		case strings.HasSuffix(request.URL.Path, "/threads/thread-1"):
			if request.URL.Query().Get("format") != "full" {
				t.Errorf("Gmail thread retrieval requested non-full format: %q", request.URL.Query().Get("format"))
			}
			return testResponse(http.StatusOK, `{
				"id":"thread-1",
				"snippet":"Please schedule an interview.",
				"messages":[{
					"id":"message-1",
					"snippet":"Please schedule an interview.",
					"payload":{"mimeType":"text/plain","body":{"data":"`+plainBody+`"},"headers":[
						{"name":"From","value":"Recruiter <recruiter@example.com>"},
						{"name":"Subject","value":"Interview invitation"},
						{"name":"Date","value":"Tue, 01 Jan 2030 12:00:00 +0000"}
					]}
				}]
			}`), nil
		case strings.HasSuffix(request.URL.Path, "/messages/send"):
			var message struct {
				Raw string `json:"raw"`
			}
			if err := json.NewDecoder(request.Body).Decode(&message); err != nil {
				t.Fatal(err)
			}
			decoded, err := base64.RawURLEncoding.DecodeString(message.Raw)
			if err != nil {
				t.Fatal(err)
			}
			sentMessage = string(decoded)
			return testResponse(http.StatusOK, `{"id":"sent-message-1"}`), nil
		default:
			t.Errorf("unexpected request URL: %s", request.URL)
			return testResponse(http.StatusNotFound, `{}`), nil
		}
	})}
	integration, err := New(OAuthConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURL:  "http://localhost:8787/api/gmail/oauth/callback",
		HTTPClient:   httpClient,
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://accounts.example.test/authorize",
			TokenURL: "https://accounts.example.test/token",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	authorizationURL, err := integration.StartAuthorization()
	if err != nil {
		t.Fatal(err)
	}
	parsedAuthorizationURL, err := url.Parse(authorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsedAuthorizationURL.Query()
	if !strings.Contains(query.Get("scope"), "https://www.googleapis.com/auth/gmail.readonly") ||
		!strings.Contains(query.Get("scope"), "https://www.googleapis.com/auth/gmail.send") {
		t.Fatalf("unexpected OAuth scope: %q", query.Get("scope"))
	}
	if query.Get("code_challenge_method") != "S256" {
		t.Fatalf("expected PKCE S256, got %q", query.Get("code_challenge_method"))
	}
	state := query.Get("state")
	if err := integration.CompleteAuthorization(context.Background(), state, "auth-code"); err != nil {
		t.Fatal(err)
	}
	if !integration.Connected() {
		t.Fatal("Gmail should be connected after exchanging the code")
	}
	if verifier == "" {
		t.Fatal("OAuth exchange did not include a PKCE verifier")
	}
	hash := sha256.Sum256([]byte(verifier))
	wantChallenge := base64.RawURLEncoding.EncodeToString(hash[:])
	if query.Get("code_challenge") != wantChallenge {
		t.Fatal("authorization challenge does not match the verifier")
	}

	threads, err := integration.SearchThreads(context.Background(), "from:recruiter@example.com", 2)
	if err != nil {
		t.Fatal(err)
	}
	if authorizationHeader != "Bearer test-access" {
		t.Fatalf("Gmail API received unexpected authorization: %q", authorizationHeader)
	}
	if len(threads) != 1 || threads[0].ID != "thread-1" || len(threads[0].Messages) != 1 {
		t.Fatalf("unexpected thread results: %#v", threads)
	}
	message := threads[0].Messages[0]
	if message.From != "Recruiter <recruiter@example.com>" || message.Subject != "Interview invitation" ||
		message.Body != "We are hiring a Backend Engineer at Example Corp." {
		t.Fatalf("message metadata was not mapped: %#v", message)
	}
	messageID, err := integration.SendMessage(context.Background(), "hiring@example.com", "Interested in Backend Engineer", "Hello, I am interested.")
	if err != nil || messageID != "sent-message-1" {
		t.Fatalf("Gmail send failed: message ID %q, error %v", messageID, err)
	}
	if !strings.Contains(sentMessage, "To: hiring@example.com\r\n") ||
		!strings.Contains(sentMessage, "Subject: Interested in Backend Engineer\r\n") ||
		!strings.Contains(sentMessage, "\r\n\r\nHello, I am interested.") {
		t.Fatalf("Gmail send payload was not formatted correctly: %q", sentMessage)
	}

	integration.Disconnect()
	if integration.Connected() {
		t.Fatal("Gmail should be disconnected")
	}
	if _, err := integration.SearchThreads(context.Background(), "", 1); err == nil {
		t.Fatal("search should fail after disconnecting")
	}
}

func TestSendMessageRejectsInvalidInputs(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid message must not reach Gmail")
		return nil, nil
	})}
	for _, testCase := range []struct {
		name, recipient, subject, body string
	}{
		{"invalid recipient", "not-an-email", "Subject", "Body"},
		{"header injection", "hiring@example.com\r\nBcc: other@example.com", "Subject", "Body"},
		{"subject newline", "hiring@example.com", "Subject\r\nBcc: other@example.com", "Body"},
		{"empty body", "hiring@example.com", "Subject", " "},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := SendMessage(context.Background(), client, testCase.recipient, testCase.subject, testCase.body); err == nil {
				t.Fatal("expected invalid message to be rejected")
			}
		})
	}
}

func TestOAuthRejectsInvalidOrReusedState(t *testing.T) {
	integration, err := New(OAuthConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURL:  "http://localhost:8787/api/gmail/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := integration.CompleteAuthorization(context.Background(), "not-a-state", "code"); err == nil {
		t.Fatal("expected an unknown OAuth state to be rejected")
	}
	authorizationURL, err := integration.StartAuthorization()
	if err != nil {
		t.Fatal(err)
	}
	query, err := url.Parse(authorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	state := query.Query().Get("state")
	if err := integration.CompleteAuthorization(context.Background(), state, ""); err == nil {
		t.Fatal("expected an empty authorization code to be rejected")
	}
	if err := integration.CompleteAuthorization(context.Background(), state, "code"); err == nil {
		t.Fatal("expected an OAuth state to be single-use")
	}
}

func TestNewOAuthRejectsNonLocalHTTPRedirect(t *testing.T) {
	_, err := NewOAuth(OAuthConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURL:  "http://example.com/callback",
	})
	if err == nil {
		t.Fatal("expected non-local HTTP redirect to be rejected")
	}
}

func testResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

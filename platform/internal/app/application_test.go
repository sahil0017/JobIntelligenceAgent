package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewHandlerLeavesUnconfiguredGmailDisabled(t *testing.T) {
	t.Setenv("GMAIL_OAUTH_CLIENT_ID", "")
	t.Setenv("GMAIL_OAUTH_CLIENT_SECRET", "")
	t.Setenv("GMAIL_OAUTH_REDIRECT_URL", "")
	handler, err := NewHandler(filepath.Join(t.TempDir(), "sources.json"))
	if err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/gmail/status", nil))
	if response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), `"configured":false`) ||
		!strings.Contains(response.Body.String(), `"connected":false`) {
		t.Fatalf("unexpected Gmail status: %d %s", response.Code, response.Body)
	}
}

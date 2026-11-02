package cortex

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientPassesResponsesThroughAndSendsJSON(t *testing.T) {
	var gotBody, gotType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody, gotType = string(body), r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"id":"abc","status":"running"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	run, err := client.Start(context.Background(), AnalysisRequest{
		Profile: Profile{Skills: "Go"},
		Jobs:    []Job{{ID: "1", Role: "Engineer"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(run) != `{"id":"abc","status":"running"}` {
		t.Fatalf("response was not passed through verbatim: %s", run)
	}
	if gotType != "application/json" || !strings.Contains(gotBody, `"resumeText"`) || !strings.Contains(gotBody, `"skills":"Go"`) {
		t.Fatalf("unexpected request: %s (%s)", gotBody, gotType)
	}
}

func TestClientMapsNotFoundAndUnavailable(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	client, _ := NewClient(server.URL, nil)
	if _, err := client.Get(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	server.Close()
	if _, err := client.Health(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want ErrUnavailable once the service is down, got %v", err)
	}
}

func TestNewClientRejectsNonHTTPURLs(t *testing.T) {
	for _, bad := range []string{"file:///etc/passwd", "ftp://host", "not a url", "http://"} {
		if _, err := NewClient(bad, nil); err == nil {
			t.Errorf("NewClient(%q) should fail", bad)
		}
	}
}

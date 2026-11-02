package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"careerpilot/intelligence/runs"
	"careerpilot/intelligence/tools"
	"careerpilot/platform/internal/integrations/webjobs"
	"careerpilot/platform/internal/storage"
)

type fakeAgentSearcher struct{ params webjobs.Profile }

func (searcher *fakeAgentSearcher) SearchWithOptions(_ context.Context, _, _, _, _ string, profile webjobs.Profile, _ webjobs.Options) (webjobs.Result, error) {
	searcher.params = profile
	return webjobs.Result{Jobs: []webjobs.JobResult{{Company: "Acme", Score: 80}}}, nil
}

type noReview struct{}

func (noReview) Review(context.Context, storage.AgentConfig, webjobs.Profile, []webjobs.JobResult) (string, error) {
	return "", nil
}

func newAgentAPI(t *testing.T) (*API, *fakeAgentSearcher) {
	t.Helper()
	store, err := NewSourceStore(filepath.Join(t.TempDir(), "careerpilot.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Database().Close() })
	searcher := &fakeAgentSearcher{}
	return New(store, tools.NewRegistry(), Services{AgentRuns: runs.NewManager(searcher, noReview{})}), searcher
}

func call(api *API, method, path, body string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, httptest.NewRequest(method, path, strings.NewReader(body)))
	return response
}

func TestAgentConfigDefaultsSaveAndValidation(t *testing.T) {
	api, _ := newAgentAPI(t)
	if response := call(api, http.MethodGet, "/api/agent/config", ""); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"maxRounds":4`) {
		t.Fatalf("expected default config, got %d %s", response.Code, response.Body)
	}
	valid := `{"model":"llama3.1:8b","ollamaUrl":"http://127.0.0.1:11434","maxRounds":3,"targetMatches":5,"maxPages":20,"minScore":50,"llmReview":false}`
	if response := call(api, http.MethodPut, "/api/agent/config", valid); response.Code != http.StatusOK {
		t.Fatalf("valid config rejected: %d %s", response.Code, response.Body)
	}
	if response := call(api, http.MethodGet, "/api/agent/config", ""); !strings.Contains(response.Body.String(), `"model":"llama3.1:8b"`) {
		t.Fatalf("config was not persisted: %s", response.Body)
	}
	for _, bad := range []string{
		`{"model":"","ollamaUrl":"http://x","maxRounds":3,"targetMatches":5,"maxPages":20,"minScore":50}`,
		`{"model":"m","ollamaUrl":"ftp://x","maxRounds":3,"targetMatches":5,"maxPages":20,"minScore":50}`,
		`{"model":"m","ollamaUrl":"http://x","maxRounds":99,"targetMatches":5,"maxPages":20,"minScore":50}`,
		`{"model":"m","ollamaUrl":"http://x","maxRounds":3,"targetMatches":5,"maxPages":2,"minScore":50}`,
	} {
		if response := call(api, http.MethodPut, "/api/agent/config", bad); response.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for %s, got %d", bad, response.Code)
		}
	}
}

func TestAgentRunRequiresKeyThenRunsWithSavedProfile(t *testing.T) {
	api, searcher := newAgentAPI(t)
	body := `{"country":"India","state":"Karnataka","area":"Bengaluru"}`
	if response := call(api, http.MethodPost, "/api/agent/runs", body); response.Code != http.StatusPreconditionRequired {
		t.Fatalf("expected 428 without a Brave key, got %d %s", response.Code, response.Body)
	}
	if err := api.sources.Database().SaveProfile(storage.Profile{Title: "Go Engineer", Skills: "Go"}); err != nil {
		t.Fatal(err)
	}
	call(api, http.MethodPut, "/api/settings/web-search", `{"apiKey":"private-brave-key"}`)
	if response := call(api, http.MethodPost, "/api/agent/runs", `{"country":"","area":""}`); response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing location, got %d", response.Code)
	}
	started := call(api, http.MethodPost, "/api/agent/runs", body)
	if started.Code != http.StatusAccepted || strings.Contains(started.Body.String(), "private-brave-key") {
		t.Fatalf("unexpected start response: %d %s", started.Code, started.Body)
	}
	id := between(started.Body.String(), `"id":"`, `"`)
	deadline := time.Now().Add(3 * time.Second)
	for {
		response := call(api, http.MethodGet, "/api/agent/runs/"+id, "")
		if strings.Contains(response.Body.String(), `"status":"completed"`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run did not complete: %s", response.Body)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if searcher.params.Title != "Go Engineer" {
		t.Fatalf("saved profile was not used: %#v", searcher.params)
	}
	if list := call(api, http.MethodGet, "/api/agent/runs", ""); !strings.Contains(list.Body.String(), id) {
		t.Fatalf("run missing from list: %s", list.Body)
	}
	if response := call(api, http.MethodGet, "/api/agent/runs/missing", ""); response.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", response.Code)
	}
	if response := call(api, http.MethodPost, "/api/agent/runs/"+id+"/cancel", ""); response.Code != http.StatusAccepted {
		t.Fatalf("expected cancel accepted, got %d", response.Code)
	}
}

func between(value, start, end string) string {
	_, rest, _ := strings.Cut(value, start)
	result, _, _ := strings.Cut(rest, end)
	return result
}

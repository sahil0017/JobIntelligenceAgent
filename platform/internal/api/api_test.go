package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"careerpilot/contracts/domain"
	"careerpilot/intelligence/agents"
	"careerpilot/intelligence/tools"
	"careerpilot/platform/internal/integrations/gmail"
	"careerpilot/platform/internal/integrations/webjobs"
	"careerpilot/platform/internal/services/discovery"
	"careerpilot/platform/internal/storage"
)

func TestAPISourcesAndHealthEndpoints(t *testing.T) {
	store, err := NewSourceStore(filepath.Join(t.TempDir(), "sources.json"))
	if err != nil {
		t.Fatal(err)
	}
	registry := tools.NewRegistry()
	if err := registry.Register(tools.Tool{
		Name: "discover_jobs",
		Risk: tools.RiskReadOnly,
		Handler: func(context.Context, json.RawMessage) (any, error) {
			return map[string]any{"jobs": []any{}, "sourceErrors": []any{}, "checkedSources": 1}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	api := New(store, registry)

	for _, path := range []string{"/api/health", "/api/sources"} {
		response := httptest.NewRecorder()
		api.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Errorf("%s returned %d: %s", path, response.Code, response.Body)
		}
	}

	request := httptest.NewRequest(http.MethodPost, "/api/sources", strings.NewReader(`{"provider":"lever","company":"Example","boardId":"example"}`))
	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create source returned %d: %s", response.Code, response.Body)
	}

	duplicate := httptest.NewRecorder()
	api.Handler().ServeHTTP(duplicate, httptest.NewRequest(http.MethodPost, "/api/sources", strings.NewReader(`{"provider":"lever","company":"Other","boardId":"example"}`)))
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate source returned %d: %s", duplicate.Code, duplicate.Body)
	}
}

func TestAPIDiscoveryPreservesFrontendResponseContract(t *testing.T) {
	store, err := NewSourceStore(filepath.Join(t.TempDir(), "sources.json"))
	if err != nil {
		t.Fatal(err)
	}
	registry := tools.NewRegistry()
	if err := registry.Register(tools.Tool{
		Name: "discover_jobs",
		Risk: tools.RiskReadOnly,
		Handler: func(context.Context, json.RawMessage) (any, error) {
			return map[string]any{"jobs": []any{}, "sourceErrors": []any{}, "checkedSources": 1, "workflow": []any{}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	New(store, registry).Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/jobs?skills=Go&title=Engineer&location=Remote", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("discovery returned %d: %s", response.Code, response.Body)
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"jobs", "sourceErrors", "checkedSources"} {
		if _, ok := result[key]; !ok {
			t.Errorf("response missing frontend field %q", key)
		}
	}
}

func TestAPIDiscoveryRejectsOversizedProfileQuery(t *testing.T) {
	store, err := NewSourceStore(filepath.Join(t.TempDir(), "sources.json"))
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	New(store, tools.NewRegistry()).Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/jobs?skills="+strings.Repeat("x", 501), nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("oversized profile returned %d, want 400", response.Code)
	}
}

type fakeGmailIntegration struct {
	connected bool
	startURL  string
	state     string
	code      string
	query     string
	limit     int
	sentTo    string
	sentSubj  string
	sentBody  string
}

type fakeCompanySearch struct {
	apiKey string
	query  string
	limit  int
}

type fakeAreaJobSearch struct {
	apiKey  string
	country string
	state   string
	area    string
	profile webjobs.Profile
	result  webjobs.Result
}

func (search *fakeAreaJobSearch) Search(_ context.Context, apiKey, country, state, area string, profile webjobs.Profile) (webjobs.Result, error) {
	search.apiKey, search.country, search.state, search.area, search.profile = apiKey, country, state, area, profile
	return search.result, nil
}

func (search *fakeCompanySearch) Search(_ context.Context, apiKey, query string, limit int) ([]domain.Company, error) {
	search.apiKey, search.query, search.limit = apiKey, query, limit
	return []domain.Company{{ID: "place-1", Name: "Example Labs", Website: "https://example.test"}}, nil
}

func (integration *fakeGmailIntegration) Configure(storage.IntegrationConfig) error {
	return nil
}

func (integration *fakeGmailIntegration) Configured() bool {
	return true
}

func (integration *fakeGmailIntegration) StartAuthorization() (string, error) {
	return integration.startURL, nil
}

func (integration *fakeGmailIntegration) CompleteAuthorization(_ context.Context, state, code string) error {
	integration.state = state
	integration.code = code
	integration.connected = true
	return nil
}

func (integration *fakeGmailIntegration) Connected() bool {
	return integration.connected
}

func (integration *fakeGmailIntegration) Disconnect() {
	integration.connected = false
}

func (integration *fakeGmailIntegration) SearchThreads(_ context.Context, query string, limit int) ([]gmail.ThreadSummary, error) {
	integration.query = query
	integration.limit = limit
	return []gmail.ThreadSummary{{ID: "thread-1", Messages: []gmail.MessageSummary{{
		ID: "message-1", From: "jobs@example.test", Subject: "Backend Engineer opening",
		Snippet: "We are hiring", Body: "We are hiring a backend engineer.",
	}}}}, nil
}

func (integration *fakeGmailIntegration) SendMessage(_ context.Context, recipient, subject, body string) (string, error) {
	integration.sentTo = recipient
	integration.sentSubj = subject
	integration.sentBody = body
	return "sent-message", nil
}

func (integration *fakeGmailIntegration) EmailAddress(context.Context) (string, error) {
	return "candidate@example.test", nil
}

type fakeDiscoveryAgent struct{}

func (fakeDiscoveryAgent) Run(_ context.Context, request agents.DiscoveryRequest) (agents.DiscoveryResponse, error) {
	return agents.DiscoveryResponse{
		Answer: "Found a match.",
		DiscoveryOutput: discovery.Result{
			DiscoveryResult: domain.DiscoveryResult{
				Jobs: []domain.Job{{Company: "Example", Role: request.Profile.Title}},
			},
		},
	}, nil
}

func TestGmailAndAgentEndpoints(t *testing.T) {
	store, err := NewSourceStore(filepath.Join(t.TempDir(), "sources.json"))
	if err != nil {
		t.Fatal(err)
	}
	gmailIntegration := &fakeGmailIntegration{startURL: "https://accounts.example.test/authorize"}
	api := New(store, tools.NewRegistry(), Services{Gmail: gmailIntegration, DiscoveryAgent: fakeDiscoveryAgent{}})

	status := httptest.NewRecorder()
	api.Handler().ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/gmail/status", nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"configured":true`) {
		t.Fatalf("unexpected Gmail status: %d %s", status.Code, status.Body)
	}

	start := httptest.NewRecorder()
	api.Handler().ServeHTTP(start, httptest.NewRequest(http.MethodGet, "/api/gmail/oauth/start", nil))
	if start.Code != http.StatusOK || !strings.Contains(start.Body.String(), gmailIntegration.startURL) {
		t.Fatalf("unexpected Gmail authorization response: %d %s", start.Code, start.Body)
	}

	callback := httptest.NewRecorder()
	api.Handler().ServeHTTP(callback, httptest.NewRequest(http.MethodGet, "/api/gmail/oauth/callback?state=state-123&code=code-123", nil))
	if callback.Code != http.StatusSeeOther || callback.Header().Get("Location") != "http://localhost:5173/?connected=gmail" ||
		gmailIntegration.state != "state-123" || gmailIntegration.code != "code-123" {
		t.Fatalf("Gmail callback was not completed: %d %s", callback.Code, callback.Body)
	}

	threads := httptest.NewRecorder()
	api.Handler().ServeHTTP(threads, httptest.NewRequest(http.MethodGet, "/api/gmail/threads", nil))
	if threads.Code != http.StatusOK || gmailIntegration.query == "" || gmailIntegration.limit != 10 {
		t.Fatalf("Gmail default search failed: %d %s", threads.Code, threads.Body)
	}
	oversizedQuery := httptest.NewRecorder()
	api.Handler().ServeHTTP(oversizedQuery, httptest.NewRequest(
		http.MethodGet,
		"/api/gmail/threads?q="+strings.Repeat("x", 501),
		nil,
	))
	if oversizedQuery.Code != http.StatusBadRequest {
		t.Fatalf("oversized Gmail query returned %d, want 400", oversizedQuery.Code)
	}
	sendWithoutConfirmation := httptest.NewRecorder()
	api.Handler().ServeHTTP(sendWithoutConfirmation, httptest.NewRequest(
		http.MethodPost,
		"/api/gmail/send",
		strings.NewReader(`{"to":"hr@example.test","subject":"Role","body":"Hello"}`),
	))
	if sendWithoutConfirmation.Code != http.StatusBadRequest || gmailIntegration.sentTo != "" {
		t.Fatalf("message without explicit confirmation was sent: %d %s", sendWithoutConfirmation.Code, sendWithoutConfirmation.Body)
	}
	sendConfirmed := httptest.NewRecorder()
	api.Handler().ServeHTTP(sendConfirmed, httptest.NewRequest(
		http.MethodPost,
		"/api/gmail/send",
		strings.NewReader(`{"to":"hr@example.test","subject":"Role","body":"Hello","confirm":true}`),
	))
	if sendConfirmed.Code != http.StatusOK || gmailIntegration.sentTo != "hr@example.test" ||
		gmailIntegration.sentSubj != "Role" || gmailIntegration.sentBody != "Hello" {
		t.Fatalf("confirmed message was not sent: %d %s", sendConfirmed.Code, sendConfirmed.Body)
	}

	saveApplied := httptest.NewRecorder()
	api.Handler().ServeHTTP(saveApplied, httptest.NewRequest(
		http.MethodPost,
		"/api/applied-jobs",
		strings.NewReader(`{"job":{"id":"greenhouse:example:42","company":"Example","role":"Engineer","url":"https://boards.greenhouse.io/example/jobs/42","status":"Applied","sourceProvider":"greenhouse","sourceBoardId":"example"}}`),
	))
	if saveApplied.Code != http.StatusCreated {
		t.Fatalf("save applied job returned %d: %s", saveApplied.Code, saveApplied.Body)
	}
	unconfirmedDelete := httptest.NewRecorder()
	api.Handler().ServeHTTP(unconfirmedDelete, httptest.NewRequest(http.MethodDelete, "/api/applied-jobs/greenhouse%3Aexample%3A42", nil))
	if unconfirmedDelete.Code != http.StatusConflict {
		t.Fatalf("unconfirmed applied job deletion returned %d: %s", unconfirmedDelete.Code, unconfirmedDelete.Body)
	}

	agent := httptest.NewRecorder()
	api.Handler().ServeHTTP(agent, httptest.NewRequest(http.MethodPost, "/api/agent/discover", strings.NewReader(`{"profile":{"title":"Engineer"}}`)))
	if agent.Code != http.StatusOK || !strings.Contains(agent.Body.String(), `"answer":"Found a match."`) {
		t.Fatalf("agent endpoint failed: %d %s", agent.Code, agent.Body)
	}

	disconnect := httptest.NewRecorder()
	api.Handler().ServeHTTP(disconnect, httptest.NewRequest(http.MethodDelete, "/api/gmail/connection", nil))
	if disconnect.Code != http.StatusOK || gmailIntegration.connected {
		t.Fatalf("Gmail disconnect failed: %d %s", disconnect.Code, disconnect.Body)
	}
}

func TestProfileIntegrationAndPortalConfigurationPersistInSQLite(t *testing.T) {
	store, err := NewSourceStore(filepath.Join(t.TempDir(), "careerpilot.db"))
	if err != nil {
		t.Fatal(err)
	}

	gmailIntegration := &fakeGmailIntegration{}
	api := New(store, tools.NewRegistry(), Services{Gmail: gmailIntegration})
	handler := api.Handler()

	profileResponse := httptest.NewRecorder()
	handler.ServeHTTP(profileResponse, httptest.NewRequest(http.MethodPut, "/api/profile", strings.NewReader(
		`{"name":"Sahil","title":"Backend Engineer","email":"sahil@example.test","location":"Remote","experience":"4 years","skills":"Go, SQL","roles":"Backend Engineer"}`,
	)))
	if profileResponse.Code != http.StatusOK {
		t.Fatalf("save profile returned %d: %s", profileResponse.Code, profileResponse.Body)
	}
	profile, err := store.Database().LoadProfile()
	if err != nil || profile.Name != "Sahil" || profile.Skills != "Go, SQL" {
		t.Fatalf("profile not persisted: %#v, %v", profile, err)
	}
	readiness := httptest.NewRecorder()
	handler.ServeHTTP(readiness, httptest.NewRequest(
		http.MethodPost,
		"/api/applications/prepare",
		strings.NewReader(`{"job":{"company":"Example","role":"Engineer","url":"https://jobs.example.test/1"}}`),
	))
	if readiness.Code != http.StatusOK || !strings.Contains(readiness.Body.String(), `"submissionSupported":false`) ||
		!strings.Contains(readiness.Body.String(), `"requiresUserApproval":true`) {
		t.Fatalf("application readiness must not submit jobs: %d %s", readiness.Code, readiness.Body)
	}

	integrationResponse := httptest.NewRecorder()
	handler.ServeHTTP(integrationResponse, httptest.NewRequest(
		http.MethodPut,
		"/api/settings/integrations/gmail",
		strings.NewReader(`{"clientId":"client-id","clientSecret":"top-secret","redirectUrl":"http://localhost:8787/api/gmail/oauth/callback"}`),
	))
	if integrationResponse.Code != http.StatusOK || strings.Contains(integrationResponse.Body.String(), "top-secret") {
		t.Fatalf("save Gmail settings response exposed a secret: %d %s", integrationResponse.Code, integrationResponse.Body)
	}
	config, err := store.Database().LoadIntegration("gmail")
	if err != nil || config.ClientID != "client-id" || config.ClientSecret != "top-secret" {
		t.Fatalf("OAuth configuration not persisted: %#v, %v", config, err)
	}

	portalResponse := httptest.NewRecorder()
	handler.ServeHTTP(portalResponse, httptest.NewRequest(
		http.MethodPost,
		"/api/portals",
		strings.NewReader(`{"name":"Naukri","provider":"naukri","url":"https://www.naukri.com/","notes":"Use my existing account"}`),
	))
	if portalResponse.Code != http.StatusCreated || !strings.Contains(portalResponse.Body.String(), `"status":"adapter_unavailable"`) {
		t.Fatalf("save Naukri portal returned %d: %s", portalResponse.Code, portalResponse.Body)
	}
	portals, err := store.Database().ListPortals()
	if err != nil || len(portals) != 1 || portals[0].Name != "Naukri" {
		t.Fatalf("portal configuration not persisted: %#v, %v", portals, err)
	}
}

func TestAppliedJobClosureRequiresExplicitKeepOrDelete(t *testing.T) {
	sourceStore, err := NewSourceStore(filepath.Join(t.TempDir(), "careerpilot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sourceStore.Database().Close()
	api := New(sourceStore, tools.NewRegistry())
	job := domain.Job{
		ID: "greenhouse:example:42", Company: "Example", Role: "Engineer",
		URL: "https://boards.greenhouse.io/example/jobs/42", Status: "Applied",
		SourceID: domain.ProviderGreenhouse, SourceBoard: "example",
	}
	save := httptest.NewRecorder()
	api.Handler().ServeHTTP(save, httptest.NewRequest(http.MethodPost, "/api/applied-jobs", strings.NewReader(
		`{"job":{"id":"greenhouse:example:42","company":"Example","role":"Engineer","url":"https://boards.greenhouse.io/example/jobs/42","status":"Applied","sourceProvider":"greenhouse","sourceBoardId":"example"}}`,
	)))
	if save.Code != http.StatusCreated {
		t.Fatalf("save applied job returned %d: %s", save.Code, save.Body)
	}
	recordClosure := func() {
		t.Helper()
		for range 3 {
			if _, err := sourceStore.Database().RecordTrackedJobCheck(job.ID, nil, time.Now()); err != nil {
				t.Fatal(err)
			}
		}
	}
	recordClosure()
	keep := httptest.NewRecorder()
	api.Handler().ServeHTTP(keep, httptest.NewRequest(http.MethodPost, "/api/applied-jobs/greenhouse%3Aexample%3A42/keep", nil))
	if keep.Code != http.StatusOK {
		t.Fatalf("keep closed job returned %d: %s", keep.Code, keep.Body)
	}
	tracked, err := sourceStore.Database().ListTrackedJobs()
	if err != nil || len(tracked) != 1 || tracked[0].ClosurePending {
		t.Fatalf("kept job was not retained: %#v, %v", tracked, err)
	}

	recordClosure()
	remove := httptest.NewRecorder()
	api.Handler().ServeHTTP(remove, httptest.NewRequest(http.MethodDelete, "/api/applied-jobs/greenhouse%3Aexample%3A42", nil))
	if remove.Code != http.StatusOK {
		t.Fatalf("confirmed closed-job deletion returned %d: %s", remove.Code, remove.Body)
	}
	tracked, err = sourceStore.Database().ListTrackedJobs()
	if err != nil || len(tracked) != 0 {
		t.Fatalf("confirmed deletion left the job in storage: %#v, %v", tracked, err)
	}
}

func TestGooglePlacesKeyIsSavedAndCompanySearchUsesIt(t *testing.T) {
	sourceStore, err := NewSourceStore(filepath.Join(t.TempDir(), "careerpilot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sourceStore.Database().Close()
	search := &fakeCompanySearch{}
	api := New(sourceStore, tools.NewRegistry(), Services{Places: search})
	saveKey := httptest.NewRecorder()
	api.Handler().ServeHTTP(saveKey, httptest.NewRequest(http.MethodPut, "/api/settings/places", strings.NewReader(
		`{"apiKey":"private-test-key"}`,
	)))
	if saveKey.Code != http.StatusOK || strings.Contains(saveKey.Body.String(), "private-test-key") {
		t.Fatalf("Places key response should only report configuration status: %d %s", saveKey.Code, saveKey.Body)
	}
	config, err := sourceStore.Database().LoadIntegration("google_places")
	if err != nil || config.ClientSecret != "private-test-key" {
		t.Fatalf("Google Places key was not persisted: %#v, %v", config, err)
	}

	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/companies/search", strings.NewReader(
		`{"query":"software companies in Bengaluru"}`,
	)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"name":"Example Labs"`) ||
		search.apiKey != "private-test-key" || search.query != "software companies in Bengaluru" || search.limit != 20 {
		t.Fatalf("company search did not use configured key/default limit: %d %s, search=%#v", response.Code, response.Body, search)
	}

	deleteKey := httptest.NewRecorder()
	api.Handler().ServeHTTP(deleteKey, httptest.NewRequest(http.MethodDelete, "/api/settings/places", nil))
	if deleteKey.Code != http.StatusOK {
		t.Fatalf("delete Places key returned %d: %s", deleteKey.Code, deleteKey.Body)
	}
	unconfigured := httptest.NewRecorder()
	api.Handler().ServeHTTP(unconfigured, httptest.NewRequest(http.MethodPost, "/api/companies/search", strings.NewReader(
		`{"query":"software companies in Bengaluru"}`,
	)))
	if unconfigured.Code != http.StatusPreconditionRequired {
		t.Fatalf("search without Places key returned %d: %s", unconfigured.Code, unconfigured.Body)
	}
}

func TestAreaJobSearchUsesEncryptedKeyAndLocalResume(t *testing.T) {
	sourceStore, err := NewSourceStore(filepath.Join(t.TempDir(), "careerpilot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sourceStore.Database().Close()
	if err := sourceStore.Database().SaveProfile(storage.Profile{Title: "Backend Engineer", Skills: "Go, Kubernetes"}); err != nil {
		t.Fatal(err)
	}
	if err := sourceStore.Database().SaveResume(storage.Resume{Filename: "resume.pdf", Text: "local-only resume text"}); err != nil {
		t.Fatal(err)
	}
	local := webjobs.JobResult{
		ID: "web:local", Company: "Verified Co", Role: "Backend Engineer",
		Location: "Bengaluru, India", WorkMode: "Hybrid", Source: "Verified public career page",
		URL: "https://careers.example.com/jobs/1", Status: "New",
	}
	remote := webjobs.JobResult{
		ID: "web:remote", Company: "Remote Co", Role: "Backend Engineer",
		Location: "Remote, India", WorkMode: "Remote", Source: "Verified public career page",
		URL: "https://careers.remote.example/jobs/1", Status: "New",
	}
	search := &fakeAreaJobSearch{result: webjobs.Result{
		Jobs: []webjobs.JobResult{local, remote}, CheckedSources: 2,
	}}
	api := New(sourceStore, tools.NewRegistry(), Services{WebJobs: search})
	saveKey := httptest.NewRecorder()
	api.Handler().ServeHTTP(saveKey, httptest.NewRequest(http.MethodPut, "/api/settings/web-search", strings.NewReader(
		`{"apiKey":"private-brave-key"}`,
	)))
	if saveKey.Code != http.StatusOK || strings.Contains(saveKey.Body.String(), "private-brave-key") {
		t.Fatalf("web search key response should only report configuration status: %d %s", saveKey.Code, saveKey.Body)
	}
	config, err := sourceStore.Database().LoadIntegration("brave_search")
	if err != nil || config.ClientSecret != "private-brave-key" {
		t.Fatalf("Brave Search key was not persisted encrypted: %#v, %v", config, err)
	}

	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/area-jobs", strings.NewReader(
		`{"country":"India","state":"Karnataka","area":"Bengaluru","includeRemote":false}`,
	)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"company":"Verified Co"`) ||
		strings.Contains(response.Body.String(), `"company":"Remote Co"`) {
		t.Fatalf("area search returned an unexpected response: %d %s", response.Code, response.Body)
	}
	if search.apiKey != "private-brave-key" || search.country != "India" || search.state != "Karnataka" ||
		search.area != "Bengaluru" || search.profile.Title != "Backend Engineer" ||
		search.profile.Skills != "Go, Kubernetes" || search.profile.ResumeText != "local-only resume text" {
		t.Fatalf("area search omitted saved configuration or local profile: %#v", search)
	}

	settings := httptest.NewRecorder()
	api.Handler().ServeHTTP(settings, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if settings.Code != http.StatusOK || strings.Contains(settings.Body.String(), "private-brave-key") ||
		!strings.Contains(settings.Body.String(), `"webSearch":{"configured":true}`) {
		t.Fatalf("settings should report only web-search key status: %d %s", settings.Code, settings.Body)
	}
}

func TestAreaJobSearchRequiresKeyAndValidLocation(t *testing.T) {
	sourceStore, err := NewSourceStore(filepath.Join(t.TempDir(), "careerpilot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sourceStore.Database().Close()
	api := New(sourceStore, tools.NewRegistry(), Services{WebJobs: &fakeAreaJobSearch{}})

	unconfigured := httptest.NewRecorder()
	api.Handler().ServeHTTP(unconfigured, httptest.NewRequest(http.MethodPost, "/api/area-jobs", strings.NewReader(
		`{"country":"India","area":"Bengaluru"}`,
	)))
	if unconfigured.Code != http.StatusPreconditionRequired {
		t.Fatalf("area search without web search key returned %d: %s", unconfigured.Code, unconfigured.Body)
	}

	invalid := httptest.NewRecorder()
	api.Handler().ServeHTTP(invalid, httptest.NewRequest(http.MethodPost, "/api/area-jobs", strings.NewReader(
		`{"country":"","area":"Bengaluru"}`,
	)))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("area search with invalid location returned %d: %s", invalid.Code, invalid.Body)
	}
}

func TestAreaDiscoveryIncludesSavedResumeAndSelectedLocation(t *testing.T) {
	sourceStore, err := NewSourceStore(filepath.Join(t.TempDir(), "careerpilot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sourceStore.Database().Close()
	const resumeText = "Go, distributed systems, Kubernetes"
	if err := sourceStore.Database().SaveResume(storage.Resume{Filename: "resume.pdf", Text: resumeText}); err != nil {
		t.Fatal(err)
	}

	var captured discovery.Request
	registry := tools.NewRegistry()
	if err := registry.Register(tools.Tool{
		Name: "discover_jobs",
		Risk: tools.RiskReadOnly,
		Handler: func(_ context.Context, input json.RawMessage) (any, error) {
			if err := json.Unmarshal(input, &captured); err != nil {
				t.Fatal(err)
			}
			return map[string]any{"jobs": []domain.Job{}, "sourceErrors": []any{}, "checkedSources": 1}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	api := New(sourceStore, registry)
	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		"/api/jobs?skills=Go&title=Backend+Engineer&location=Remote&area=Bengaluru&includeRemote=true&remoteRegion=India", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("area discovery returned %d: %s", response.Code, response.Body)
	}
	if captured.Area != "Bengaluru" || !captured.IncludeRemote || captured.RemoteRegion != "India" ||
		captured.Profile.Location != "Bengaluru" || captured.Profile.ResumeText != resumeText {
		t.Fatalf("resume or selected area missing from discovery: %#v", captured)
	}
}

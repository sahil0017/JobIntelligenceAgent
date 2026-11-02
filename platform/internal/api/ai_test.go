package api

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"careerpilot/intelligence/tools"
	"careerpilot/platform/internal/integrations/cortex"
	"careerpilot/platform/internal/storage"
)

type fakeAI struct {
	started     cortex.AnalysisRequest
	application cortex.ApplicationRequest
	decision    cortex.Decision
	portal      cortex.PortalSearchRequest
	signedInTo  string
	applicant   cortex.Applicant
	err         error
}

func (ai *fakeAI) Health(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{"status":"ok","engine":"fake","deepLearning":false}`), ai.err
}
func (ai *fakeAI) Pipeline(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`[{"id":"prepare"}]`), ai.err
}
func (ai *fakeAI) Start(_ context.Context, request cortex.AnalysisRequest) (json.RawMessage, error) {
	ai.started = request
	return json.RawMessage(`{"id":"run1","status":"running"}`), ai.err
}
func (ai *fakeAI) List(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`[]`), ai.err
}
func (ai *fakeAI) Get(_ context.Context, id string) (json.RawMessage, error) {
	if id == "missing" {
		return nil, cortex.ErrNotFound
	}
	return json.RawMessage(`{"id":"` + id + `"}`), ai.err
}
func (ai *fakeAI) Cancel(context.Context, string) (json.RawMessage, error) {
	return json.RawMessage(`{"status":"cancelling"}`), ai.err
}

func (ai *fakeAI) ApplicationPipeline(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`[{"id":"scrape"}]`), ai.err
}
func (ai *fakeAI) StartApplication(_ context.Context, request cortex.ApplicationRequest) (json.RawMessage, error) {
	ai.application = request
	return json.RawMessage(`{"id":"app1","status":"running"}`), ai.err
}
func (ai *fakeAI) Applications(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`[]`), ai.err
}
func (ai *fakeAI) Application(_ context.Context, id string) (json.RawMessage, error) {
	if id == "missing" {
		return nil, cortex.ErrNotFound
	}
	return json.RawMessage(`{"id":"` + id + `"}`), ai.err
}
func (ai *fakeAI) DecideApplication(_ context.Context, id string, decision cortex.Decision) (json.RawMessage, error) {
	ai.decision = decision
	if id == "done" {
		return nil, cortex.ErrConflict
	}
	return json.RawMessage(`{"id":"` + id + `"}`), ai.err
}
func (ai *fakeAI) CancelApplication(context.Context, string) (json.RawMessage, error) {
	return json.RawMessage(`{"status":"cancelling"}`), ai.err
}

func (ai *fakeAI) PortalSearch(_ context.Context, request cortex.PortalSearchRequest) (json.RawMessage, error) {
	ai.portal = request
	return json.RawMessage(`{"status":"auth_required","host":"x"}`), ai.err
}
func (ai *fakeAI) PortalSession(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{"signedIn":false}`), ai.err
}
func (ai *fakeAI) PortalSignIn(_ context.Context, address string) (json.RawMessage, error) {
	ai.signedInTo = address
	return json.RawMessage(`{"signedIn":false,"windowOpen":true}`), ai.err
}
func (ai *fakeAI) PortalSignOut(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{"signedIn":false}`), ai.err
}

func (ai *fakeAI) ApplyApplication(_ context.Context, id string, applicant cortex.Applicant) (json.RawMessage, error) {
	ai.applicant = applicant
	if id == "draft" {
		return nil, cortex.ErrConflict
	}
	return json.RawMessage(`{"id":"` + id + `"}`), ai.err
}

func (ai *fakeAI) QuickApply(_ context.Context, request cortex.QuickApply) (json.RawMessage, error) {
	ai.applicant = request.Applicant
	return json.RawMessage(`{"status":"opened","filled":["email"]}`), ai.err
}

func newAIAPI(t *testing.T, ai AIAnalyzer) *API {
	t.Helper()
	store, err := NewSourceStore(filepath.Join(t.TempDir(), "careerpilot.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Database().Close() })
	return New(store, tools.NewRegistry(), Services{AI: ai})
}

func TestAnalysisAttachesSavedProfileAndResumeServerSide(t *testing.T) {
	ai := &fakeAI{}
	api := newAIAPI(t, ai)
	database := api.sources.Database()
	if err := database.SaveProfile(storage.Profile{Title: "Engineer", Skills: "Go, Kafka"}); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveResume(storage.Resume{Filename: "cv.pdf", Text: "Built Go services"}); err != nil {
		t.Fatal(err)
	}

	response := call(api, http.MethodPost, "/api/ai/analyses", `{"jobs":[{"id":"j1","role":"Backend Engineer","company":"Acme"}]}`)
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"run1"`) {
		t.Fatalf("unexpected response %d: %s", response.Code, response.Body.String())
	}
	if ai.started.Profile.Skills != "Go, Kafka" || ai.started.ResumeText != "Built Go services" {
		t.Fatalf("profile/resume were not attached: %+v", ai.started)
	}
	if ai.started.Jobs[0].Tags == nil {
		t.Fatal("tags must be an empty list, not null, for the Python service")
	}
}

func TestAnalysisValidationAndBrowserCannotSupplyResume(t *testing.T) {
	api := newAIAPI(t, &fakeAI{})
	cases := map[string]string{
		"no jobs":        `{"jobs":[]}`,
		"missing role":   `{"jobs":[{"id":"1"}]}`,
		"resume smuggle": `{"jobs":[{"id":"1","role":"x"}],"resumeText":"secret"}`,
	}
	for name, body := range cases {
		if response := call(api, http.MethodPost, "/api/ai/analyses", body); response.Code != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %d", name, response.Code)
		}
	}
	tooMany := `{"jobs":[` + strings.Repeat(`{"id":"1","role":"x"},`, 100) + `{"id":"1","role":"x"}]}`
	if response := call(api, http.MethodPost, "/api/ai/analyses", tooMany); response.Code != http.StatusBadRequest {
		t.Errorf("101 jobs: want 400, got %d", response.Code)
	}
}

func TestAIStatusReportsOfflineWithoutFailing(t *testing.T) {
	api := newAIAPI(t, &fakeAI{err: cortex.ErrUnavailable})
	response := call(api, http.MethodGet, "/api/ai/status", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"available":false`) {
		t.Fatalf("unexpected offline status: %d %s", response.Code, response.Body.String())
	}
	online := call(newAIAPI(t, &fakeAI{}), http.MethodGet, "/api/ai/status", "")
	if !strings.Contains(online.Body.String(), `"available":true`) || !strings.Contains(online.Body.String(), `"engine":"fake"`) {
		t.Fatalf("unexpected online status: %s", online.Body.String())
	}
}

func TestAIErrorsMapToStatusCodes(t *testing.T) {
	down := newAIAPI(t, &fakeAI{err: cortex.ErrUnavailable})
	if response := call(down, http.MethodGet, "/api/ai/analyses", ""); response.Code != http.StatusServiceUnavailable {
		t.Errorf("offline list: want 503, got %d", response.Code)
	}
	api := newAIAPI(t, &fakeAI{})
	if response := call(api, http.MethodGet, "/api/ai/analyses/missing", ""); response.Code != http.StatusNotFound {
		t.Errorf("missing run: want 404, got %d", response.Code)
	}
	if response := call(api, http.MethodPost, "/api/ai/analyses/run1/cancel", ""); response.Code != http.StatusAccepted {
		t.Errorf("cancel: want 202, got %d", response.Code)
	}
	if response := call(api, http.MethodDelete, "/api/ai/analyses", ""); response.Code != http.StatusMethodNotAllowed {
		t.Errorf("delete: want 405, got %d", response.Code)
	}
	unconfigured := newAIAPI(t, nil)
	if response := call(unconfigured, http.MethodGet, "/api/ai/analyses", ""); response.Code != http.StatusServiceUnavailable {
		t.Errorf("unconfigured: want 503, got %d", response.Code)
	}
}

func TestApplicationAttachesResumeAndValidatesInput(t *testing.T) {
	ai := &fakeAI{}
	api := newAIAPI(t, ai)
	if err := api.sources.Database().SaveResume(storage.Resume{Filename: "cv.pdf", Text: "Built Go services"}); err != nil {
		t.Fatal(err)
	}
	ok := `{"job":{"id":"j1","role":"Backend Engineer"},"url":"https://example.com/job"}`
	if response := call(api, http.MethodPost, "/api/ai/applications", ok); response.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d: %s", response.Code, response.Body.String())
	}
	if ai.application.ResumeText != "Built Go services" || ai.application.Job.Tags == nil {
		t.Fatalf("resume or tags not attached: %+v", ai.application)
	}
	bad := map[string]string{
		"no role": `{"job":{"id":"1"}}`,
		"ftp url": `{"job":{"id":"1","role":"x"},"url":"ftp://example.com"}`,
		"no host": `{"job":{"id":"1","role":"x"},"url":"https://"}`,
		"smuggle": `{"job":{"id":"1","role":"x"},"resumeText":"secret"}`,
	}
	for name, body := range bad {
		if response := call(api, http.MethodPost, "/api/ai/applications", body); response.Code != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %d", name, response.Code)
		}
	}
}

func TestApplicationDecisionRoutes(t *testing.T) {
	ai := &fakeAI{}
	api := newAIAPI(t, ai)
	if response := call(api, http.MethodPost, "/api/ai/applications/app1/decision", `{"approve":true,"coverNote":"Hi"}`); response.Code != http.StatusAccepted {
		t.Fatalf("decision: want 202, got %d", response.Code)
	}
	if !ai.decision.Approve || ai.decision.CoverNote == nil || *ai.decision.CoverNote != "Hi" {
		t.Fatalf("decision not forwarded: %+v", ai.decision)
	}
	if response := call(api, http.MethodPost, "/api/ai/applications/done/decision", `{"approve":true}`); response.Code != http.StatusConflict {
		t.Errorf("already decided: want 409, got %d", response.Code)
	}
	if response := call(api, http.MethodPost, "/api/ai/applications/app1/decision", `{"approve":true,"extra":1}`); response.Code != http.StatusBadRequest {
		t.Errorf("unknown field: want 400, got %d", response.Code)
	}
	if response := call(api, http.MethodGet, "/api/ai/applications/missing", ""); response.Code != http.StatusNotFound {
		t.Errorf("missing: want 404, got %d", response.Code)
	}
	if response := call(api, http.MethodGet, "/api/ai/applications/app1/decision", ""); response.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET decision: want 405, got %d", response.Code)
	}
	if response := call(api, http.MethodPost, "/api/ai/applications/app1/cancel", ""); response.Code != http.StatusAccepted {
		t.Errorf("cancel: want 202, got %d", response.Code)
	}
}

func TestPortalRoutesValidateAndAttachCandidate(t *testing.T) {
	ai := &fakeAI{}
	api := newAIAPI(t, ai)
	if err := api.sources.Database().SaveResume(storage.Resume{Filename: "cv.pdf", Text: "Built Go services"}); err != nil {
		t.Fatal(err)
	}
	if response := call(api, http.MethodPost, "/api/ai/portals/search", `{"url":"https://example.com/careers","keywords":"go"}`); response.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", response.Code, response.Body.String())
	}
	if ai.portal.ResumeText != "Built Go services" || ai.portal.Keywords != "go" {
		t.Fatalf("candidate not attached: %+v", ai.portal)
	}
	for name, body := range map[string]string{"ftp": `{"url":"ftp://example.com"}`, "empty": `{"url":""}`, "smuggle": `{"url":"https://example.com","resumeText":"x"}`} {
		if response := call(api, http.MethodPost, "/api/ai/portals/search", body); response.Code != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %d", name, response.Code)
		}
	}
	if response := call(api, http.MethodPost, "/api/ai/portals/session", `{"url":"https://example.com/login"}`); response.Code != http.StatusOK || ai.signedInTo != "https://example.com/login" {
		t.Fatalf("sign-in: %d %q", response.Code, ai.signedInTo)
	}
	if response := call(api, http.MethodPost, "/api/ai/portals/session", `{"url":"file:///etc/passwd"}`); response.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for file url, got %d", response.Code)
	}
	if response := call(api, http.MethodDelete, "/api/ai/portals/session", ""); response.Code != http.StatusOK {
		t.Fatalf("sign-out: %d", response.Code)
	}
}

func TestApplyUsesSavedProfileAndRequiresApproval(t *testing.T) {
	ai := &fakeAI{}
	api := newAIAPI(t, ai)
	if err := api.sources.Database().SaveProfile(storage.Profile{Name: "Ada Lovelace", Email: "ada@example.com"}); err != nil {
		t.Fatal(err)
	}
	if response := call(api, http.MethodPost, "/api/ai/applications/r1/apply", ""); response.Code != http.StatusAccepted || ai.applicant.Email != "ada@example.com" {
		t.Fatalf("apply: %d %+v", response.Code, ai.applicant)
	}
	if response := call(api, http.MethodPost, "/api/ai/applications/draft/apply", ""); response.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d", response.Code)
	}
	if response := call(api, http.MethodGet, "/api/ai/applications/r1/apply", ""); response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", response.Code)
	}
}

func TestQuickApplyUsesSavedProfileAndValidatesURL(t *testing.T) {
	ai := &fakeAI{}
	api := newAIAPI(t, ai)
	if err := api.sources.Database().SaveProfile(storage.Profile{Name: "Ada Lovelace", Email: "ada@example.com"}); err != nil {
		t.Fatal(err)
	}
	if response := call(api, http.MethodPost, "/api/ai/apply", `{"url":"ftp://x"}`); response.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", response.Code)
	}
	if response := call(api, http.MethodPost, "/api/ai/apply", `{"url":"https://example.com/job/1"}`); response.Code != http.StatusOK || ai.applicant.Email != "ada@example.com" {
		t.Fatalf("quick apply: %d %+v", response.Code, ai.applicant)
	}
}

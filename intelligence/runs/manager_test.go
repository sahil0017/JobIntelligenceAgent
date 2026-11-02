package runs

import (
	"careerpilot/contracts/domain"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeSearcher struct {
	result  domain.SearchResult
	err     error
	events  []domain.SearchEvent
	block   chan struct{}
	options domain.SearchOptions
}

func (searcher *fakeSearcher) SearchWithOptions(ctx context.Context, _, _, _, _ string, _ domain.SearchProfile, options domain.SearchOptions) (domain.SearchResult, error) {
	searcher.options = options
	for _, event := range searcher.events {
		options.OnEvent(event)
	}
	if searcher.block != nil {
		select {
		case <-searcher.block:
		case <-ctx.Done():
			return domain.SearchResult{}, ctx.Err()
		}
	}
	return searcher.result, searcher.err
}

type fakeReviewer struct {
	text string
	err  error
}

func (reviewer fakeReviewer) Review(context.Context, domain.AgentConfig, domain.SearchProfile, []domain.SearchJob) (string, error) {
	return reviewer.text, reviewer.err
}

func waitFor(t *testing.T, manager *Manager, id string) Run {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		run, err := manager.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if run.Status != StatusRunning {
			return run
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("run did not finish")
	return Run{}
}

func testParams() Params {
	config := domain.DefaultAgentConfig()
	config.MaxRounds, config.MinScore = 2, 55
	return Params{APIKey: "k", Country: "India", Area: "Bengaluru", Config: config}
}

func TestRunCompletesWithStepsTraceAndReview(t *testing.T) {
	searcher := &fakeSearcher{
		events: []domain.SearchEvent{
			{Stage: "plan", Message: "planned 2 search rounds"},
			{Stage: "search", Message: "round 1: query"},
			{Stage: "verify", Message: "verified 1 posting(s) on careers.acme.com"},
			{Stage: "filter", Message: "kept 1 jobs"},
		},
		result: domain.SearchResult{Jobs: []domain.SearchJob{{Company: "Acme", Role: "Go Engineer", Score: 80}}, CheckedSources: 3},
	}
	manager := NewManager(searcher, fakeReviewer{text: "Apply to Acme first."})
	run, err := manager.Start(testParams())
	if err != nil {
		t.Fatal(err)
	}
	finished := waitFor(t, manager, run.ID)
	if finished.Status != StatusCompleted || finished.Review != "Apply to Acme first." || finished.Result == nil || len(finished.Result.Jobs) != 1 {
		t.Fatalf("unexpected run: %#v", finished)
	}
	for _, step := range finished.Steps {
		if step.Status != StepDone {
			t.Errorf("step %s = %s, want done", step.ID, step.Status)
		}
	}
	if len(finished.Trace) < 5 {
		t.Fatalf("expected trace entries, got %#v", finished.Trace)
	}
	if searcher.options.MaxRounds != 2 || searcher.options.MinScore != 55 {
		t.Fatalf("config not passed to search: %#v", searcher.options)
	}
}

func TestRunSkipsReviewWhenModelUnavailable(t *testing.T) {
	searcher := &fakeSearcher{result: domain.SearchResult{Jobs: []domain.SearchJob{{Company: "Acme", Score: 80}}}}
	manager := NewManager(searcher, fakeReviewer{err: errors.New("connection refused")})
	run, _ := manager.Start(testParams())
	finished := waitFor(t, manager, run.ID)
	review := finished.Steps[len(finished.Steps)-1]
	if finished.Status != StatusCompleted || review.Status != StepSkipped || !strings.Contains(review.Detail, "connection refused") {
		t.Fatalf("expected completed run with skipped review, got %#v", finished)
	}
}

func TestRunReviewDisabledAndSearchFailure(t *testing.T) {
	params := testParams()
	params.Config.LLMReview = false
	manager := NewManager(&fakeSearcher{err: errors.New("Brave Search returned HTTP 401")}, fakeReviewer{})
	run, _ := manager.Start(params)
	finished := waitFor(t, manager, run.ID)
	if finished.Status != StatusFailed || !strings.Contains(finished.Error, "401") {
		t.Fatalf("expected failed run, got %#v", finished)
	}
	if finished.Steps[len(finished.Steps)-1].Status != StepSkipped {
		t.Fatalf("review should stay skipped when disabled: %#v", finished.Steps)
	}
}

func TestOnlyOneActiveRunAndCancel(t *testing.T) {
	searcher := &fakeSearcher{block: make(chan struct{})}
	manager := NewManager(searcher, fakeReviewer{})
	run, err := manager.Start(testParams())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Start(testParams()); !errors.Is(err, ErrBusy) {
		t.Fatalf("expected ErrBusy, got %v", err)
	}
	if err := manager.Cancel(run.ID); err != nil {
		t.Fatal(err)
	}
	finished := waitFor(t, manager, run.ID)
	if finished.Status != StatusCancelled {
		t.Fatalf("expected cancelled, got %s", finished.Status)
	}
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := manager.Start(testParams()); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("manager stayed busy after cancel")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := manager.Cancel("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestOllamaReviewerSendsNoResumeAndValidatesURL(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		data, _ := io.ReadAll(request.Body)
		body = string(data)
		_, _ = writer.Write([]byte(`{"message":{"content":" Acme is best. "}}`))
	}))
	defer server.Close()
	config := domain.DefaultAgentConfig()
	config.OllamaURL = server.URL
	text, err := NewOllamaReviewer().Review(context.Background(), config,
		domain.SearchProfile{Title: "Go Engineer", Skills: "Go", ResumeText: "secret resume phrase"},
		[]domain.SearchJob{{Company: "Acme", Role: "Go Engineer", Score: 90}})
	if err != nil || text != "Acme is best." {
		t.Fatalf("unexpected review %q, %v", text, err)
	}
	if strings.Contains(body, "secret resume phrase") || !strings.Contains(body, "Acme") {
		t.Fatalf("unexpected request body: %s", body)
	}
	config.OllamaURL = "ftp://x"
	if _, err := NewOllamaReviewer().Review(context.Background(), config, domain.SearchProfile{}, nil); err == nil {
		t.Fatal("expected URL validation error")
	}
}

func TestFinishStepKeepsFirstDuration(t *testing.T) {
	run := &Run{Steps: newSteps(false)}
	startStep(run, "plan", "")
	time.Sleep(5 * time.Millisecond)
	finishStep(run, "plan", StepDone, "planned")
	first := run.Steps[0].DurationMs
	time.Sleep(20 * time.Millisecond)
	finishStep(run, "plan", StepDone, "")
	if first == 0 || run.Steps[0].DurationMs != first {
		t.Fatalf("duration changed after the step finished: %d -> %d", first, run.Steps[0].DurationMs)
	}
}

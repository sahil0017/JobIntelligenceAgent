// Package runs runs the job-search agent as an observable, cancellable
// pipeline: plan, search, verify, score/filter, and an optional local-LLM review.
package runs

import (
	"careerpilot/contracts/domain"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type StepStatus string

const (
	StepPending StepStatus = "pending"
	StepRunning StepStatus = "running"
	StepDone    StepStatus = "done"
	StepSkipped StepStatus = "skipped"
	StepFailed  StepStatus = "failed"
)

const (
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"

	maxKeptRuns = 20
	maxTrace    = 300
	runTimeout  = 4 * time.Minute
)

var (
	ErrBusy     = errors.New("an agent run is already in progress")
	ErrNotFound = errors.New("agent run not found")
)

type Step struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      StepStatus `json:"status"`
	Detail      string     `json:"detail"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	DurationMs  int64      `json:"durationMs"`
}

type TraceEntry struct {
	Time    time.Time `json:"time"`
	Stage   string    `json:"stage"`
	Message string    `json:"message"`
}

type Run struct {
	ID         string               `json:"id"`
	Status     string               `json:"status"`
	Country    string               `json:"country"`
	State      string               `json:"state"`
	Area       string               `json:"area"`
	StartedAt  time.Time            `json:"startedAt"`
	FinishedAt *time.Time           `json:"finishedAt,omitempty"`
	Steps      []Step               `json:"steps"`
	Trace      []TraceEntry         `json:"trace"`
	Result     *domain.SearchResult `json:"result,omitempty"`
	Review     string               `json:"review"`
	Error      string               `json:"error"`
}

type Searcher interface {
	SearchWithOptions(ctx context.Context, apiKey, country, state, area string, profile domain.SearchProfile, options domain.SearchOptions) (domain.SearchResult, error)
}

type Reviewer interface {
	Review(ctx context.Context, config domain.AgentConfig, profile domain.SearchProfile, jobs []domain.SearchJob) (string, error)
}

type Params struct {
	APIKey  string
	Country string
	State   string
	Area    string
	Profile domain.SearchProfile
	Config  domain.AgentConfig
}

type Manager struct {
	searcher Searcher
	reviewer Reviewer
	mu       sync.Mutex
	runs     map[string]*Run
	order    []string
	cancels  map[string]context.CancelFunc
	active   bool
}

func NewManager(searcher Searcher, reviewer Reviewer) *Manager {
	return &Manager{searcher: searcher, reviewer: reviewer, runs: map[string]*Run{}, cancels: map[string]context.CancelFunc{}}
}

func newSteps(llmReview bool) []Step {
	steps := []Step{
		{ID: "plan", Title: "Plan", Description: "Build search queries from your role, skills and location"},
		{ID: "search", Title: "Search", Description: "Query the web, including job-board hosts, in bounded rounds"},
		{ID: "verify", Title: "Verify", Description: "Read each page, honoring robots.txt, and keep only verified postings"},
		{ID: "filter", Title: "Score and filter", Description: "Score against your resume locally and drop weak matches"},
		{ID: "review", Title: "Review", Description: "Optional local-model summary of the best matches"},
	}
	for i := range steps {
		steps[i].Status = StepPending
	}
	if !llmReview {
		last := &steps[len(steps)-1]
		last.Status, last.Detail = StepSkipped, "disabled in agent settings"
	}
	return steps
}

// Start launches a run in the background. Only one run may be active at a time.
func (manager *Manager) Start(params Params) (Run, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.active {
		return Run{}, ErrBusy
	}
	idBytes := make([]byte, 8)
	if _, err := rand.Read(idBytes); err != nil {
		return Run{}, fmt.Errorf("generate run ID: %w", err)
	}
	run := &Run{
		ID: hex.EncodeToString(idBytes), Status: StatusRunning,
		Country: params.Country, State: params.State, Area: params.Area,
		StartedAt: time.Now().UTC(), Steps: newSteps(params.Config.LLMReview), Trace: []TraceEntry{},
	}
	manager.runs[run.ID] = run
	manager.order = append(manager.order, run.ID)
	for len(manager.order) > maxKeptRuns {
		delete(manager.runs, manager.order[0])
		manager.order = manager.order[1:]
	}
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	manager.cancels[run.ID] = cancel
	manager.active = true
	go manager.execute(ctx, run.ID, params)
	return snapshot(run), nil
}

func (manager *Manager) Get(id string) (Run, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	run, ok := manager.runs[id]
	if !ok {
		return Run{}, ErrNotFound
	}
	return snapshot(run), nil
}

// List returns recent runs, newest first, without traces or full results.
func (manager *Manager) List() []Run {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	runs := make([]Run, 0, len(manager.order))
	for i := len(manager.order) - 1; i >= 0; i-- {
		run := snapshot(manager.runs[manager.order[i]])
		run.Trace, run.Result = nil, nil
		runs = append(runs, run)
	}
	return runs
}

func (manager *Manager) Cancel(id string) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if _, ok := manager.runs[id]; !ok {
		return ErrNotFound
	}
	if cancel, ok := manager.cancels[id]; ok {
		cancel()
	}
	return nil
}

func snapshot(run *Run) Run {
	copied := *run
	copied.Steps = append(make([]Step, 0, len(run.Steps)), run.Steps...)
	copied.Trace = append(make([]TraceEntry, 0, len(run.Trace)), run.Trace...)
	if run.Result != nil {
		result := *run.Result
		copied.Result = &result
	}
	return copied
}

func (manager *Manager) update(id string, change func(*Run)) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if run, ok := manager.runs[id]; ok {
		change(run)
	}
}

func stepIndex(run *Run, id string) int {
	for i := range run.Steps {
		if run.Steps[i].ID == id {
			return i
		}
	}
	return -1
}

func startStep(run *Run, id, detail string) {
	if i := stepIndex(run, id); i >= 0 {
		step := &run.Steps[i]
		if step.Status == StepPending {
			now := time.Now().UTC()
			step.Status, step.StartedAt = StepRunning, &now
		}
		if detail != "" {
			step.Detail = detail
		}
	}
}

func finishStep(run *Run, id string, status StepStatus, detail string) {
	i := stepIndex(run, id)
	if i < 0 {
		return
	}
	step := &run.Steps[i]
	finished := step.Status == StepDone || step.Status == StepSkipped || step.Status == StepFailed
	if finished && detail == "" {
		return
	}
	if !finished && step.StartedAt != nil {
		step.DurationMs = time.Since(*step.StartedAt).Milliseconds()
	}
	step.Status = status
	if detail != "" {
		step.Detail = detail
	}
}

func addTrace(run *Run, stage, message string) {
	if len(run.Trace) >= maxTrace {
		return
	}
	run.Trace = append(run.Trace, TraceEntry{Time: time.Now().UTC(), Stage: stage, Message: message})
}

func (manager *Manager) execute(ctx context.Context, id string, params Params) {
	defer func() {
		manager.mu.Lock()
		if cancel, ok := manager.cancels[id]; ok {
			cancel()
			delete(manager.cancels, id)
		}
		manager.active = false
		manager.mu.Unlock()
	}()

	manager.update(id, func(run *Run) {
		startStep(run, "plan", "")
		addTrace(run, "plan", fmt.Sprintf("starting search for %s", strings.Join(nonEmpty(params.Area, params.State, params.Country), ", ")))
	})
	options := domain.SearchOptions{
		MaxRounds: params.Config.MaxRounds, TargetMatches: params.Config.TargetMatches,
		MaxPages: params.Config.MaxPages, MinScore: params.Config.MinScore,
		OnEvent: func(event domain.SearchEvent) {
			manager.update(id, func(run *Run) {
				addTrace(run, event.Stage, event.Message)
				switch event.Stage {
				case "plan":
					finishStep(run, "plan", StepDone, event.Message)
				case "search":
					startStep(run, "search", event.Message)
				case "verify":
					startStep(run, "search", "")
					startStep(run, "verify", event.Message)
				case "filter":
					startStep(run, "filter", event.Message)
				}
			})
		},
	}
	result, err := manager.searcher.SearchWithOptions(ctx, params.APIKey, params.Country, params.State, params.Area, params.Profile, options)
	if err != nil {
		manager.fail(ctx, id, err)
		return
	}
	manager.update(id, func(run *Run) {
		finishStep(run, "plan", StepDone, "")
		finishStep(run, "search", StepDone, fmt.Sprintf("%d pages read", result.CheckedSources))
		finishStep(run, "verify", StepDone, fmt.Sprintf("%d verified jobs, %d pages skipped", len(result.Jobs)+result.FilteredLowMatch, len(result.SourceErrors)))
		if i := stepIndex(run, "filter"); i >= 0 && run.Steps[i].StartedAt == nil {
			run.Steps[i].Status = StepSkipped
			run.Steps[i].Detail = "save skills or a resume in your profile to score matches"
		} else {
			finishStep(run, "filter", StepDone, "")
		}
		copied := result
		run.Result = &copied
	})
	if params.Config.LLMReview && len(result.Jobs) > 0 {
		manager.review(ctx, id, params, result)
	} else if params.Config.LLMReview {
		manager.update(id, func(run *Run) {
			finishStep(run, "review", StepSkipped, "no matching jobs to review")
		})
	}
	manager.update(id, func(run *Run) {
		now := time.Now().UTC()
		run.Status, run.FinishedAt = StatusCompleted, &now
		addTrace(run, "done", fmt.Sprintf("finished with %d matching jobs", len(result.Jobs)))
	})
}

func (manager *Manager) review(ctx context.Context, id string, params Params, result domain.SearchResult) {
	manager.update(id, func(run *Run) { startStep(run, "review", "asking the local model to summarize the best matches") })
	reviewCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	jobs := result.Jobs
	if len(jobs) > 8 {
		jobs = jobs[:8]
	}
	text, err := manager.reviewer.Review(reviewCtx, params.Config, params.Profile, jobs)
	manager.update(id, func(run *Run) {
		if err != nil {
			finishStep(run, "review", StepSkipped, "local model unavailable: "+err.Error())
			addTrace(run, "review", "review skipped: "+err.Error())
			return
		}
		run.Review = text
		finishStep(run, "review", StepDone, "summary ready")
		addTrace(run, "review", "local model summary ready")
	})
}

func (manager *Manager) fail(ctx context.Context, id string, err error) {
	manager.update(id, func(run *Run) {
		now := time.Now().UTC()
		run.FinishedAt = &now
		status, message := StatusFailed, err.Error()
		if errors.Is(ctx.Err(), context.Canceled) {
			status, message = StatusCancelled, "run cancelled"
		} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			message = "run exceeded its time limit"
		}
		run.Status, run.Error = status, message
		for i := range run.Steps {
			if run.Steps[i].Status == StepRunning || run.Steps[i].Status == StepPending {
				finishStep(run, run.Steps[i].ID, StepFailed, message)
			}
		}
		addTrace(run, "error", message)
	})
}

func nonEmpty(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

package workflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"careerpilot/contracts/domain"
)

type DiscoveryData struct {
	Sources       []domain.CareerSource
	Profile       domain.CandidateProfile
	Area          string
	IncludeRemote bool
	RemoteRegion  string
	Jobs          []domain.Job
	SourceErrors  []domain.SourceError
}

type StageStatus string

const (
	StagePending  StageStatus = "pending"
	StageRunning  StageStatus = "running"
	StageComplete StageStatus = "complete"
	StageFailed   StageStatus = "failed"
)

type StageEvent struct {
	Name      string      `json:"name"`
	Status    StageStatus `json:"status"`
	StartedAt time.Time   `json:"startedAt"`
	EndedAt   time.Time   `json:"endedAt,omitempty"`
	Error     string      `json:"error,omitempty"`
}

type Fetcher func(context.Context, domain.CareerSource, domain.CandidateProfile) ([]domain.Job, error)
type Stage func(context.Context, *DiscoveryData, Fetcher) error

type Engine struct {
	stages []struct {
		name string
		run  Stage
	}
}

func NewDiscoveryEngine() *Engine {
	return &Engine{stages: []struct {
		name string
		run  Stage
	}{
		{name: "validate_sources", run: func(ctx context.Context, data *DiscoveryData, _ Fetcher) error {
			return validateInput(ctx, data)
		}},
		{name: "fetch_public_boards", run: func(ctx context.Context, data *DiscoveryData, fetch Fetcher) error {
			return fetchJobs(ctx, data, fetch)
		}},
		{name: "normalize_and_deduplicate", run: func(ctx context.Context, data *DiscoveryData, _ Fetcher) error {
			return deduplicate(ctx, data)
		}},
		{name: "filter_selected_area", run: func(ctx context.Context, data *DiscoveryData, _ Fetcher) error {
			return filterArea(ctx, data)
		}},
		{name: "rank_matches", run: func(ctx context.Context, data *DiscoveryData, _ Fetcher) error {
			return rank(ctx, data)
		}},
	}}
}

func filterArea(_ context.Context, data *DiscoveryData) error {
	area := strings.ToLower(strings.TrimSpace(data.Area))
	if area == "" {
		return nil
	}
	remoteRegion := strings.ToLower(strings.TrimSpace(data.RemoteRegion))
	filtered := make([]domain.Job, 0, len(data.Jobs))
	for _, job := range data.Jobs {
		location := strings.ToLower(strings.TrimSpace(job.Location))
		localMatch := strings.Contains(location, area)
		remoteMatch := data.IncludeRemote && remoteRegion != "" &&
			(strings.Contains(location, "remote") || strings.Contains(location, "distributed")) &&
			strings.Contains(location, remoteRegion)
		if localMatch || remoteMatch {
			filtered = append(filtered, job)
		}
	}
	data.Jobs = filtered
	return nil
}

func (engine *Engine) Run(ctx context.Context, data *DiscoveryData, fetch Fetcher) ([]StageEvent, error) {
	if data.Jobs == nil {
		data.Jobs = []domain.Job{}
	}
	if data.SourceErrors == nil {
		data.SourceErrors = []domain.SourceError{}
	}
	events := make([]StageEvent, 0, len(engine.stages))
	for _, stage := range engine.stages {
		event := StageEvent{Name: stage.name, Status: StageRunning, StartedAt: time.Now().UTC()}
		err := stage.run(ctx, data, fetch)
		event.EndedAt = time.Now().UTC()
		if err != nil {
			event.Status = StageFailed
			event.Error = err.Error()
			events = append(events, event)
			return events, fmt.Errorf("workflow stage %s: %w", stage.name, err)
		}
		event.Status = StageComplete
		events = append(events, event)
	}
	return events, nil
}

func validateInput(_ context.Context, data *DiscoveryData) error {
	if len(data.Sources) == 0 {
		return fmt.Errorf("no career sources are configured")
	}
	return nil
}

func deduplicate(_ context.Context, data *DiscoveryData) error {
	seen := make(map[string]struct{}, len(data.Jobs))
	jobs := make([]domain.Job, 0, len(data.Jobs))
	for _, job := range data.Jobs {
		if _, ok := seen[job.ID]; ok {
			continue
		}
		seen[job.ID] = struct{}{}
		jobs = append(jobs, job)
	}
	data.Jobs = jobs
	return nil
}

func rank(_ context.Context, data *DiscoveryData) error {
	if len(data.Jobs) > 1 {
		sortJobs(data.Jobs)
	}
	if len(data.Jobs) > 300 {
		data.Jobs = data.Jobs[:300]
	}
	return nil
}

package workflow

import (
	"context"
	"errors"
	"testing"

	"careerpilot/contracts/domain"
)

func TestEngineRunsStagesDeduplicatesAndRanksJobs(t *testing.T) {
	data := &DiscoveryData{Sources: []domain.CareerSource{{Company: "Example"}}}
	engine := NewDiscoveryEngine()
	events, err := engine.Run(context.Background(), data, func(context.Context, domain.CareerSource, domain.CandidateProfile) ([]domain.Job, error) {
		return []domain.Job{
			{ID: "low", Score: 10},
			{ID: "high", Score: 90},
			{ID: "high", Score: 90},
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(events) != 5 || len(data.Jobs) != 2 || data.Jobs[0].ID != "high" {
		t.Fatalf("unexpected workflow result: events=%#v jobs=%#v", events, data.Jobs)
	}
	for _, event := range events {
		if event.Status != StageComplete {
			t.Fatalf("stage %q not complete: %s", event.Name, event.Status)
		}
	}
}

func TestDiscoveryFiltersListingsToSelectedAreaAndOptionalRemote(t *testing.T) {
	tests := []struct {
		name          string
		includeRemote bool
		wantIDs       []string
	}{
		{name: "local area only", wantIDs: []string{"local"}},
		{name: "local area and remote", includeRemote: true, wantIDs: []string{"local", "remote"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := &DiscoveryData{
				Area: "Bengaluru", IncludeRemote: test.includeRemote,
				Jobs: []domain.Job{
					{ID: "local", Location: "Bengaluru, India"},
					{ID: "remote", Location: "Remote - India"},
					{ID: "remote-other-country", Location: "US-Remote"},
					{ID: "other-city", Location: "Hyderabad, India"},
				},
				RemoteRegion: "India",
			}
			if err := filterArea(context.Background(), data); err != nil {
				t.Fatal(err)
			}
			if len(data.Jobs) != len(test.wantIDs) {
				t.Fatalf("got %d jobs, want %d", len(data.Jobs), len(test.wantIDs))
			}
			for index, want := range test.wantIDs {
				if data.Jobs[index].ID != want {
					t.Errorf("job %d ID = %q, want %q", index, data.Jobs[index].ID, want)
				}
			}
		})
	}
	data := &DiscoveryData{
		Jobs: []domain.Job{{ID: "unfiltered", Location: "Anywhere"}},
	}
	if err := filterArea(context.Background(), data); err != nil || len(data.Jobs) != 1 {
		t.Fatalf("empty area should preserve all jobs: %#v, %v", data.Jobs, err)
	}
}

func TestEngineReturnsSourceFailuresInResultWithoutFailingAllSources(t *testing.T) {
	data := &DiscoveryData{Sources: []domain.CareerSource{{Company: "Down"}}}
	_, err := NewDiscoveryEngine().Run(context.Background(), data, func(context.Context, domain.CareerSource, domain.CandidateProfile) ([]domain.Job, error) {
		return nil, errors.New("source unavailable")
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(data.SourceErrors) != 1 || data.SourceErrors[0].Company != "Down" {
		t.Fatalf("source failure was not surfaced: %#v", data.SourceErrors)
	}
}

func TestEngineRejectsEmptySourceList(t *testing.T) {
	_, err := NewDiscoveryEngine().Run(context.Background(), &DiscoveryData{}, func(context.Context, domain.CareerSource, domain.CandidateProfile) ([]domain.Job, error) {
		t.Fatal("fetch should not run without sources")
		return nil, nil
	})
	if err == nil {
		t.Fatal("expected empty source list to fail")
	}
}

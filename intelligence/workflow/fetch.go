package workflow

import (
	"context"
	"sort"
	"sync"

	"careerpilot/contracts/domain"
)

type outcome struct {
	jobs  []domain.Job
	error error
}

func fetchJobs(ctx context.Context, data *DiscoveryData, fetch Fetcher) error {
	outcomes := make([]outcome, len(data.Sources))
	var group sync.WaitGroup
	semaphore := make(chan struct{}, 6)
	for index, source := range data.Sources {
		group.Add(1)
		go func(index int, source domain.CareerSource) {
			defer group.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				outcomes[index].error = ctx.Err()
				return
			}
			outcomes[index].jobs, outcomes[index].error = fetch(ctx, source, data.Profile)
		}(index, source)
	}
	group.Wait()
	if err := ctx.Err(); err != nil {
		return err
	}
	for index, result := range outcomes {
		if result.error != nil {
			data.SourceErrors = append(data.SourceErrors, domain.SourceError{
				Company: data.Sources[index].Company,
				Error:   result.error.Error(),
			})
			continue
		}
		data.Jobs = append(data.Jobs, result.jobs...)
	}
	return nil
}

func sortJobs(jobs []domain.Job) {
	sort.SliceStable(jobs, func(i, j int) bool { return jobs[i].Score > jobs[j].Score })
}

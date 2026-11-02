package linkedin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"careerpilot/contracts/domain"
)

var ErrJobsProductAccessRequired = errors.New("LinkedIn Jobs API requires an approved product and endpoint configuration")

type JobSearch struct {
	Keywords string
	Location string
	Start    int
	Count    int
}

type JobMapper interface {
	MapJobs(json.RawMessage) ([]domain.Job, error)
}

type JobsClient struct {
	client   *Client
	endpoint string
	mapper   JobMapper
}

func NewJobsClient(client *Client, approvedEndpoint string, mapper JobMapper) (*JobsClient, error) {
	if client == nil || mapper == nil {
		return nil, fmt.Errorf("LinkedIn API client and job mapper are required")
	}
	endpoint := strings.TrimSpace(approvedEndpoint)
	if endpoint == "" {
		return nil, ErrJobsProductAccessRequired
	}
	if !strings.HasPrefix(endpoint, "/") || strings.HasPrefix(endpoint, "//") {
		return nil, fmt.Errorf("approved LinkedIn jobs endpoint must be a relative API path")
	}
	return &JobsClient{client: client, endpoint: endpoint, mapper: mapper}, nil
}

func (jobs *JobsClient) Search(ctx context.Context, search JobSearch) ([]domain.Job, error) {
	if jobs == nil {
		return nil, ErrJobsProductAccessRequired
	}
	if search.Start < 0 {
		return nil, fmt.Errorf("job search start must not be negative")
	}
	if search.Count < 0 || search.Count > 100 {
		return nil, fmt.Errorf("job search count must be between 0 and 100")
	}
	query := make(url.Values)
	if search.Keywords != "" {
		query.Set("keywords", search.Keywords)
	}
	if search.Location != "" {
		query.Set("location", search.Location)
	}
	if search.Start > 0 {
		query.Set("start", fmt.Sprint(search.Start))
	}
	if search.Count > 0 {
		query.Set("count", fmt.Sprint(search.Count))
	}
	var payload json.RawMessage
	if err := jobs.client.getJSON(ctx, jobs.endpoint, query, &payload); err != nil {
		return nil, fmt.Errorf("search LinkedIn jobs: %w", err)
	}
	listings, err := jobs.mapper.MapJobs(payload)
	if err != nil {
		return nil, fmt.Errorf("map LinkedIn jobs response: %w", err)
	}
	return listings, nil
}

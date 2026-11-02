package lever

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"careerpilot/contracts/domain"
	commonjobs "careerpilot/platform/internal/integrations/jobs"
)

func (client *Client) Name() domain.Provider {
	return domain.ProviderLever
}

func (client *Client) Fetch(ctx context.Context, source domain.CareerSource, profile domain.CandidateProfile) ([]domain.Job, error) {
	source, err := commonjobs.ValidateSource(source)
	if err != nil {
		return nil, err
	}
	if source.Provider != domain.ProviderLever {
		return nil, fmt.Errorf("Lever client cannot fetch provider %q", source.Provider)
	}
	return client.Jobs(ctx, source, profile)
}

func (client *Client) Jobs(ctx context.Context, source domain.CareerSource, profile domain.CandidateProfile) ([]domain.Job, error) {
	if err := commonjobs.ValidateBoardID(source.BoardID); err != nil {
		return nil, err
	}
	endpoint := "https://api.lever.co/v0/postings/" + url.PathEscape(source.BoardID) + "?mode=json"
	var response []posting
	if err := commonjobs.FetchJSON(ctx, client.httpClient, endpoint, "lever", source.Company, &response); err != nil {
		return nil, err
	}

	result := make([]domain.Job, 0, len(response))
	for _, posting := range response {
		job, ok := mapPosting(source, posting, profile)
		if ok {
			result = append(result, job)
		}
	}
	return result, nil
}

func postingDate(createdAt int64) string {
	if createdAt <= 0 {
		return ""
	}
	return time.UnixMilli(createdAt).UTC().Format(time.RFC3339)
}

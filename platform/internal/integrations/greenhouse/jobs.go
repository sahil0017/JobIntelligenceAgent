package greenhouse

import (
	"context"
	"fmt"
	"net/url"

	"careerpilot/contracts/domain"
	commonjobs "careerpilot/platform/internal/integrations/jobs"
)

func (client *Client) Name() domain.Provider {
	return domain.ProviderGreenhouse
}

func (client *Client) Fetch(ctx context.Context, source domain.CareerSource, profile domain.CandidateProfile) ([]domain.Job, error) {
	source, err := commonjobs.ValidateSource(source)
	if err != nil {
		return nil, err
	}
	if source.Provider != domain.ProviderGreenhouse {
		return nil, fmt.Errorf("Greenhouse client cannot fetch provider %q", source.Provider)
	}
	return client.Jobs(ctx, source, profile)
}

func (client *Client) Jobs(ctx context.Context, source domain.CareerSource, profile domain.CandidateProfile) ([]domain.Job, error) {
	if err := commonjobs.ValidateBoardID(source.BoardID); err != nil {
		return nil, err
	}
	endpoint := "https://boards-api.greenhouse.io/v1/boards/" + url.PathEscape(source.BoardID) + "/jobs?content=true"
	var response boardResponse
	if err := commonjobs.FetchJSON(ctx, client.httpClient, endpoint, "greenhouse", source.Company, &response); err != nil {
		return nil, err
	}

	result := make([]domain.Job, 0, len(response.Jobs))
	for _, posting := range response.Jobs {
		job, ok := mapPosting(source, posting, profile)
		if ok {
			result = append(result, job)
		}
	}
	return result, nil
}

package lever

import (
	"careerpilot/contracts/domain"
	commonjobs "careerpilot/platform/internal/integrations/jobs"
)

func mapPosting(source domain.CareerSource, posting posting, profile domain.CandidateProfile) (domain.Job, bool) {
	description := posting.DescriptionPlain
	if description == "" {
		description = posting.Description
	}
	return commonjobs.MapJob(
		source,
		posting.ID,
		posting.Text,
		posting.Categories.Location,
		posting.HostedURL,
		description,
		postingDate(posting.CreatedAt),
		profile,
	)
}

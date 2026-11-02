package greenhouse

import (
	"fmt"

	"careerpilot/contracts/domain"
	commonjobs "careerpilot/platform/internal/integrations/jobs"
)

func mapPosting(source domain.CareerSource, posting posting, profile domain.CandidateProfile) (domain.Job, bool) {
	return commonjobs.MapJob(
		source,
		fmt.Sprint(posting.ID),
		posting.Title,
		posting.Location.Name,
		posting.AbsoluteURL,
		posting.Content,
		posting.FirstPublished,
		profile,
	)
}

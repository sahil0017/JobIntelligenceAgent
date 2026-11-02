package workflow

import "careerpilot/contracts/domain"

// DiscoveryRequest is the input shared by the discovery service and the discovery agent.
type DiscoveryRequest struct {
	Sources       []domain.CareerSource   `json:"sources"`
	Profile       domain.CandidateProfile `json:"profile"`
	Area          string                  `json:"area,omitempty"`
	IncludeRemote bool                    `json:"includeRemote,omitempty"`
	RemoteRegion  string                  `json:"remoteRegion,omitempty"`
}

// DiscoveryOutput is a discovery result together with the stages that produced it.
type DiscoveryOutput struct {
	domain.DiscoveryResult
	Workflow []StageEvent `json:"workflow"`
	Area     string       `json:"area,omitempty"`
}

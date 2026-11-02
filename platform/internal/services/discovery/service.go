package discovery

import (
	"context"
	"encoding/json"
	"fmt"

	"careerpilot/contracts/domain"
	"careerpilot/intelligence/workflow"
	"careerpilot/platform/internal/integrations/jobs"
)

type Service struct {
	providers *jobs.Registry
	engine    *workflow.Engine
}

type Request = workflow.DiscoveryRequest

type Result = workflow.DiscoveryOutput

func New(providers *jobs.Registry, engine *workflow.Engine) *Service {
	return &Service{providers: providers, engine: engine}
}

func (service *Service) Run(ctx context.Context, input json.RawMessage) (any, error) {
	var request Request
	if err := json.Unmarshal(input, &request); err != nil {
		return nil, fmt.Errorf("decode discovery input: %w", err)
	}
	for index, source := range request.Sources {
		validated, err := jobs.ValidateSource(source)
		if err != nil {
			return nil, fmt.Errorf("source %q: %w", source.Company, err)
		}
		request.Sources[index] = validated
	}
	data := &workflow.DiscoveryData{
		Sources: request.Sources, Profile: request.Profile,
		Area: request.Area, IncludeRemote: request.IncludeRemote, RemoteRegion: request.RemoteRegion,
	}
	events, err := service.engine.Run(ctx, data, func(ctx context.Context, source domain.CareerSource, profile domain.CandidateProfile) ([]domain.Job, error) {
		provider, ok := service.providers.Get(source.Provider)
		if !ok {
			return nil, fmt.Errorf("provider %q is not registered", source.Provider)
		}
		return provider.Fetch(ctx, source, profile)
	})
	if err != nil {
		return nil, err
	}
	return Result{
		DiscoveryResult: domain.DiscoveryResult{
			Jobs:           data.Jobs,
			SourceErrors:   data.SourceErrors,
			CheckedSources: len(request.Sources),
		},
		Workflow: events,
		Area:     request.Area,
	}, nil
}

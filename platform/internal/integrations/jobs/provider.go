package jobs

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"careerpilot/contracts/domain"
)

type Provider interface {
	Name() domain.Provider
	Fetch(context.Context, domain.CareerSource, domain.CandidateProfile) ([]domain.Job, error)
}

type Registry struct {
	providers map[domain.Provider]Provider
}

func NewRegistry(providers ...Provider) (*Registry, error) {
	registry := &Registry{providers: make(map[domain.Provider]Provider, len(providers))}
	for _, provider := range providers {
		if provider == nil {
			return nil, fmt.Errorf("job provider must not be nil")
		}
		name := provider.Name()
		if name == "" {
			return nil, fmt.Errorf("job provider name must not be empty")
		}
		if _, exists := registry.providers[name]; exists {
			return nil, fmt.Errorf("job provider %q is already registered", name)
		}
		registry.providers[name] = provider
	}
	return registry, nil
}

func (registry *Registry) Get(provider domain.Provider) (Provider, bool) {
	implementation, exists := registry.providers[provider]
	return implementation, exists
}

func ValidateSource(source domain.CareerSource) (domain.CareerSource, error) {
	source.Provider = domain.Provider(strings.ToLower(strings.TrimSpace(string(source.Provider))))
	source.Company = strings.TrimSpace(source.Company)
	source.BoardID = strings.TrimSpace(source.BoardID)
	if source.Provider != domain.ProviderGreenhouse && source.Provider != domain.ProviderLever {
		return source, fmt.Errorf("provider must be Greenhouse or Lever")
	}
	if source.Company == "" || len(source.Company) > 100 {
		return source, fmt.Errorf("company name must be between 1 and 100 characters")
	}
	if !boardIDPattern.MatchString(source.BoardID) {
		return source, fmt.Errorf("board ID may contain only letters, numbers, dots, underscores, and hyphens")
	}
	return source, nil
}

var boardIDPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,100}$`)

func NewHTTPClient(client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	return &http.Client{Timeout: 15 * time.Second}
}

package jobs

import (
	"context"
	"testing"

	"careerpilot/contracts/domain"
)

func TestRegistryRejectsDuplicateProviders(t *testing.T) {
	first := testProvider(domain.ProviderGreenhouse)
	second := testProvider(domain.ProviderGreenhouse)
	if _, err := NewRegistry(first, second); err == nil {
		t.Fatal("expected duplicate provider registration to fail")
	}
}

func TestRegistryReturnsRegisteredProvider(t *testing.T) {
	provider := testProvider(domain.ProviderLever)
	registry, err := NewRegistry(provider)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := registry.Get(domain.ProviderLever)
	if !ok || got != provider {
		t.Fatalf("registry returned %v, %v", got, ok)
	}
}

func TestValidateSourceRejectsUnsupportedAndUnsafeSource(t *testing.T) {
	if _, err := ValidateSource(domain.CareerSource{Provider: "linkedin", Company: "LinkedIn", BoardID: "jobs"}); err == nil {
		t.Fatal("expected unconfigured provider to be rejected")
	}
	if _, err := ValidateSource(domain.CareerSource{Provider: domain.ProviderGreenhouse, Company: "Example", BoardID: "https://example.com"}); err == nil {
		t.Fatal("expected URL-shaped board ID to be rejected")
	}
}

func testProvider(name domain.Provider) Provider {
	return providerFunc{name: name}
}

type providerFunc struct {
	name domain.Provider
}

func (provider providerFunc) Name() domain.Provider { return provider.name }
func (provider providerFunc) Fetch(context.Context, domain.CareerSource, domain.CandidateProfile) ([]domain.Job, error) {
	return nil, nil
}

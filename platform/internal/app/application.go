package app

import (
	"careerpilot/intelligence/runs"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"careerpilot/intelligence/agents"
	"careerpilot/intelligence/tools"
	"careerpilot/intelligence/workflow"
	"careerpilot/platform/internal/api"
	"careerpilot/platform/internal/integrations/cortex"
	"careerpilot/platform/internal/integrations/gmail"
	"careerpilot/platform/internal/integrations/googleplaces"
	"careerpilot/platform/internal/integrations/greenhouse"
	"careerpilot/platform/internal/integrations/jobs"
	"careerpilot/platform/internal/integrations/lever"
	"careerpilot/platform/internal/integrations/linkedin"
	"careerpilot/platform/internal/integrations/webjobs"
	"careerpilot/platform/internal/services/discovery"
	"careerpilot/platform/internal/services/monitor"
	"careerpilot/platform/internal/storage"
)

func NewHandler(sourceStorePath string) (http.Handler, error) {
	handler, _, err := NewHandlerWithMonitor(sourceStorePath)
	return handler, err
}

func NewHandlerWithMonitor(sourceStorePath string) (http.Handler, func(context.Context), error) {
	sourceStore, err := api.NewSourceStore(sourceStorePath)
	if err != nil {
		return nil, nil, fmt.Errorf("create source store: %w", err)
	}

	providers, err := jobs.NewRegistry(
		greenhouse.NewClient(nil),
		lever.NewClient(nil),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("register job providers: %w", err)
	}
	discoverer := discovery.New(providers, workflow.NewDiscoveryEngine())
	toolRegistry := tools.NewRegistry()
	if err := toolRegistry.Register(tools.Tool{
		Name:        "discover_jobs",
		Description: "Read public job board listings and rank them against a candidate profile.",
		Risk:        tools.RiskReadOnly,
		Handler:     discoverer.Run,
	}); err != nil {
		return nil, nil, fmt.Errorf("register discovery tool: %w", err)
	}

	discoveryAgent, err := agents.NewDiscoveryAgent(
		envOrDefault("OLLAMA_URL", "http://127.0.0.1:11434"),
		envOrDefault("OLLAMA_MODEL", "qwen2.5:7b"),
		sourceStore,
		toolRegistry,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("configure discovery agent: %w", err)
	}

	gmailConfig, err := loadIntegrationConfig(sourceStore.Database(), "gmail")
	if err != nil {
		return nil, nil, err
	}
	gmailIntegration := gmail.NewUnconfigured()
	if gmailConfig.ClientID != "" || gmailConfig.ClientSecret != "" || gmailConfig.RedirectURL != "" {
		if err := gmailIntegration.Configure(gmailConfig); err != nil {
			return nil, nil, fmt.Errorf("configure Gmail integration: %w", err)
		}
	}

	linkedinConfig, err := loadIntegrationConfig(sourceStore.Database(), "linkedin")
	if err != nil {
		return nil, nil, err
	}
	linkedinIntegration, err := linkedin.NewManager(linkedinConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("configure LinkedIn integration: %w", err)
	}
	service := monitor.New(sourceStore.Database(), providers, gmailIntegration)
	startMonitoring := func(ctx context.Context) {
		go service.Run(ctx)
	}
	webJobs := webjobs.NewClient(nil)
	aiClient, err := cortex.NewClient("", nil)
	if err != nil {
		return nil, nil, fmt.Errorf("configure AI service: %w", err)
	}
	return api.New(sourceStore, toolRegistry, api.Services{
		AI:             aiClient,
		Gmail:          gmailIntegration,
		LinkedIn:       linkedinIntegration,
		Places:         googleplaces.NewClient(nil),
		WebJobs:        webJobs,
		AgentRuns:      runs.NewManager(webJobs, runs.NewOllamaReviewer()),
		DiscoveryAgent: discoveryAgent,
	}).Handler(), startMonitoring, nil
}

func loadIntegrationConfig(store interface {
	LoadIntegration(string) (storage.IntegrationConfig, error)
	SaveIntegration(storage.IntegrationConfig) error
}, provider string) (storage.IntegrationConfig, error) {
	config, err := store.LoadIntegration(provider)
	if err != nil {
		return storage.IntegrationConfig{}, err
	}
	if config.ClientID != "" {
		return config, nil
	}
	prefix := strings.ToUpper(provider) + "_OAUTH_"
	config = storage.IntegrationConfig{
		Provider:     provider,
		ClientID:     strings.TrimSpace(os.Getenv(prefix + "CLIENT_ID")),
		ClientSecret: strings.TrimSpace(os.Getenv(prefix + "CLIENT_SECRET")),
		RedirectURL:  strings.TrimSpace(os.Getenv(prefix + "REDIRECT_URL")),
	}
	if config.ClientID == "" && config.ClientSecret == "" && config.RedirectURL == "" {
		return config, nil
	}
	if config.ClientID == "" || config.ClientSecret == "" || config.RedirectURL == "" {
		return storage.IntegrationConfig{}, fmt.Errorf("%s OAuth environment configuration is incomplete", provider)
	}
	if err := store.SaveIntegration(config); err != nil {
		return storage.IntegrationConfig{}, err
	}
	return config, nil
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

package api

import (
	"careerpilot/contracts/domain"
	"careerpilot/intelligence/agents"
	"careerpilot/platform/internal/integrations/cortex"
	"careerpilot/platform/internal/integrations/gmail"
	"careerpilot/platform/internal/integrations/linkedin"
	"careerpilot/platform/internal/integrations/webjobs"
	"careerpilot/platform/internal/storage"
	"context"
	"encoding/json"
)

type GmailIntegration interface {
	StartAuthorization() (string, error)
	CompleteAuthorization(context.Context, string, string) error
	Configure(storage.IntegrationConfig) error
	Configured() bool
	Connected() bool
	Disconnect()
	SearchThreads(context.Context, string, int) ([]gmail.ThreadSummary, error)
	SendMessage(context.Context, string, string, string) (string, error)
	EmailAddress(context.Context) (string, error)
}

type LinkedInIntegration interface {
	Configure(storage.IntegrationConfig) error
	Configured() bool
	StartAuthorization() (string, error)
	CompleteAuthorization(context.Context, string, string) (linkedin.Profile, error)
	Profile() (linkedin.Profile, bool)
	Disconnect()
}

type DiscoveryAgent interface {
	Run(context.Context, agents.DiscoveryRequest) (agents.DiscoveryResponse, error)
}

type CompanySearch interface {
	Search(context.Context, string, string, int) ([]domain.Company, error)
}

type AreaJobSearch interface {
	Search(context.Context, string, string, string, string, webjobs.Profile) (webjobs.Result, error)
}

// AIAnalyzer is the Python Cortex service. Results are opaque JSON that the console renders.
type AIAnalyzer interface {
	Health(context.Context) (json.RawMessage, error)
	Pipeline(context.Context) (json.RawMessage, error)
	Start(context.Context, cortex.AnalysisRequest) (json.RawMessage, error)
	List(context.Context) (json.RawMessage, error)
	Get(context.Context, string) (json.RawMessage, error)
	Cancel(context.Context, string) (json.RawMessage, error)
	ApplicationPipeline(context.Context) (json.RawMessage, error)
	StartApplication(context.Context, cortex.ApplicationRequest) (json.RawMessage, error)
	Applications(context.Context) (json.RawMessage, error)
	Application(context.Context, string) (json.RawMessage, error)
	DecideApplication(context.Context, string, cortex.Decision) (json.RawMessage, error)
	CancelApplication(context.Context, string) (json.RawMessage, error)
	ApplyApplication(context.Context, string, cortex.Applicant) (json.RawMessage, error)
	QuickApply(context.Context, cortex.QuickApply) (json.RawMessage, error)
	PortalSearch(context.Context, cortex.PortalSearchRequest) (json.RawMessage, error)
	PortalSession(context.Context) (json.RawMessage, error)
	PortalSignIn(context.Context, string) (json.RawMessage, error)
	PortalSignOut(context.Context) (json.RawMessage, error)
}

type Services struct {
	Gmail          GmailIntegration
	LinkedIn       LinkedInIntegration
	Places         CompanySearch
	WebJobs        AreaJobSearch
	DiscoveryAgent DiscoveryAgent
	AgentRuns      AgentRunner
	AI             AIAnalyzer
}

type integrationConfigRequest struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	RedirectURL  string `json:"redirectUrl"`
}

package api

import (
	"net/http"

	"careerpilot/intelligence/tools"
)

const maxRequestBody = 16 << 10

type API struct {
	sources        *SourceStore
	tools          *tools.Registry
	gmail          GmailIntegration
	linkedin       LinkedInIntegration
	places         CompanySearch
	webJobs        AreaJobSearch
	discoveryAgent DiscoveryAgent
	agentRuns      AgentRunner
	ai             AIAnalyzer
	mux            *http.ServeMux
}

func New(sources *SourceStore, registry *tools.Registry, services ...Services) *API {
	api := &API{sources: sources, tools: registry, mux: http.NewServeMux()}
	if len(services) > 0 {
		api.gmail = services[0].Gmail
		api.linkedin = services[0].LinkedIn
		api.places = services[0].Places
		api.webJobs = services[0].WebJobs
		api.discoveryAgent = services[0].DiscoveryAgent
		api.agentRuns = services[0].AgentRuns
		api.ai = services[0].AI
	}
	api.mux.HandleFunc("/api/ai/status", api.aiStatus)
	api.mux.HandleFunc("/api/ai/pipeline", api.aiPipeline)
	api.mux.HandleFunc("/api/ai/analyses", api.aiAnalysisCollection)
	api.mux.HandleFunc("/api/ai/analyses/", api.aiAnalysisItem)
	api.mux.HandleFunc("/api/ai/application-pipeline", api.aiApplicationPipeline)
	api.mux.HandleFunc("/api/ai/applications", api.aiApplicationCollection)
	api.mux.HandleFunc("/api/ai/applications/", api.aiApplicationItem)
	api.mux.HandleFunc("/api/ai/portals/search", api.aiPortalSearch)
	api.mux.HandleFunc("/api/ai/portals/session", api.aiPortalSession)
	api.mux.HandleFunc("/api/ai/apply", api.aiQuickApply)
	api.mux.HandleFunc("/api/health", api.health)
	api.mux.HandleFunc("/api/sources", api.sourceCollection)
	api.mux.HandleFunc("/api/sources/", api.sourceItem)
	api.mux.HandleFunc("/api/sources-catalog", api.sourceCatalog)
	api.mux.HandleFunc("/api/jobs", api.jobs)
	api.mux.HandleFunc("/api/agent/discover", api.agentDiscover)
	api.mux.HandleFunc("/api/agent/config", api.agentConfig)
	api.mux.HandleFunc("/api/agent/runs", api.agentRunCollection)
	api.mux.HandleFunc("/api/agent/runs/", api.agentRunItem)
	api.mux.HandleFunc("/api/gmail/status", api.gmailStatus)
	api.mux.HandleFunc("/api/gmail/oauth/start", api.gmailOAuthStart)
	api.mux.HandleFunc("/api/gmail/oauth/callback", api.gmailOAuthCallback)
	api.mux.HandleFunc("/api/gmail/threads", api.gmailThreads)
	api.mux.HandleFunc("/api/gmail/send", api.gmailSend)
	api.mux.HandleFunc("/api/gmail/connection", api.gmailConnection)
	api.mux.HandleFunc("/api/applied-jobs", api.appliedJobs)
	api.mux.HandleFunc("/api/applied-jobs/", api.appliedJobItem)
	api.mux.HandleFunc("/api/settings", api.settings)
	api.mux.HandleFunc("/api/settings/places", api.placesSettings)
	api.mux.HandleFunc("/api/settings/web-search", api.webSearchSettings)
	api.mux.HandleFunc("/api/settings/integrations/", api.integrationSettings)
	api.mux.HandleFunc("/api/companies/search", api.companySearch)
	api.mux.HandleFunc("/api/area-jobs", api.areaJobs)
	api.mux.HandleFunc("/api/profile", api.profile)
	api.mux.HandleFunc("/api/resume", api.resume)
	api.mux.HandleFunc("/api/portals", api.portalCollection)
	api.mux.HandleFunc("/api/portals/", api.portalItem)
	api.mux.HandleFunc("/api/linkedin/oauth/start", api.linkedinOAuthStart)
	api.mux.HandleFunc("/api/linkedin/oauth/callback", api.linkedinOAuthCallback)
	api.mux.HandleFunc("/api/linkedin/profile", api.linkedinProfile)
	api.mux.HandleFunc("/api/linkedin/connection", api.linkedinProfile)
	api.mux.HandleFunc("/api/applications/prepare", api.prepareApplication)
	return api
}

func (api *API) Handler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		api.mux.ServeHTTP(writer, request)
	})
}

func (api *API) health(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		api.methodNotAllowed(writer, http.MethodGet)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

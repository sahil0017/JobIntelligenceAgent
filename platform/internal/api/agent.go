package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"careerpilot/intelligence/runs"
	"careerpilot/platform/internal/integrations/webjobs"
	"careerpilot/platform/internal/storage"
)

type AgentRunner interface {
	Start(runs.Params) (runs.Run, error)
	Get(string) (runs.Run, error)
	List() []runs.Run
	Cancel(string) error
}

func validateAgentConfig(config storage.AgentConfig) (storage.AgentConfig, error) {
	config.Model = strings.TrimSpace(config.Model)
	config.OllamaURL = strings.TrimSpace(config.OllamaURL)
	switch {
	case config.Model == "" || len(config.Model) > 100:
		return config, errors.New("model name is required and must be at most 100 characters")
	case len(config.OllamaURL) > 200:
		return config, errors.New("Ollama URL must be at most 200 characters")
	case config.MaxRounds < 1 || config.MaxRounds > 6:
		return config, errors.New("search rounds must be between 1 and 6")
	case config.TargetMatches < 1 || config.TargetMatches > 50:
		return config, errors.New("target matches must be between 1 and 50")
	case config.MaxPages < 5 || config.MaxPages > 60:
		return config, errors.New("page budget must be between 5 and 60")
	case config.MinScore < 1 || config.MinScore > 100:
		return config, errors.New("minimum match score must be between 1 and 100")
	}
	if err := runs.ValidateOllamaURL(config.OllamaURL); err != nil {
		return config, err
	}
	return config, nil
}

func (api *API) agentConfig(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		config, err := api.sources.Database().LoadAgentConfig()
		if err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(writer, http.StatusOK, config)
	case http.MethodPut:
		var config storage.AgentConfig
		if err := decodeRequest(request, &config); err != nil {
			writeError(writer, http.StatusBadRequest, err.Error())
			return
		}
		config, err := validateAgentConfig(config)
		if err != nil {
			writeError(writer, http.StatusBadRequest, err.Error())
			return
		}
		if err := api.sources.Database().SaveAgentConfig(config); err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(writer, http.StatusOK, config)
	default:
		api.methodNotAllowed(writer, http.MethodGet+", "+http.MethodPut)
	}
}

func (api *API) agentRunCollection(writer http.ResponseWriter, request *http.Request) {
	if api.agentRuns == nil {
		writeError(writer, http.StatusServiceUnavailable, "the job-search agent is unavailable")
		return
	}
	switch request.Method {
	case http.MethodGet:
		writeJSON(writer, http.StatusOK, map[string]any{"runs": api.agentRuns.List()})
	case http.MethodPost:
		api.startAgentRun(writer, request)
	default:
		api.methodNotAllowed(writer, http.MethodGet+", "+http.MethodPost)
	}
}

func (api *API) startAgentRun(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Country string `json:"country"`
		State   string `json:"state"`
		Area    string `json:"area"`
	}
	if err := decodeRequest(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	input.Country, input.State, input.Area = strings.TrimSpace(input.Country), strings.TrimSpace(input.State), strings.TrimSpace(input.Area)
	if input.Country == "" || input.Area == "" || len(input.Country) > 80 || len(input.State) > 80 || len(input.Area) > 100 {
		writeError(writer, http.StatusBadRequest, "country and area are required; location fields must be within their length limits")
		return
	}
	database := api.sources.Database()
	brave, err := database.LoadIntegration("brave_search")
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	if brave.ClientSecret == "" {
		writeError(writer, http.StatusPreconditionRequired, "configure a Brave Search API key in Settings before running the agent")
		return
	}
	config, err := database.LoadAgentConfig()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	saved, err := database.LoadProfile()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "could not load the saved profile for job matching")
		return
	}
	resume, err := database.LoadResume()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "could not load the saved resume for job matching")
		return
	}
	if runes := []rune(resume.Text); len(runes) > 20_000 {
		resume.Text = string(runes[:20_000])
	}
	run, err := api.agentRuns.Start(runs.Params{
		APIKey: brave.ClientSecret, Country: input.Country, State: input.State, Area: input.Area, Config: config,
		Profile: webjobs.Profile{Title: saved.Title, Skills: saved.Skills, ResumeText: resume.Text},
	})
	if err != nil {
		if errors.Is(err, runs.ErrBusy) {
			writeError(writer, http.StatusConflict, err.Error())
			return
		}
		log.Printf("start agent run: %v", err)
		writeError(writer, http.StatusInternalServerError, "could not start the agent run")
		return
	}
	writeJSON(writer, http.StatusAccepted, run)
}

func (api *API) agentRunItem(writer http.ResponseWriter, request *http.Request) {
	if api.agentRuns == nil {
		writeError(writer, http.StatusServiceUnavailable, "the job-search agent is unavailable")
		return
	}
	path := strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/agent/runs/"), "/")
	id, action, _ := strings.Cut(path, "/")
	if id == "" || strings.Contains(action, "/") {
		writeError(writer, http.StatusNotFound, "agent run not found")
		return
	}
	switch {
	case action == "" && request.Method == http.MethodGet:
		run, err := api.agentRuns.Get(id)
		if err != nil {
			writeError(writer, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(writer, http.StatusOK, run)
	case action == "cancel" && request.Method == http.MethodPost:
		if err := api.agentRuns.Cancel(id); err != nil {
			writeError(writer, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(writer, http.StatusAccepted, map[string]string{"status": fmt.Sprintf("cancelling run %s", id)})
	case action == "" || action == "cancel":
		api.methodNotAllowed(writer, http.MethodGet+", "+http.MethodPost)
	default:
		writeError(writer, http.StatusNotFound, "agent run not found")
	}
}

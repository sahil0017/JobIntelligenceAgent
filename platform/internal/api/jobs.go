package api

import (
	"careerpilot/contracts/domain"
	"careerpilot/platform/internal/integrations/webjobs"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

func (api *API) jobs(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		api.methodNotAllowed(writer, http.MethodGet)
		return
	}
	profile := domain.CandidateProfile{
		Skills:   request.URL.Query().Get("skills"),
		Title:    request.URL.Query().Get("title"),
		Location: request.URL.Query().Get("location"),
	}
	area := strings.TrimSpace(request.URL.Query().Get("area"))
	remoteRegion := strings.TrimSpace(request.URL.Query().Get("remoteRegion"))
	if len(profile.Skills) > 500 || len(profile.Title) > 500 || len(profile.Location) > 500 ||
		len(area) > 150 || len(remoteRegion) > 100 {
		writeError(writer, http.StatusBadRequest, "profile fields or selected area exceed their length limits")
		return
	}
	resume, err := api.sources.Database().LoadResume()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "could not load the saved resume for matching")
		return
	}
	resumeRunes := []rune(resume.Text)
	if len(resumeRunes) > 20_000 {
		resume.Text = string(resumeRunes[:20_000])
	}
	profile.ResumeText = resume.Text
	if area != "" {
		profile.Location = area
	}
	sources, err := api.sources.List()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	includeRemote := request.URL.Query().Get("includeRemote") == "true"
	input, err := json.Marshal(map[string]any{
		"sources": sources, "profile": profile, "area": area,
		"includeRemote": includeRemote, "remoteRegion": remoteRegion,
	})
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "could not prepare discovery request")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 40*time.Second)
	defer cancel()
	result, err := api.tools.Execute(ctx, "discover_jobs", input, false)
	if err != nil {
		log.Printf("job discovery failed: %v", err)
		writeError(writer, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (api *API) companySearch(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		api.methodNotAllowed(writer, http.MethodPost)
		return
	}
	if api.places == nil {
		writeError(writer, http.StatusServiceUnavailable, "Google Places search is unavailable")
		return
	}
	var input struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := decodeRequest(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	input.Query = strings.TrimSpace(input.Query)
	if input.Query == "" || len(input.Query) > 250 {
		writeError(writer, http.StatusBadRequest, "company search query must be between 1 and 250 characters")
		return
	}
	if input.Limit == 0 {
		input.Limit = 20
	}
	if input.Limit < 1 || input.Limit > 20 {
		writeError(writer, http.StatusBadRequest, "company search limit must be between 1 and 20")
		return
	}
	config, err := api.sources.Database().LoadIntegration("google_places")
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	if config.ClientSecret == "" {
		writeError(writer, http.StatusPreconditionRequired, "configure a Google Places API key in Settings before searching")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 25*time.Second)
	defer cancel()
	companies, err := api.places.Search(ctx, config.ClientSecret, input.Query, input.Limit)
	if err != nil {
		log.Printf("Google Places company search failed: %v", err)
		writeError(writer, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"companies": companies})
}

func (api *API) areaJobs(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		api.methodNotAllowed(writer, http.MethodPost)
		return
	}
	if api.webJobs == nil {
		writeError(writer, http.StatusServiceUnavailable, "public web job discovery is unavailable")
		return
	}
	var input struct {
		Country       string `json:"country"`
		State         string `json:"state"`
		Area          string `json:"area"`
		IncludeRemote bool   `json:"includeRemote"`
	}
	if err := decodeRequest(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	input.Country = strings.TrimSpace(input.Country)
	input.State = strings.TrimSpace(input.State)
	input.Area = strings.TrimSpace(input.Area)
	if input.Country == "" || input.Area == "" || len(input.Country) > 80 ||
		len(input.State) > 80 || len(input.Area) > 100 {
		writeError(writer, http.StatusBadRequest, "country and area are required; location fields must be within their length limits")
		return
	}
	config, err := api.sources.Database().LoadIntegration("brave_search")
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	if config.ClientSecret == "" {
		writeError(writer, http.StatusPreconditionRequired, "configure a Brave Search API key in Settings before searching")
		return
	}
	savedProfile, err := api.sources.Database().LoadProfile()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "could not load the saved profile for job matching")
		return
	}
	resume, err := api.sources.Database().LoadResume()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "could not load the saved resume for job matching")
		return
	}
	resumeRunes := []rune(resume.Text)
	if len(resumeRunes) > 20_000 {
		resume.Text = string(resumeRunes[:20_000])
	}
	profile := webjobs.Profile{
		Title: savedProfile.Title, Skills: savedProfile.Skills, ResumeText: resume.Text,
	}
	ctx, cancel := context.WithTimeout(request.Context(), 60*time.Second)
	defer cancel()
	result, err := api.webJobs.Search(ctx, config.ClientSecret, input.Country, input.State, input.Area, profile)
	if err != nil {
		log.Printf("public web job discovery failed: %v", err)
		writeError(writer, http.StatusBadGateway, err.Error())
		return
	}
	if !input.IncludeRemote {
		filtered := result.Jobs[:0]
		for _, job := range result.Jobs {
			if job.WorkMode != "Remote" {
				filtered = append(filtered, job)
			}
		}
		result.Jobs = filtered
	}
	writeJSON(writer, http.StatusOK, result)
}

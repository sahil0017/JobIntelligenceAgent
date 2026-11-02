package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"careerpilot/platform/internal/integrations/cortex"
)

const (
	maxAnalysisBody    = 256 << 10
	maxAnalysisJobs    = 100
	maxResumeCharacter = 60_000
)

type analysisJobsRequest struct {
	Jobs []cortex.Job `json:"jobs"`
}

// writeAIError maps AI-service failures to API errors without leaking internal detail.
func writeAIError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, cortex.ErrUnavailable):
		writeError(writer, http.StatusServiceUnavailable, cortex.ErrUnavailable.Error())
	case errors.Is(err, cortex.ErrNotFound):
		writeError(writer, http.StatusNotFound, cortex.ErrNotFound.Error())
	case errors.Is(err, cortex.ErrRejected):
		writeError(writer, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, cortex.ErrConflict):
		writeError(writer, http.StatusConflict, err.Error())
	default:
		log.Printf("AI service: %v", err)
		writeError(writer, http.StatusBadGateway, "the AI service returned an unexpected response")
	}
}

func (api *API) requireAI(writer http.ResponseWriter) bool {
	if api.ai == nil {
		writeError(writer, http.StatusServiceUnavailable, "the AI service is not configured")
		return false
	}
	return true
}

func writeRaw(writer http.ResponseWriter, status int, body json.RawMessage) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	if _, err := writer.Write(body); err != nil {
		log.Printf("write API response: %v", err)
	}
}

// aiStatus always answers 200 so the console can show "offline" without treating it as a failure.
func (api *API) aiStatus(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		api.methodNotAllowed(writer, http.MethodGet)
		return
	}
	if api.ai == nil {
		writeJSON(writer, http.StatusOK, map[string]any{"available": false, "message": "the AI service is not configured"})
		return
	}
	health, err := api.ai.Health(request.Context())
	if err != nil {
		writeJSON(writer, http.StatusOK, map[string]any{"available": false, "message": cortex.ErrUnavailable.Error()})
		return
	}
	var info map[string]any
	if err := json.Unmarshal(health, &info); err != nil {
		writeJSON(writer, http.StatusOK, map[string]any{"available": false, "message": "the AI service returned an unexpected response"})
		return
	}
	info["available"] = true
	writeJSON(writer, http.StatusOK, info)
}

func (api *API) aiPipeline(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		api.methodNotAllowed(writer, http.MethodGet)
		return
	}
	if !api.requireAI(writer) {
		return
	}
	pipeline, err := api.ai.Pipeline(request.Context())
	if err != nil {
		writeAIError(writer, err)
		return
	}
	writeRaw(writer, http.StatusOK, pipeline)
}

func (api *API) aiAnalysisCollection(writer http.ResponseWriter, request *http.Request) {
	if !api.requireAI(writer) {
		return
	}
	switch request.Method {
	case http.MethodGet:
		runs, err := api.ai.List(request.Context())
		if err != nil {
			writeAIError(writer, err)
			return
		}
		writeRaw(writer, http.StatusOK, runs)
	case http.MethodPost:
		api.startAnalysis(writer, request)
	default:
		api.methodNotAllowed(writer, http.MethodGet+", "+http.MethodPost)
	}
}

// startAnalysis attaches the saved profile and resume on the server, so the browser never has to
// send them and the resume text only travels to the local AI service.
func (api *API) startAnalysis(writer http.ResponseWriter, request *http.Request) {
	var input analysisJobsRequest
	if err := decodeRequestLimit(request, &input, maxAnalysisBody); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if len(input.Jobs) == 0 || len(input.Jobs) > maxAnalysisJobs {
		writeError(writer, http.StatusBadRequest, "send between 1 and 100 jobs to analyse")
		return
	}
	for index := range input.Jobs {
		job := &input.Jobs[index]
		job.ID, job.Role = strings.TrimSpace(job.ID), strings.TrimSpace(job.Role)
		if job.ID == "" || job.Role == "" {
			writeError(writer, http.StatusBadRequest, "every job needs an id and a role")
			return
		}
		if job.Tags == nil {
			job.Tags = []string{}
		}
	}
	profile, resumeText, ok := api.loadCandidate(writer)
	if !ok {
		return
	}
	run, err := api.ai.Start(request.Context(), cortex.AnalysisRequest{
		Profile:    profile,
		ResumeText: resumeText,
		Jobs:       input.Jobs,
	})
	if err != nil {
		writeAIError(writer, err)
		return
	}
	writeRaw(writer, http.StatusAccepted, run)
}

func (api *API) aiAnalysisItem(writer http.ResponseWriter, request *http.Request) {
	if !api.requireAI(writer) {
		return
	}
	path := strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/ai/analyses/"), "/")
	id, action, _ := strings.Cut(path, "/")
	if id == "" || strings.Contains(action, "/") {
		writeError(writer, http.StatusNotFound, cortex.ErrNotFound.Error())
		return
	}
	switch {
	case action == "" && request.Method == http.MethodGet:
		run, err := api.ai.Get(request.Context(), id)
		if err != nil {
			writeAIError(writer, err)
			return
		}
		writeRaw(writer, http.StatusOK, run)
	case action == "cancel" && request.Method == http.MethodPost:
		status, err := api.ai.Cancel(request.Context(), id)
		if err != nil {
			writeAIError(writer, err)
			return
		}
		writeRaw(writer, http.StatusAccepted, status)
	case action == "" || action == "cancel":
		api.methodNotAllowed(writer, http.MethodGet+", "+http.MethodPost)
	default:
		writeError(writer, http.StatusNotFound, cortex.ErrNotFound.Error())
	}
}

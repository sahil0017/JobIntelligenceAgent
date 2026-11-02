package api

import (
	"careerpilot/contracts/domain"
	"careerpilot/intelligence/agents"
	"careerpilot/platform/internal/storage"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

func (api *API) appliedJobs(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		trackedJobs, err := api.sources.Database().ListTrackedJobs()
		if err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"trackedJobs": trackedJobs})
	case http.MethodPost:
		var input struct {
			Job domain.Job `json:"job"`
		}
		if err := decodeRequest(request, &input); err != nil {
			writeError(writer, http.StatusBadRequest, err.Error())
			return
		}
		if input.Job.Status != "Applied" || strings.TrimSpace(input.Job.ID) == "" ||
			strings.TrimSpace(input.Job.Company) == "" || strings.TrimSpace(input.Job.Role) == "" {
			writeError(writer, http.StatusBadRequest, "only a complete job marked Applied can be saved for follow-up")
			return
		}
		if input.Job.SourceID != "" && input.Job.SourceID != domain.ProviderGreenhouse && input.Job.SourceID != domain.ProviderLever {
			writeError(writer, http.StatusBadRequest, "applied job has an unsupported monitoring provider")
			return
		}
		if input.Job.SourceID != "" && input.Job.SourceBoard == "" {
			writeError(writer, http.StatusBadRequest, "applied job is missing its career-board identifier")
			return
		}
		if err := api.sources.Database().SaveTrackedJob(input.Job, string(input.Job.SourceID), input.Job.SourceBoard); err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(writer, http.StatusCreated, map[string]bool{"saved": true})
	default:
		api.methodNotAllowed(writer, http.MethodGet+", "+http.MethodPost)
	}
}

func (api *API) appliedJobItem(writer http.ResponseWriter, request *http.Request) {
	path := strings.TrimPrefix(request.URL.Path, "/api/applied-jobs/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 || parts[0] == "" {
		writeError(writer, http.StatusBadRequest, "applied job ID is required")
		return
	}
	id, err := url.PathUnescape(parts[0])
	if err != nil || id == "" || strings.Contains(id, "/") {
		writeError(writer, http.StatusBadRequest, "invalid applied job ID")
		return
	}
	if len(parts) == 2 && parts[1] == "events" {
		if request.Method != http.MethodGet {
			api.methodNotAllowed(writer, http.MethodGet)
			return
		}
		events, err := api.sources.Database().ListTrackedJobEvents(id)
		if err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"events": events})
		return
	}
	if len(parts) == 2 && parts[1] == "keep" {
		if request.Method != http.MethodPost {
			api.methodNotAllowed(writer, http.MethodPost)
			return
		}
		if err := api.sources.Database().KeepTrackedJob(id); err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				writeError(writer, http.StatusNotFound, "job is not pending deletion")
				return
			}
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(writer, http.StatusOK, map[string]bool{"kept": true})
		return
	}
	if len(parts) != 1 {
		writeError(writer, http.StatusNotFound, "applied job route not found")
		return
	}
	switch request.Method {
	case http.MethodPut:
		var input struct {
			Job domain.Job `json:"job"`
		}
		if err := decodeRequest(request, &input); err != nil {
			writeError(writer, http.StatusBadRequest, err.Error())
			return
		}
		if input.Job.ID != id || strings.TrimSpace(input.Job.Company) == "" || strings.TrimSpace(input.Job.Role) == "" {
			writeError(writer, http.StatusBadRequest, "job ID, company, and role must be valid")
			return
		}
		if err := api.sources.Database().UpdateTrackedJob(input.Job); err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				writeError(writer, http.StatusNotFound, "applied job not found")
				return
			}
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(writer, http.StatusOK, map[string]bool{"updated": true})
	case http.MethodDelete:
		if err := api.sources.Database().ConfirmDeleteTrackedJob(id); err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				writeError(writer, http.StatusConflict, "job deletion requires a pending closure and explicit confirmation")
				return
			}
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(writer, http.StatusOK, map[string]bool{"deleted": true})
	default:
		api.methodNotAllowed(writer, http.MethodPut+", "+http.MethodDelete)
	}
}

func (api *API) prepareApplication(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		api.methodNotAllowed(writer, http.MethodPost)
		return
	}
	var input struct {
		Job domain.Job `json:"job"`
	}
	if err := decodeRequest(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	profile, err := api.sources.Database().LoadProfile()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	resume, err := api.sources.Database().LoadResume()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, agents.PrepareApplication(profile, resume, input.Job))
}

package api

import (
	"careerpilot/platform/internal/storage"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (api *API) portalCollection(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		portals, err := api.sources.Database().ListPortals()
		if err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"portals": portals})
	case http.MethodPost:
		var portal storage.PortalConfig
		if err := decodeRequest(request, &portal); err != nil {
			writeError(writer, http.StatusBadRequest, err.Error())
			return
		}
		portal.Name = strings.TrimSpace(portal.Name)
		portal.Provider = strings.ToLower(strings.TrimSpace(portal.Provider))
		portal.URL = strings.TrimSpace(portal.URL)
		portal.Notes = strings.TrimSpace(portal.Notes)
		if len(portal.Name) == 0 || len(portal.Name) > 100 || len(portal.Notes) > 1000 {
			writeError(writer, http.StatusBadRequest, "portal name and notes are invalid")
			return
		}
		if portal.Provider != "naukri" && portal.Provider != "other" {
			writeError(writer, http.StatusBadRequest, "portal provider must be naukri or other")
			return
		}
		parsed, err := url.ParseRequestURI(portal.URL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
			writeError(writer, http.StatusBadRequest, "portal URL must be an absolute HTTPS URL")
			return
		}
		portals, err := api.sources.Database().ListPortals()
		if err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		if len(portals) >= 20 {
			writeError(writer, http.StatusBadRequest, "a maximum of 20 portal configurations is supported")
			return
		}
		portal.ID, err = storage.NewID()
		if err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		portal.Status = "adapter_unavailable"
		portal.CreatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := api.sources.Database().AddPortal(portal); err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(writer, http.StatusCreated, map[string]storage.PortalConfig{"portal": portal})
	default:
		api.methodNotAllowed(writer, http.MethodGet+", "+http.MethodPost)
	}
}

func (api *API) portalItem(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodDelete {
		api.methodNotAllowed(writer, http.MethodDelete)
		return
	}
	id, err := url.PathUnescape(strings.TrimPrefix(request.URL.Path, "/api/portals/"))
	if err != nil || id == "" || strings.Contains(id, "/") {
		writeError(writer, http.StatusBadRequest, "invalid portal ID")
		return
	}
	if err := api.sources.Database().RemovePortal(id); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(writer, http.StatusNotFound, err.Error())
			return
		}
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	portals, err := api.sources.Database().ListPortals()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"portals": portals})
}

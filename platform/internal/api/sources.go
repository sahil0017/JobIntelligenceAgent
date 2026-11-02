package api

import (
	"careerpilot/contracts/domain"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

func (api *API) sourceCollection(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		sources, err := api.sources.List()
		if err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"sources": sources})
	case http.MethodPost:
		var source domain.CareerSource
		if err := decodeRequest(request, &source); err != nil {
			writeError(writer, http.StatusBadRequest, err.Error())
			return
		}
		source, err := api.sources.Add(source)
		if err != nil {
			switch {
			case errors.Is(err, errDuplicateSource):
				writeError(writer, http.StatusConflict, err.Error())
			case errors.Is(err, errSourceLimit):
				writeError(writer, http.StatusBadRequest, err.Error())
			default:
				writeError(writer, http.StatusBadRequest, err.Error())
			}
			return
		}
		writeJSON(writer, http.StatusCreated, map[string]domain.CareerSource{"source": source})
	default:
		api.methodNotAllowed(writer, http.MethodGet+", "+http.MethodPost)
	}
}

func (api *API) sourceItem(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodDelete {
		api.methodNotAllowed(writer, http.MethodDelete)
		return
	}
	id, err := url.PathUnescape(strings.TrimPrefix(request.URL.Path, "/api/sources/"))
	if err != nil || id == "" || strings.Contains(id, "/") {
		writeError(writer, http.StatusBadRequest, "invalid source ID")
		return
	}
	sources, err := api.sources.Remove(id)
	if err != nil {
		if errors.Is(err, errSourceNotFound) {
			writeError(writer, http.StatusNotFound, err.Error())
			return
		}
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"sources": sources})
}

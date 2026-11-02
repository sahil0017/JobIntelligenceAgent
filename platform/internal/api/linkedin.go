package api

import (
	"context"
	"log"
	"net/http"
	"time"
)

func (api *API) linkedinOAuthStart(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		api.methodNotAllowed(writer, http.MethodGet)
		return
	}
	if api.linkedin == nil || !api.linkedin.Configured() {
		writeError(writer, http.StatusServiceUnavailable, "LinkedIn OAuth is not configured")
		return
	}
	authorizationURL, err := api.linkedin.StartAuthorization()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"authorizationUrl": authorizationURL})
}

func (api *API) linkedinOAuthCallback(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		api.methodNotAllowed(writer, http.MethodGet)
		return
	}
	if api.linkedin == nil {
		writeError(writer, http.StatusServiceUnavailable, "LinkedIn OAuth is not configured")
		return
	}
	if request.URL.Query().Get("error") != "" {
		http.Redirect(writer, request, "http://localhost:5173/?oauth_error=linkedin", http.StatusSeeOther)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	profile, err := api.linkedin.CompleteAuthorization(ctx, request.URL.Query().Get("state"), request.URL.Query().Get("code"))
	if err != nil {
		log.Printf("complete LinkedIn OAuth: %v", err)
		writeError(writer, http.StatusBadRequest, "LinkedIn authorization could not be completed")
		return
	}
	current, err := api.sources.Database().LoadProfile()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	if profile.Name != "" {
		current.Name = profile.Name
	}
	if profile.Email != "" {
		current.Email = profile.Email
	}
	if err := api.sources.Database().SaveProfile(current); err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	http.Redirect(writer, request, "http://localhost:5173/?connected=linkedin", http.StatusSeeOther)
}

func (api *API) linkedinProfile(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		if request.Method == http.MethodDelete {
			if api.linkedin == nil {
				writeError(writer, http.StatusServiceUnavailable, "LinkedIn OAuth is not configured")
				return
			}
			api.linkedin.Disconnect()
			writeJSON(writer, http.StatusOK, map[string]bool{"connected": false})
			return
		}
		api.methodNotAllowed(writer, http.MethodGet+", "+http.MethodDelete)
		return
	}
	if api.linkedin == nil {
		writeError(writer, http.StatusServiceUnavailable, "LinkedIn OAuth is not configured")
		return
	}
	profile, connected := api.linkedin.Profile()
	writeJSON(writer, http.StatusOK, map[string]any{"connected": connected, "profile": profile})
}

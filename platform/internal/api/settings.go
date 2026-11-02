package api

import (
	"careerpilot/platform/internal/integrations/linkedin"
	"careerpilot/platform/internal/storage"
	"net/http"
	"strings"
)

func (api *API) settings(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		api.methodNotAllowed(writer, http.MethodGet)
		return
	}
	gmailConfig, err := api.sources.Database().LoadIntegration("gmail")
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	linkedinConfig, err := api.sources.Database().LoadIntegration("linkedin")
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	webSearchConfig, err := api.sources.Database().LoadIntegration("brave_search")
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	portals, err := api.sources.Database().ListPortals()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	profile, err := api.sources.Database().LoadProfile()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	linkedinProfile, linkedinConnected := linkedin.Profile{}, false
	if api.linkedin != nil {
		linkedinProfile, linkedinConnected = api.linkedin.Profile()
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"profile": profile,
		"integrations": map[string]any{
			"gmail": map[string]any{
				"configured":       api.gmail != nil && api.gmail.Configured(),
				"connected":        api.gmail != nil && api.gmail.Connected(),
				"clientId":         gmailConfig.ClientID,
				"redirectUrl":      gmailConfig.RedirectURL,
				"secretConfigured": gmailConfig.SecretConfigured,
				"scopes": []string{
					"https://www.googleapis.com/auth/gmail.readonly",
					"https://www.googleapis.com/auth/gmail.send",
				},
			},
			"linkedin": map[string]any{
				"configured":       api.linkedin != nil && api.linkedin.Configured(),
				"connected":        linkedinConnected,
				"clientId":         linkedinConfig.ClientID,
				"redirectUrl":      linkedinConfig.RedirectURL,
				"secretConfigured": linkedinConfig.SecretConfigured,
				"profile":          linkedinProfile,
				"scopes":           []string{"openid", "profile", "email"},
			},
		},
		"portals":   portals,
		"webSearch": map[string]bool{"configured": webSearchConfig.ClientSecret != ""},
	})
}

func (api *API) webSearchSettings(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		config, err := api.sources.Database().LoadIntegration("brave_search")
		if err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(writer, http.StatusOK, map[string]bool{"configured": config.ClientSecret != ""})
	case http.MethodPut:
		var input struct {
			APIKey string `json:"apiKey"`
		}
		if err := decodeRequest(request, &input); err != nil {
			writeError(writer, http.StatusBadRequest, err.Error())
			return
		}
		input.APIKey = strings.TrimSpace(input.APIKey)
		if input.APIKey == "" || len(input.APIKey) > 512 {
			writeError(writer, http.StatusBadRequest, "Brave Search API key must be between 1 and 512 characters")
			return
		}
		if err := api.sources.Database().SaveIntegration(storage.IntegrationConfig{
			Provider: "brave_search", ClientSecret: input.APIKey,
		}); err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(writer, http.StatusOK, map[string]bool{"configured": true})
	case http.MethodDelete:
		if err := api.sources.Database().DeleteIntegration("brave_search"); err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(writer, http.StatusOK, map[string]bool{"configured": false})
	default:
		api.methodNotAllowed(writer, http.MethodGet+", "+http.MethodPut+", "+http.MethodDelete)
	}
}

func (api *API) placesSettings(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		config, err := api.sources.Database().LoadIntegration("google_places")
		if err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(writer, http.StatusOK, map[string]bool{"configured": config.ClientSecret != ""})
	case http.MethodPut:
		var input struct {
			APIKey string `json:"apiKey"`
		}
		if err := decodeRequest(request, &input); err != nil {
			writeError(writer, http.StatusBadRequest, err.Error())
			return
		}
		input.APIKey = strings.TrimSpace(input.APIKey)
		if input.APIKey == "" || len(input.APIKey) > 512 {
			writeError(writer, http.StatusBadRequest, "Google Places API key must be between 1 and 512 characters")
			return
		}
		if err := api.sources.Database().SaveIntegration(storage.IntegrationConfig{
			Provider: "google_places", ClientSecret: input.APIKey,
		}); err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(writer, http.StatusOK, map[string]bool{"configured": true})
	case http.MethodDelete:
		if err := api.sources.Database().DeleteIntegration("google_places"); err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(writer, http.StatusOK, map[string]bool{"configured": false})
	default:
		api.methodNotAllowed(writer, http.MethodGet+", "+http.MethodPut+", "+http.MethodDelete)
	}
}

func (api *API) integrationSettings(writer http.ResponseWriter, request *http.Request) {
	provider := strings.TrimPrefix(request.URL.Path, "/api/settings/integrations/")
	if provider != "gmail" && provider != "linkedin" {
		writeError(writer, http.StatusNotFound, "integration configuration not found")
		return
	}
	switch request.Method {
	case http.MethodPut:
		var values integrationConfigRequest
		if err := decodeRequest(request, &values); err != nil {
			writeError(writer, http.StatusBadRequest, err.Error())
			return
		}
		submitted := storage.IntegrationConfig{
			Provider:     provider,
			ClientID:     strings.TrimSpace(values.ClientID),
			ClientSecret: values.ClientSecret,
			RedirectURL:  strings.TrimSpace(values.RedirectURL),
		}
		current, err := api.sources.Database().LoadIntegration(provider)
		if err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		if submitted.ClientSecret == "" {
			submitted.ClientSecret = current.ClientSecret
		}
		if submitted.ClientID == "" || submitted.ClientSecret == "" || submitted.RedirectURL == "" {
			writeError(writer, http.StatusBadRequest, "client ID, client secret, and redirect URL are required")
			return
		}
		var integration interface {
			Configure(storage.IntegrationConfig) error
		}
		if provider == "gmail" {
			integration = api.gmail
		} else {
			integration = api.linkedin
		}
		if integration == nil {
			writeError(writer, http.StatusServiceUnavailable, "integration is unavailable")
			return
		}
		if err := integration.Configure(submitted); err != nil {
			writeError(writer, http.StatusBadRequest, err.Error())
			return
		}
		if err := api.sources.Database().SaveIntegration(submitted); err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		saved, err := api.sources.Database().LoadIntegration(provider)
		if err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		saved.ClientSecret = ""
		writeJSON(writer, http.StatusOK, saved)
	case http.MethodDelete:
		if err := api.sources.Database().DeleteIntegration(provider); err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		if provider == "gmail" && api.gmail != nil {
			api.gmail.Disconnect()
			if err := api.gmail.Configure(storage.IntegrationConfig{Provider: provider}); err != nil {
				writeError(writer, http.StatusInternalServerError, err.Error())
				return
			}
		}
		if provider == "linkedin" && api.linkedin != nil {
			api.linkedin.Disconnect()
			if err := api.linkedin.Configure(storage.IntegrationConfig{Provider: provider}); err != nil {
				writeError(writer, http.StatusInternalServerError, err.Error())
				return
			}
		}
		writeJSON(writer, http.StatusOK, map[string]bool{"configured": false})
	default:
		api.methodNotAllowed(writer, http.MethodPut+", "+http.MethodDelete)
	}
}

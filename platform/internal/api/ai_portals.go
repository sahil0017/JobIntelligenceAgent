package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"careerpilot/platform/internal/integrations/cortex"
)

const maxPortalBody = 8 << 10

type portalSearchInput struct {
	URL      string `json:"url"`
	Keywords string `json:"keywords"`
}

type portalSignInInput struct {
	URL string `json:"url"`
}

func validPageURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || len(raw) > maxPageURLLength {
		return "", false
	}
	return raw, true
}

func (api *API) aiPortalSearch(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		api.methodNotAllowed(writer, http.MethodPost)
		return
	}
	if !api.requireAI(writer) {
		return
	}
	var input portalSearchInput
	if err := decodeRequestLimit(request, &input, maxPortalBody); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	address, ok := validPageURL(input.URL)
	if !ok {
		writeError(writer, http.StatusBadRequest, "the careers page address must be a valid http(s) URL")
		return
	}
	keywords := strings.TrimSpace(input.Keywords)
	if len([]rune(keywords)) > 300 {
		writeError(writer, http.StatusBadRequest, "keywords are too long")
		return
	}
	profile, resumeText, ok := api.loadCandidate(writer)
	if !ok {
		return
	}
	result, err := api.ai.PortalSearch(request.Context(), cortex.PortalSearchRequest{URL: address, Keywords: keywords, Profile: profile, ResumeText: resumeText})
	if err != nil {
		writeAIError(writer, err)
		return
	}
	writeRaw(writer, http.StatusOK, result)
}

// aiPortalSession manages the local browser sign-in used for portals that answer 401/403. The
// user signs in inside a browser window; no Google credentials pass through this API.
func (api *API) aiPortalSession(writer http.ResponseWriter, request *http.Request) {
	if !api.requireAI(writer) {
		return
	}
	switch request.Method {
	case http.MethodGet:
		api.writePortalSession(writer, request, func() (rawJSON, error) { return api.ai.PortalSession(request.Context()) })
	case http.MethodPost:
		var input portalSignInInput
		if err := decodeRequestLimit(request, &input, maxPortalBody); err != nil {
			writeError(writer, http.StatusBadRequest, err.Error())
			return
		}
		address, ok := validPageURL(input.URL)
		if !ok {
			writeError(writer, http.StatusBadRequest, "the sign-in address must be a valid http(s) URL")
			return
		}
		api.writePortalSession(writer, request, func() (rawJSON, error) { return api.ai.PortalSignIn(request.Context(), address) })
	case http.MethodDelete:
		api.writePortalSession(writer, request, func() (rawJSON, error) { return api.ai.PortalSignOut(request.Context()) })
	default:
		api.methodNotAllowed(writer, "GET, POST, DELETE")
	}
}

type rawJSON = json.RawMessage

func (api *API) writePortalSession(writer http.ResponseWriter, _ *http.Request, call func() (rawJSON, error)) {
	result, err := call()
	if err != nil {
		writeAIError(writer, err)
		return
	}
	writeRaw(writer, http.StatusOK, result)
}

type quickApplyInput struct {
	URL string `json:"url"`
}

// aiQuickApply opens a job's application form right away and pre-fills it from the saved profile.
// The click that calls this is the person's approval; the form is never submitted for them.
func (api *API) aiQuickApply(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		api.methodNotAllowed(writer, http.MethodPost)
		return
	}
	if !api.requireAI(writer) {
		return
	}
	var input quickApplyInput
	if err := decodeRequestLimit(request, &input, maxPortalBody); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	address, ok := validPageURL(input.URL)
	if !ok {
		writeError(writer, http.StatusBadRequest, "the job needs a valid http(s) page address")
		return
	}
	profile, err := api.sources.Database().LoadProfile()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "could not load the saved profile")
		return
	}
	result, err := api.ai.QuickApply(request.Context(), cortex.QuickApply{URL: address, Applicant: cortex.Applicant{Name: profile.Name, Email: profile.Email, Location: profile.Location}})
	if err != nil {
		writeAIError(writer, err)
		return
	}
	writeRaw(writer, http.StatusOK, result)
}

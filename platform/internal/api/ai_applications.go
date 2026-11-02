package api

import (
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"careerpilot/platform/internal/integrations/cortex"
)

const (
	maxApplicationBody = 64 << 10
	maxDecisionBody    = 64 << 10
	maxPageURLLength   = 2000
	maxDraftTextRunes  = 4000
)

type applicationJobRequest struct {
	Job         cortex.Job `json:"job"`
	URL         string     `json:"url"`
	DraftAnyway bool       `json:"draftAnyway"`
}

// loadCandidate reads the saved profile and résumé on the server, so the browser never has to send
// them and the résumé text only travels to the local AI service.
func (api *API) loadCandidate(writer http.ResponseWriter) (cortex.Profile, string, bool) {
	database := api.sources.Database()
	profile, err := database.LoadProfile()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "could not load the saved profile")
		return cortex.Profile{}, "", false
	}
	resume, err := database.LoadResume()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "could not load the saved resume")
		return cortex.Profile{}, "", false
	}
	if runes := []rune(resume.Text); len(runes) > maxResumeCharacter {
		resume.Text = string(runes[:maxResumeCharacter])
	}
	return cortex.Profile{Title: profile.Title, Skills: profile.Skills, Roles: profile.Roles, Experience: profile.Experience}, resume.Text, true
}

func (api *API) aiApplicationPipeline(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		api.methodNotAllowed(writer, http.MethodGet)
		return
	}
	if !api.requireAI(writer) {
		return
	}
	pipeline, err := api.ai.ApplicationPipeline(request.Context())
	if err != nil {
		writeAIError(writer, err)
		return
	}
	writeRaw(writer, http.StatusOK, pipeline)
}

func (api *API) aiApplicationCollection(writer http.ResponseWriter, request *http.Request) {
	if !api.requireAI(writer) {
		return
	}
	switch request.Method {
	case http.MethodGet:
		runs, err := api.ai.Applications(request.Context())
		if err != nil {
			writeAIError(writer, err)
			return
		}
		writeRaw(writer, http.StatusOK, runs)
	case http.MethodPost:
		api.startApplication(writer, request)
	default:
		api.methodNotAllowed(writer, http.MethodGet+", "+http.MethodPost)
	}
}

func (api *API) startApplication(writer http.ResponseWriter, request *http.Request) {
	var input applicationJobRequest
	if err := decodeRequestLimit(request, &input, maxApplicationBody); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	input.Job.ID, input.Job.Role = strings.TrimSpace(input.Job.ID), strings.TrimSpace(input.Job.Role)
	if input.Job.ID == "" || input.Job.Role == "" {
		writeError(writer, http.StatusBadRequest, "the job needs an id and a role")
		return
	}
	if input.Job.Tags == nil {
		input.Job.Tags = []string{}
	}
	input.URL = strings.TrimSpace(input.URL)
	if input.URL != "" {
		parsed, err := url.Parse(input.URL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || len(input.URL) > maxPageURLLength {
			writeError(writer, http.StatusBadRequest, "the job page address must be a valid http(s) URL")
			return
		}
	}
	profile, resumeText, ok := api.loadCandidate(writer)
	if !ok {
		return
	}
	run, err := api.ai.StartApplication(request.Context(), cortex.ApplicationRequest{
		Profile: profile, ResumeText: resumeText, Job: input.Job, URL: input.URL, DraftAnyway: input.DraftAnyway,
	})
	if err != nil {
		writeAIError(writer, err)
		return
	}
	writeRaw(writer, http.StatusAccepted, run)
}

func (api *API) aiApplicationItem(writer http.ResponseWriter, request *http.Request) {
	if !api.requireAI(writer) {
		return
	}
	path := strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/ai/applications/"), "/")
	id, action, _ := strings.Cut(path, "/")
	if id == "" || strings.Contains(action, "/") {
		writeError(writer, http.StatusNotFound, cortex.ErrNotFound.Error())
		return
	}
	switch {
	case action == "" && request.Method == http.MethodGet:
		run, err := api.ai.Application(request.Context(), id)
		if err != nil {
			writeAIError(writer, err)
			return
		}
		writeRaw(writer, http.StatusOK, run)
	case action == "cancel" && request.Method == http.MethodPost:
		status, err := api.ai.CancelApplication(request.Context(), id)
		if err != nil {
			writeAIError(writer, err)
			return
		}
		writeRaw(writer, http.StatusAccepted, status)
	case action == "decision" && request.Method == http.MethodPost:
		api.decideApplication(writer, request, id)
	case action == "apply" && request.Method == http.MethodPost:
		api.applyApplication(writer, request, id)
	case action == "" || action == "cancel" || action == "decision" || action == "apply":
		api.methodNotAllowed(writer, http.MethodGet+", "+http.MethodPost)
	default:
		writeError(writer, http.StatusNotFound, cortex.ErrNotFound.Error())
	}
}

// decideApplication is the only way a draft leaves "awaiting approval", and it needs a person's click.
func (api *API) decideApplication(writer http.ResponseWriter, request *http.Request, id string) {
	var decision cortex.Decision
	if err := decodeRequestLimit(request, &decision, maxDecisionBody); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if decision.TailoredBullets != nil && len(*decision.TailoredBullets) > 12 {
		writeError(writer, http.StatusBadRequest, "keep the tailored bullets to 12 or fewer")
		return
	}
	for _, text := range decisionTexts(decision) {
		if utf8.RuneCountInString(text) > maxDraftTextRunes {
			writeError(writer, http.StatusBadRequest, "a draft text is too long")
			return
		}
	}
	run, err := api.ai.DecideApplication(request.Context(), id, decision)
	if err != nil {
		writeAIError(writer, err)
		return
	}
	writeRaw(writer, http.StatusAccepted, run)
}

func decisionTexts(decision cortex.Decision) []string {
	var texts []string
	if decision.TailoredBullets != nil {
		texts = append(texts, *decision.TailoredBullets...)
	}
	if decision.CoverNote != nil {
		texts = append(texts, *decision.CoverNote)
	}
	if decision.OutreachMessage != nil {
		texts = append(texts, *decision.OutreachMessage)
	}
	return texts
}

// applyApplication asks the Apply agent to open the approved job's form in a visible browser and
// pre-fill it. The person reviews the form and presses Submit themselves.
func (api *API) applyApplication(writer http.ResponseWriter, request *http.Request, id string) {
	profile, err := api.sources.Database().LoadProfile()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "could not load the saved profile")
		return
	}
	run, err := api.ai.ApplyApplication(request.Context(), id, cortex.Applicant{Name: profile.Name, Email: profile.Email, Location: profile.Location})
	if err != nil {
		writeAIError(writer, err)
		return
	}
	writeRaw(writer, http.StatusAccepted, run)
}

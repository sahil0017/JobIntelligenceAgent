package api

import (
	"careerpilot/platform/internal/storage"
	"io"
	"net/http"
	"path/filepath"
)

func (api *API) profile(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		profile, err := api.sources.Database().LoadProfile()
		if err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(writer, http.StatusOK, profile)
	case http.MethodPut:
		var profile storage.Profile
		if err := decodeRequest(request, &profile); err != nil {
			writeError(writer, http.StatusBadRequest, err.Error())
			return
		}
		if len(profile.Name) > 200 || len(profile.Title) > 500 || len(profile.Email) > 320 ||
			len(profile.Location) > 500 || len(profile.Experience) > 200 || len(profile.Skills) > 5000 || len(profile.Roles) > 2000 {
			writeError(writer, http.StatusBadRequest, "profile fields exceed their allowed lengths")
			return
		}
		if err := api.sources.Database().SaveProfile(profile); err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(writer, http.StatusOK, profile)
	default:
		api.methodNotAllowed(writer, http.MethodGet+", "+http.MethodPut)
	}
}

func (api *API) resume(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		resume, err := api.sources.Database().LoadResume()
		if err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		resume.Text = ""
		writeJSON(writer, http.StatusOK, resume)
	case http.MethodPost:
		request.Body = http.MaxBytesReader(writer, request.Body, maxResumeSize+1<<20)
		if err := request.ParseMultipartForm(maxResumeSize); err != nil {
			writeError(writer, http.StatusBadRequest, "resume upload must be a multipart form with a file no larger than 10 MB")
			return
		}
		defer request.MultipartForm.RemoveAll()
		file, header, err := request.FormFile("resume")
		if err != nil {
			writeError(writer, http.StatusBadRequest, "resume file is required")
			return
		}
		defer file.Close()
		content, err := io.ReadAll(io.LimitReader(file, maxResumeSize+1))
		if err != nil {
			writeError(writer, http.StatusBadRequest, "could not read resume file")
			return
		}
		filename := filepath.Base(header.Filename)
		text, err := parseResume(filename, content)
		if err != nil {
			writeError(writer, http.StatusBadRequest, err.Error())
			return
		}
		resume := storage.Resume{Filename: filename, Text: text}
		if err := api.sources.Database().SaveResume(resume); err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		resume.Text = ""
		writeJSON(writer, http.StatusOK, resume)
	case http.MethodDelete:
		if err := api.sources.Database().DeleteResume(); err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(writer, http.StatusOK, map[string]bool{"deleted": true})
	default:
		api.methodNotAllowed(writer, http.MethodGet+", "+http.MethodPost+", "+http.MethodDelete)
	}
}

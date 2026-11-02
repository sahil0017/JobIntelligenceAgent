package api

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"
)

func (api *API) gmailStatus(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		api.methodNotAllowed(writer, http.MethodGet)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]bool{
		"configured": api.gmail != nil && api.gmail.Configured(),
		"connected":  api.gmail != nil && api.gmail.Connected(),
	})
}

func (api *API) gmailOAuthStart(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		api.methodNotAllowed(writer, http.MethodGet)
		return
	}
	if api.gmail == nil {
		writeError(writer, http.StatusServiceUnavailable, "Gmail OAuth is not configured")
		return
	}
	authorizationURL, err := api.gmail.StartAuthorization()
	if err != nil {
		log.Printf("start Gmail OAuth: %v", err)
		writeError(writer, http.StatusInternalServerError, "could not start Gmail authorization")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"authorizationUrl": authorizationURL})
}

func (api *API) gmailOAuthCallback(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		api.methodNotAllowed(writer, http.MethodGet)
		return
	}
	if api.gmail == nil {
		writeError(writer, http.StatusServiceUnavailable, "Gmail OAuth is not configured")
		return
	}
	if request.URL.Query().Get("error") != "" {
		http.Redirect(writer, request, "http://localhost:5173/?oauth_error=gmail", http.StatusSeeOther)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	if err := api.gmail.CompleteAuthorization(ctx, request.URL.Query().Get("state"), request.URL.Query().Get("code")); err != nil {
		log.Printf("complete Gmail OAuth: %v", err)
		writeError(writer, http.StatusBadRequest, "Gmail authorization could not be completed")
		return
	}
	http.Redirect(writer, request, "http://localhost:5173/?connected=gmail", http.StatusSeeOther)
}

func (api *API) gmailThreads(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		api.methodNotAllowed(writer, http.MethodGet)
		return
	}
	if api.gmail == nil {
		writeError(writer, http.StatusServiceUnavailable, "Gmail OAuth is not configured")
		return
	}
	if !api.gmail.Connected() {
		writeError(writer, http.StatusUnauthorized, "Gmail is not connected")
		return
	}
	query := request.URL.Query().Get("q")
	if query == "" {
		query = "newer_than:90d (job OR hiring OR recruiter OR career OR application)"
	}
	if len(query) > 500 {
		writeError(writer, http.StatusBadRequest, "Gmail search query must not exceed 500 characters")
		return
	}
	limit := 10
	if value := request.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 20 {
			writeError(writer, http.StatusBadRequest, "Gmail result limit must be between 1 and 20")
			return
		}
		limit = parsed
	}
	ctx, cancel := context.WithTimeout(request.Context(), 25*time.Second)
	defer cancel()
	threads, err := api.gmail.SearchThreads(ctx, query, limit)
	if err != nil {
		log.Printf("search Gmail threads: %v", err)
		writeError(writer, http.StatusBadGateway, "Gmail threads could not be retrieved")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"threads": threads})
}

func (api *API) gmailSend(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		api.methodNotAllowed(writer, http.MethodPost)
		return
	}
	if api.gmail == nil || !api.gmail.Connected() {
		writeError(writer, http.StatusUnauthorized, "Gmail is not connected")
		return
	}
	var input struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
		Confirm bool   `json:"confirm"`
	}
	if err := decodeRequest(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if !input.Confirm {
		writeError(writer, http.StatusBadRequest, "explicit confirmation is required before sending")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 25*time.Second)
	defer cancel()
	messageID, err := api.gmail.SendMessage(ctx, input.To, input.Subject, input.Body)
	if err != nil {
		log.Printf("send Gmail message: %v", err)
		writeError(writer, http.StatusBadGateway, "Gmail could not send the message. Reconnect Gmail and grant the requested send permission if prompted.")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"messageId": messageID, "status": "sent"})
}

func (api *API) gmailConnection(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodDelete {
		api.methodNotAllowed(writer, http.MethodDelete)
		return
	}
	if api.gmail == nil {
		writeError(writer, http.StatusServiceUnavailable, "Gmail OAuth is not configured")
		return
	}
	api.gmail.Disconnect()
	writeJSON(writer, http.StatusOK, map[string]bool{"connected": false})
}

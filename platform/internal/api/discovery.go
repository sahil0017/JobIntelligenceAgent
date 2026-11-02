package api

import (
	"careerpilot/intelligence/agents"
	"context"
	"log"
	"net/http"
	"time"
)

func (api *API) agentDiscover(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		api.methodNotAllowed(writer, http.MethodPost)
		return
	}
	if api.discoveryAgent == nil {
		writeError(writer, http.StatusServiceUnavailable, "job discovery agent is not configured")
		return
	}
	var input agents.DiscoveryRequest
	if err := decodeRequest(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 100*time.Second)
	defer cancel()
	result, err := api.discoveryAgent.Run(ctx, input)
	if err != nil {
		log.Printf("job discovery agent failed: %v", err)
		writeError(writer, http.StatusBadGateway, "job discovery agent could not complete the request")
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

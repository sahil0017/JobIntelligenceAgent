package lever

import (
	"net/http"

	"careerpilot/platform/internal/integrations/jobs"
)

type Client struct {
	httpClient *http.Client
}

func NewClient(httpClient *http.Client) *Client {
	return &Client{httpClient: jobs.NewHTTPClient(httpClient)}
}

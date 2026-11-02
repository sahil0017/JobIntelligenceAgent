package googleplaces

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"careerpilot/contracts/domain"
	"careerpilot/platform/internal/integrations/jobs"
)

const (
	searchEndpoint = "https://places.googleapis.com/v1/places:searchText"
	maxResponse    = 2 << 20
	fieldMask      = "places.id,places.displayName,places.formattedAddress,places.websiteUri,places.primaryTypeDisplayName,places.googleMapsUri"
)

type Client struct {
	httpClient *http.Client
	endpoint   string
}

func NewClient(httpClient *http.Client) *Client {
	safeClient := *jobs.NewHTTPClient(httpClient)
	safeClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Client{httpClient: &safeClient, endpoint: searchEndpoint}
}

type searchRequest struct {
	TextQuery string `json:"textQuery"`
	PageSize  int    `json:"pageSize"`
}

type searchResponse struct {
	Places []struct {
		ID                     string `json:"id"`
		DisplayName            text   `json:"displayName"`
		FormattedAddress       string `json:"formattedAddress"`
		WebsiteURI             string `json:"websiteUri"`
		PrimaryTypeDisplayName text   `json:"primaryTypeDisplayName"`
		GoogleMapsURI          string `json:"googleMapsUri"`
	} `json:"places"`
}

type text struct {
	Text string `json:"text"`
}

func (client *Client) Search(ctx context.Context, apiKey, query string, limit int) ([]domain.Company, error) {
	apiKey = strings.TrimSpace(apiKey)
	query = strings.TrimSpace(query)
	if apiKey == "" {
		return nil, fmt.Errorf("Google Places API key is required")
	}
	if query == "" || len(query) > 250 {
		return nil, fmt.Errorf("company search query must be between 1 and 250 characters")
	}
	if limit < 1 || limit > 20 {
		return nil, fmt.Errorf("company search result limit must be between 1 and 20")
	}
	payload, err := json.Marshal(searchRequest{TextQuery: query, PageSize: limit})
	if err != nil {
		return nil, fmt.Errorf("encode Google Places search: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create Google Places request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Goog-Api-Key", apiKey)
	request.Header.Set("X-Goog-FieldMask", fieldMask)
	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request Google Places search: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("Google Places search returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if err != nil {
		return nil, fmt.Errorf("read Google Places response: %w", err)
	}
	if len(body) > maxResponse {
		return nil, fmt.Errorf("Google Places response exceeds the %d MiB size limit", maxResponse>>20)
	}
	var result searchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode Google Places response: %w", err)
	}
	companies := make([]domain.Company, 0, len(result.Places))
	for _, place := range result.Places {
		if place.ID == "" || place.DisplayName.Text == "" {
			continue
		}
		companies = append(companies, domain.Company{
			ID:       place.ID,
			Name:     place.DisplayName.Text,
			Address:  place.FormattedAddress,
			Website:  place.WebsiteURI,
			Category: place.PrimaryTypeDisplayName.Text,
			MapsURL:  place.GoogleMapsURI,
		})
		if len(companies) == limit {
			break
		}
	}
	return companies, nil
}

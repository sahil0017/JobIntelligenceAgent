package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"careerpilot/contracts/domain"
)

type catalogEntry struct {
	provider domain.Provider
	company  string
	board    string
}

// engineeringCatalog lists companies that publish engineering roles on public Greenhouse or Lever
// boards. Each board is probed before it is connected, so a company that moves ATS is skipped.
var engineeringCatalog = []catalogEntry{
	{domain.ProviderGreenhouse, "Stripe", "stripe"}, {domain.ProviderGreenhouse, "Airbnb", "airbnb"},
	{domain.ProviderGreenhouse, "Databricks", "databricks"}, {domain.ProviderGreenhouse, "Cloudflare", "cloudflare"},
	{domain.ProviderGreenhouse, "Datadog", "datadog"}, {domain.ProviderGreenhouse, "Figma", "figma"},
	{domain.ProviderGreenhouse, "Discord", "discord"}, {domain.ProviderGreenhouse, "Coinbase", "coinbase"},
	{domain.ProviderGreenhouse, "Robinhood", "robinhood"}, {domain.ProviderGreenhouse, "Reddit", "reddit"},
	{domain.ProviderGreenhouse, "Pinterest", "pinterest"}, {domain.ProviderGreenhouse, "Instacart", "instacart"},
	{domain.ProviderGreenhouse, "Twilio", "twilio"}, {domain.ProviderGreenhouse, "MongoDB", "mongodb"},
	{domain.ProviderGreenhouse, "Elastic", "elastic"}, {domain.ProviderGreenhouse, "GitLab", "gitlab"},
	{domain.ProviderGreenhouse, "Dropbox", "dropbox"}, {domain.ProviderGreenhouse, "Asana", "asana"},
	{domain.ProviderGreenhouse, "Okta", "okta"}, {domain.ProviderGreenhouse, "Brex", "brex"},
	{domain.ProviderGreenhouse, "Ramp", "ramp"}, {domain.ProviderGreenhouse, "Vercel", "vercel"},
	{domain.ProviderLever, "Spotify", "spotify"}, {domain.ProviderLever, "Kraken", "kraken"},
	{domain.ProviderLever, "Zoox", "zoox"},
}

func probeURL(entry catalogEntry) string {
	board := url.PathEscape(entry.board)
	if entry.provider == domain.ProviderLever {
		return "https://api.lever.co/v0/postings/" + board + "?mode=json&limit=1"
	}
	return "https://boards-api.greenhouse.io/v1/boards/" + board + "/jobs"
}

type catalogResult struct {
	Added   []string `json:"added"`
	Skipped []string `json:"skipped"`
	Failed  []string `json:"unavailable"`
	Total   int      `json:"total"`
}

func (api *API) connectEngineeringCatalog(ctx context.Context) (catalogResult, error) {
	existing, err := api.sources.List()
	if err != nil {
		return catalogResult{}, err
	}
	have := map[string]bool{}
	for _, source := range existing {
		have[string(source.Provider)+"/"+source.BoardID] = true
	}
	client := &http.Client{Timeout: 10 * time.Second}
	live := make([]bool, len(engineeringCatalog))
	var group sync.WaitGroup
	slots := make(chan struct{}, 8)
	for index, entry := range engineeringCatalog {
		if have[string(entry.provider)+"/"+entry.board] {
			continue
		}
		group.Add(1)
		go func(index int, entry catalogEntry) {
			defer group.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL(entry), nil)
			if err != nil {
				return
			}
			response, err := client.Do(request)
			if err != nil {
				return
			}
			response.Body.Close()
			live[index] = response.StatusCode == http.StatusOK
		}(index, entry)
	}
	group.Wait()
	result := catalogResult{Added: []string{}, Skipped: []string{}, Failed: []string{}, Total: len(engineeringCatalog)}
	for index, entry := range engineeringCatalog {
		switch {
		case have[string(entry.provider)+"/"+entry.board]:
			result.Skipped = append(result.Skipped, entry.company)
		case !live[index]:
			result.Failed = append(result.Failed, entry.company)
		default:
			if _, err := api.sources.Add(domain.CareerSource{Provider: entry.provider, Company: entry.company, BoardID: entry.board}); err != nil {
				result.Failed = append(result.Failed, fmt.Sprintf("%s (%v)", entry.company, err))
				continue
			}
			result.Added = append(result.Added, entry.company)
		}
	}
	return result, nil
}

func (api *API) sourceCatalog(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		api.methodNotAllowed(writer, http.MethodPost)
		return
	}
	result, err := api.connectEngineeringCatalog(request.Context())
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

package webjobs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"careerpilot/platform/internal/integrations/jobs"
)

var (
	greenhousePath      = regexp.MustCompile(`^/([^/]+)/jobs/(\d+)`)
	leverPath           = regexp.MustCompile(`^/([^/]+)/([0-9a-fA-F-]{36})`)
	smartRecruitersPath = regexp.MustCompile(`^/([^/]+)/(\d+)`)
)

// atsPosting is a single posting read from an ATS provider's public JSON API.
type atsPosting struct {
	Company, Title, Location, Description, URL, Posted string
	Remote                                             bool
}

// atsEndpoint maps a public posting URL to the provider's official JSON API.
// ok is false when the URL is not a recognized single-posting URL.
func atsEndpoint(pageURL *url.URL) (endpoint, provider string, ok bool) {
	host := strings.ToLower(pageURL.Hostname())
	switch {
	case host == "boards.greenhouse.io" || host == "job-boards.greenhouse.io":
		if m := greenhousePath.FindStringSubmatch(pageURL.Path); m != nil {
			return fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs/%s", url.PathEscape(m[1]), m[2]), "greenhouse", true
		}
	case host == "jobs.lever.co":
		if m := leverPath.FindStringSubmatch(pageURL.Path); m != nil {
			return fmt.Sprintf("https://api.lever.co/v0/postings/%s/%s", url.PathEscape(m[1]), m[2]), "lever", true
		}
	case host == "jobs.smartrecruiters.com":
		if m := smartRecruitersPath.FindStringSubmatch(pageURL.Path); m != nil {
			return fmt.Sprintf("https://api.smartrecruiters.com/v1/companies/%s/postings/%s", url.PathEscape(m[1]), m[2]), "smartrecruiters", true
		}
	}
	return "", "", false
}

func (client *Client) fetchATSPosting(ctx context.Context, endpoint, provider string) (atsPosting, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return atsPosting{}, fmt.Errorf("create %s request: %w", provider, err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", crawlerUserAgent+"/0.1")
	response, err := client.httpClient.Do(request)
	if err != nil {
		return atsPosting{}, fmt.Errorf("fetch %s posting: %w", provider, err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return atsPosting{}, fmt.Errorf("%s posting is no longer available", provider)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return atsPosting{}, fmt.Errorf("%s posting API returned HTTP %d", provider, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxPageBody+1))
	if err != nil {
		return atsPosting{}, fmt.Errorf("read %s posting: %w", provider, err)
	}
	if len(body) > maxPageBody {
		return atsPosting{}, fmt.Errorf("%s posting exceeds the %d MiB size limit", provider, maxPageBody>>20)
	}
	switch provider {
	case "greenhouse":
		return parseGreenhouse(body)
	case "lever":
		return parseLever(body)
	default:
		return parseSmartRecruiters(body)
	}
}

func parseGreenhouse(body []byte) (atsPosting, error) {
	var data struct {
		Title       string `json:"title"`
		CompanyName string `json:"company_name"`
		AbsoluteURL string `json:"absolute_url"`
		Content     string `json:"content"`
		UpdatedAt   string `json:"updated_at"`
		Location    struct {
			Name string `json:"name"`
		} `json:"location"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return atsPosting{}, fmt.Errorf("decode greenhouse posting: %w", err)
	}
	return atsPosting{
		Company: data.CompanyName, Title: data.Title, Location: data.Location.Name,
		Description: data.Content, URL: data.AbsoluteURL, Posted: data.UpdatedAt,
		Remote: strings.Contains(strings.ToLower(data.Location.Name), "remote"),
	}, nil
}

func parseLever(body []byte) (atsPosting, error) {
	var data struct {
		Text         string `json:"text"`
		HostedURL    string `json:"hostedUrl"`
		Description  string `json:"description"`
		CreatedAt    int64  `json:"createdAt"`
		WorkplaceTyp string `json:"workplaceType"`
		Categories   struct {
			Location string `json:"location"`
		} `json:"categories"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return atsPosting{}, fmt.Errorf("decode lever posting: %w", err)
	}
	posted := ""
	if data.CreatedAt > 0 {
		posted = time.UnixMilli(data.CreatedAt).UTC().Format(time.RFC3339)
	}
	// Lever exposes no company name; the caller derives it from the URL.
	return atsPosting{
		Title: data.Text, Location: data.Categories.Location, Description: data.Description,
		URL: data.HostedURL, Posted: posted, Remote: data.WorkplaceTyp == "remote",
	}, nil
}

func parseSmartRecruiters(body []byte) (atsPosting, error) {
	var data struct {
		Name       string                `json:"name"`
		Active     bool                  `json:"active"`
		PostingURL string                `json:"postingUrl"`
		ReleasedAt string                `json:"releasedDate"`
		Company    struct{ Name string } `json:"company"`
		Location   struct {
			City, Region, Country string
			Remote                bool
		} `json:"location"`
		JobAd struct {
			Sections map[string]struct {
				Text string `json:"text"`
			} `json:"sections"`
		} `json:"jobAd"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return atsPosting{}, fmt.Errorf("decode smartrecruiters posting: %w", err)
	}
	if !data.Active {
		return atsPosting{}, fmt.Errorf("smartrecruiters posting is no longer active")
	}
	var description strings.Builder
	for _, section := range data.JobAd.Sections {
		description.WriteString(section.Text)
		description.WriteString(" ")
	}
	country := strings.ToUpper(data.Location.Country)
	if country == "IN" {
		country = "India"
	}
	return atsPosting{
		Company: data.Company.Name, Title: data.Name, Description: description.String(),
		Location: strings.Join(nonEmpty(data.Location.City, data.Location.Region, country), ", "),
		URL:      data.PostingURL, Posted: data.ReleasedAt, Remote: data.Location.Remote,
	}, nil
}

func (client *Client) fetchATSJob(ctx context.Context, pageURL *url.URL, endpoint, provider, country, state, area string, profile Profile) ([]JobResult, error) {
	posting, err := client.fetchATSPosting(ctx, endpoint, provider)
	if err != nil {
		return nil, err
	}
	if posting.Company == "" {
		if parts := strings.Split(strings.Trim(pageURL.Path, "/"), "/"); len(parts) > 0 {
			posting.Company = parts[0]
		}
	}
	title := strings.TrimSpace(posting.Title)
	if title == "" || posting.Company == "" {
		return nil, nil
	}
	if !matchesArea(posting.Location, posting.Remote, country, state, area) {
		return nil, nil
	}
	description := jobs.CleanHTML(posting.Description)
	score, tags := jobs.ScoreJob(title, description, posting.Location, jobsProfile(profile))
	canonicalURL := posting.URL
	if !sameOrATSListingURL(pageURL.Hostname(), canonicalURL) && !isRecognizedATSHost(urlHost(canonicalURL)) {
		canonicalURL = pageURL.String()
	}
	return []JobResult{{
		ID: jobID(canonicalURL), Company: posting.Company, Role: title, Location: posting.Location,
		WorkMode: workMode(posting.Location, posting.Remote), Posted: formatDate(posting.Posted),
		Source: "Verified public career page", URL: canonicalURL, Score: score, Tags: tags,
		Status: "New", Logo: firstLetter(posting.Company), LogoTone: "blue", SourceBoard: pageURL.Hostname(),
	}}, nil
}

func urlHost(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

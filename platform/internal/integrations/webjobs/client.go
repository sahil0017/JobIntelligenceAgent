package webjobs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"careerpilot/contracts/domain"
)

const (
	searchEndpoint  = "https://api.search.brave.com/res/v1/web/search"
	maxSearchBody   = 2 << 20
	maxPageBody     = 2 << 20
	maxRobotsBody   = 512 << 10
	maxErrorBody    = 2 << 10
	maxPages        = 20
	maxPagesPerHost = 3
)

type Client struct {
	httpClient     *http.Client
	searchEndpoint string
}

// These types are shared with the intelligence layer through the contracts module.
type (
	Result    = domain.SearchResult
	JobResult = domain.SearchJob
	PageError = domain.PageError
	Event     = domain.SearchEvent
	Options   = domain.SearchOptions
	Profile   = domain.SearchProfile
)

type searchResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = newSafeHTTPClient()
	} else {
		safeClient := *httpClient
		safeClient.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
		httpClient = &safeClient
	}
	return &Client{httpClient: httpClient, searchEndpoint: searchEndpoint}
}

// searchPause spaces Brave requests; the free tier allows about one per second.
var searchPause = 1100 * time.Millisecond

const (
	defaultRounds        = 4
	defaultTargetMatches = 10
	defaultTotalPages    = 40
	defaultMinMatchScore = 40
)

func withDefaults(options Options) Options {
	if options.MaxRounds <= 0 {
		options.MaxRounds = defaultRounds
	}
	if options.TargetMatches <= 0 {
		options.TargetMatches = defaultTargetMatches
	}
	if options.MaxPages <= 0 {
		options.MaxPages = defaultTotalPages
	}
	if options.MinScore <= 0 {
		options.MinScore = defaultMinMatchScore
	}
	if options.OnEvent == nil {
		options.OnEvent = func(Event) {}
	}
	return options
}

type crawlState struct {
	robots     map[string]robotsPolicy
	hostCounts map[string]int
	visited    map[string]struct{}
	pagesRead  int
	maxPages   int
	result     *Result
}

// Search runs a bounded plan-search-verify-filter loop: it issues planned queries
// in rounds until enough resume-matching jobs are found or the page budget is spent.
func (client *Client) Search(ctx context.Context, apiKey, country, state, area string, profile Profile) (Result, error) {
	return client.SearchWithOptions(ctx, apiKey, country, state, area, profile, Options{})
}

// SearchWithOptions is Search with configurable limits and progress events.
func (client *Client) SearchWithOptions(ctx context.Context, apiKey, country, state, area string, profile Profile, options Options) (Result, error) {
	options = withDefaults(options)
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return Result{}, fmt.Errorf("Brave Search API key is required")
	}
	profile.Location = strings.TrimSpace(area)
	plan := planQueries(country, state, area, profile)
	if len(plan) > options.MaxRounds {
		plan = plan[:options.MaxRounds]
	}
	options.OnEvent(Event{Stage: "plan", Message: fmt.Sprintf("planned %d search rounds", len(plan))})
	result := Result{Jobs: []JobResult{}, SourceErrors: []PageError{}, Trace: []string{}}
	crawl := &crawlState{
		robots: map[string]robotsPolicy{}, hostCounts: map[string]int{},
		visited: map[string]struct{}{}, maxPages: options.MaxPages, result: &result,
	}
	for round, query := range plan {
		if round > 0 {
			select {
			case <-ctx.Done():
				return Result{}, ctx.Err()
			case <-time.After(searchPause):
			}
		}
		options.OnEvent(Event{Stage: "search", Message: fmt.Sprintf("round %d: %s", round+1, truncateRunes(query, 120))})
		found, err := client.braveSearch(ctx, apiKey, query, country)
		if err != nil {
			if round == 0 {
				return Result{}, err
			}
			message := fmt.Sprintf("round %d: search failed: %v", round+1, err)
			result.Trace = append(result.Trace, message)
			options.OnEvent(Event{Stage: "search", Message: message})
			continue
		}
		var fresh []searchResult
		for _, page := range uniqueSearchPages(found) {
			if _, seen := crawl.visited[page.URL]; !seen {
				crawl.visited[page.URL] = struct{}{}
				fresh = append(fresh, page)
			}
		}
		if err := client.processPages(ctx, crawl, fresh, country, state, area, profile, options.OnEvent); err != nil {
			return Result{}, err
		}
		qualified := len(deduplicateAndRank(result.Jobs))
		summary := fmt.Sprintf("round %d: %d new pages, %d jobs so far", round+1, len(fresh), qualified)
		result.Trace = append(result.Trace, summary)
		options.OnEvent(Event{Stage: "verify", Message: summary})
		if qualified >= options.TargetMatches || crawl.pagesRead >= crawl.maxPages {
			break
		}
	}
	result.Jobs = deduplicateAndRank(result.Jobs)
	if profile.Skills != "" || profile.ResumeText != "" {
		kept := result.Jobs[:0]
		for _, job := range result.Jobs {
			if job.Score >= options.MinScore {
				kept = append(kept, job)
			}
		}
		result.FilteredLowMatch = len(result.Jobs) - len(kept)
		result.Jobs = kept
		options.OnEvent(Event{Stage: "filter", Message: fmt.Sprintf("kept %d jobs scoring %d or higher; filtered %d weak matches", len(kept), options.MinScore, result.FilteredLowMatch)})
	}
	return result, nil
}

func (client *Client) processPages(ctx context.Context, crawl *crawlState, pages []searchResult, country, state, area string, profile Profile, onEvent func(Event)) error {
	result := crawl.result
	addError := func(rawURL, message string) {
		result.SourceErrors = append(result.SourceErrors, PageError{URL: rawURL, Error: message, Category: classifyError(message)})
	}
	for _, page := range pages {
		if err := ctx.Err(); err != nil {
			return err
		}
		if crawl.pagesRead >= crawl.maxPages {
			return nil
		}
		parsed, err := validatePageURL(page.URL)
		if err != nil {
			addError(page.URL, err.Error())
			continue
		}
		host := strings.ToLower(parsed.Hostname())
		if crawl.hostCounts[host] >= maxPagesPerHost {
			continue
		}
		policy, exists := crawl.robots[host]
		if !exists {
			policy, err = client.fetchRobots(ctx, parsed)
			if err != nil {
				addError(page.URL, err.Error())
				continue
			}
			crawl.robots[host] = policy
		}
		robotsPath := parsed.EscapedPath()
		if parsed.RawQuery != "" {
			robotsPath += "?" + parsed.RawQuery
		}
		if !policy.Allowed(robotsPath) {
			addError(page.URL, "career page disallows automated access in robots.txt")
			continue
		}
		crawl.hostCounts[host]++
		crawl.pagesRead++
		result.CheckedSources++
		var jobs []JobResult
		if endpoint, provider, ok := atsEndpoint(parsed); ok {
			jobs, err = client.fetchATSJob(ctx, parsed, endpoint, provider, country, state, area, profile)
		} else {
			jobs, err = client.fetchJobPage(ctx, parsed, country, state, area, profile)
		}
		if err != nil {
			addError(page.URL, err.Error())
			onEvent(Event{Stage: "verify", Message: fmt.Sprintf("skipped %s: %s", host, err)})
			continue
		}
		if len(jobs) == 0 {
			addError(page.URL, "no verified structured job postings were found")
			onEvent(Event{Stage: "verify", Message: fmt.Sprintf("no verified postings on %s", host)})
		} else {
			onEvent(Event{Stage: "verify", Message: fmt.Sprintf("verified %d posting(s) on %s", len(jobs), host)})
		}
		result.Jobs = append(result.Jobs, jobs...)
	}
	return nil
}

// classifyError groups skip reasons so the UI can report pages that need a login separately.
func classifyError(message string) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "http 401"), strings.Contains(lower, "http 403"):
		return "auth_required"
	case strings.Contains(lower, "http 404"), strings.Contains(lower, "http 410"),
		strings.Contains(lower, "no longer"):
		return "closed"
	case strings.Contains(lower, "robots.txt"):
		return "robots"
	case strings.Contains(lower, "redirected"):
		return "redirect"
	case strings.Contains(lower, "no verified"):
		return "no_data"
	default:
		return "error"
	}
}

// planQueries builds the query rounds: general, ATS-restricted, then skill-focused variants.
func planQueries(country, state, area string, profile Profile) []string {
	plan := []string{
		buildSearchQuery(country, state, area, profile),
		buildATSSearchQuery(country, state, area, profile),
	}
	role := strings.TrimSpace(profile.Title)
	if role == "" {
		role = "software engineering"
	}
	role = truncateRunes(role, 100)
	location := strings.Join(nonEmpty(area, state, country), " ")
	var skills []string
	for _, skill := range strings.Split(profile.Skills, ",") {
		if skill = strings.TrimSpace(skill); skill != "" {
			skills = append(skills, truncateRunes(skill, 40))
		}
	}
	if len(skills) >= 2 {
		plan = append(plan, truncateRunes(fmt.Sprintf("%s %s %s hiring %s careers", role, skills[0], skills[1], location), 400))
	}
	plan = append(plan, truncateRunes(fmt.Sprintf("%s %s open positions apply company careers", role, location), 400))
	return plan
}

type searchResult = struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

func (client *Client) braveSearch(ctx context.Context, apiKey, query, country string) ([]searchResult, error) {
	searchURL, err := url.Parse(client.searchEndpoint)
	if err != nil {
		return nil, fmt.Errorf("parse Brave Search endpoint: %w", err)
	}
	queryValues := searchURL.Query()
	queryValues.Set("q", query)
	queryValues.Set("count", "20")
	if code := countryCode(country); code != "" {
		queryValues.Set("country", code)
	}
	searchURL.RawQuery = queryValues.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create Brave Search request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-Subscription-Token", apiKey)
	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request Brave Search: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		detail, err := io.ReadAll(io.LimitReader(response.Body, maxErrorBody+1))
		if err != nil {
			return nil, fmt.Errorf("Brave Search returned HTTP %d (could not read error details: %w)", response.StatusCode, err)
		}
		if len(detail) > maxErrorBody {
			detail = detail[:maxErrorBody]
		}
		message := strings.TrimSpace(strings.ReplaceAll(string(detail), apiKey, "[redacted]"))
		if message == "" {
			return nil, fmt.Errorf("Brave Search returned HTTP %d", response.StatusCode)
		}
		return nil, fmt.Errorf("Brave Search returned HTTP %d: %s", response.StatusCode, message)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxSearchBody+1))
	if err != nil {
		return nil, fmt.Errorf("read Brave Search response: %w", err)
	}
	if len(body) > maxSearchBody {
		return nil, fmt.Errorf("Brave Search response exceeds the %d MiB size limit", maxSearchBody>>20)
	}
	var search searchResponse
	if err := json.Unmarshal(body, &search); err != nil {
		return nil, fmt.Errorf("decode Brave Search response: %w", err)
	}
	return search.Web.Results, nil
}

func buildSearchQuery(country, state, area string, profile Profile) string {
	location := strings.Join(nonEmpty(area, state, country), " ")
	role := strings.TrimSpace(profile.Title)
	if role == "" {
		role = "software engineering"
	}
	role = truncateRunes(role, 100)
	skills := strings.TrimSpace(profile.Skills)
	skills = truncateRunes(skills, 160)
	parts := []string{role}
	if skills != "" {
		parts = append(parts, skills)
	}
	parts = append(parts, "jobs careers openings", location, "official company careers")
	return truncateRunes(strings.Join(parts, " "), 400)
}

func buildATSSearchQuery(country, state, area string, profile Profile) string {
	location := strings.Join(nonEmpty(area, state, country), " ")
	role := strings.TrimSpace(profile.Title)
	if role == "" {
		role = "software engineering"
	}
	sites := "(site:boards.greenhouse.io OR site:job-boards.greenhouse.io OR site:jobs.lever.co OR site:jobs.ashbyhq.com OR site:jobs.smartrecruiters.com)"
	return truncateRunes(strings.Join([]string{truncateRunes(role, 100), location, sites}, " "), 400)
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

func nonEmpty(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func countryCode(country string) string {
	switch strings.ToLower(strings.TrimSpace(country)) {
	case "india":
		return "IN"
	case "united states", "united states of america", "usa":
		return "US"
	case "united kingdom", "uk":
		return "GB"
	case "canada":
		return "CA"
	case "australia":
		return "AU"
	case "germany":
		return "DE"
	default:
		return ""
	}
}

func uniqueSearchPages(results []searchResult) []searchResult {
	seen := make(map[string]struct{}, len(results))
	pages := make([]searchResult, 0, len(results))
	for _, result := range results {
		parsed, err := validatePageURL(result.URL)
		if err != nil {
			continue
		}
		if isBlockedAggregator(parsed.Hostname()) {
			continue
		}
		key := parsed.String()
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		pages = append(pages, result)
	}
	return pages
}

// Large job aggregators block automated access or serve no structured postings,
// so fetching them only wastes the page budget.
var blockedAggregatorDomains = []string{
	"indeed.com", "glassdoor.com", "glassdoor.co.in", "linkedin.com", "foundit.in",
	"simplyhired.com", "simplyhired.co.in", "naukri.com", "kitjob.in", "productbased.in",
	"ziprecruiter.com", "monster.com", "shine.com", "timesjobs.com",
}

func isBlockedAggregator(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, domain := range blockedAggregatorDomains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func validatePageURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		return nil, fmt.Errorf("search result is not a valid HTTPS career page")
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return nil, fmt.Errorf("career page uses a non-standard HTTPS port")
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || net.ParseIP(host) != nil {
		return nil, fmt.Errorf("career page host is not permitted")
	}
	parsed.Host = host
	parsed.Fragment = ""
	return parsed, nil
}

func newSafeHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		TLSHandshakeTimeout:   8 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			if port != "443" {
				return nil, fmt.Errorf("outbound HTTPS connections are limited to port 443")
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("resolve public career page host: %w", err)
			}
			for _, candidate := range ips {
				if !isPublicIP(candidate.IP) {
					return nil, fmt.Errorf("career page host resolves to a non-public IP address")
				}
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("career page host has no public IP addresses")
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
	}
	return &http.Client{
		Timeout:   12 * time.Second,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func isPublicIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	blockedCIDRs := []string{
		"100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15",
		"198.51.100.0/24", "203.0.113.0/24", "2001:db8::/32",
	}
	for _, cidr := range blockedCIDRs {
		_, network, _ := net.ParseCIDR(cidr)
		if network.Contains(ip) {
			return false
		}
	}
	return true
}

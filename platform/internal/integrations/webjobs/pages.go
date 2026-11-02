package webjobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/publicsuffix"

	"careerpilot/contracts/domain"
	"careerpilot/platform/internal/integrations/jobs"
)

const crawlerUserAgent = "CareerPilotBot"

type jobPosting struct {
	Type                  any    `json:"@type"`
	Title                 string `json:"title"`
	Description           string `json:"description"`
	URL                   string `json:"url"`
	DatePosted            string `json:"datePosted"`
	ValidThrough          string `json:"validThrough"`
	Identifier            any    `json:"identifier"`
	HiringOrganization    any    `json:"hiringOrganization"`
	JobLocation           any    `json:"jobLocation"`
	JobLocationType       string `json:"jobLocationType"`
	ApplicantRequirements any    `json:"applicantLocationRequirements"`
	BaseSalary            any    `json:"baseSalary"`
}

type robotsPolicy struct {
	rules []robotsRule
}

type robotsRule struct {
	path    string
	allowed bool
}

func (policy robotsPolicy) Allowed(path string) bool {
	matchedLength := -1
	allowed := true
	for _, rule := range policy.rules {
		if rule.path == "" || !robotsPathMatches(rule.path, path) {
			continue
		}
		if len(rule.path) > matchedLength || len(rule.path) == matchedLength && rule.allowed {
			matchedLength = len(rule.path)
			allowed = rule.allowed
		}
	}
	return allowed
}

func robotsPathMatches(pattern, path string) bool {
	endAnchored := strings.HasSuffix(pattern, "$")
	pattern = strings.TrimSuffix(pattern, "$")
	pattern = "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, `.*`)
	if endAnchored {
		pattern += "$"
	}
	matched, err := regexp.MatchString(pattern, path)
	return err == nil && matched
}

func (client *Client) fetchRobots(ctx context.Context, pageURL *url.URL) (robotsPolicy, error) {
	robotsURL := *pageURL
	robotsURL.Path = "/robots.txt"
	robotsURL.RawQuery = ""
	robotsURL.Fragment = ""
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL.String(), nil)
	if err != nil {
		return robotsPolicy{}, fmt.Errorf("create robots.txt request: %w", err)
	}
	request.Header.Set("User-Agent", crawlerUserAgent+"/0.1")
	response, err := client.httpClient.Do(request)
	if err != nil {
		return robotsPolicy{}, fmt.Errorf("check robots.txt: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return robotsPolicy{}, nil
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return robotsPolicy{}, fmt.Errorf("robots.txt returned HTTP %d; skipping this site", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxRobotsBody+1))
	if err != nil {
		return robotsPolicy{}, fmt.Errorf("read robots.txt: %w", err)
	}
	if len(body) > maxRobotsBody {
		return robotsPolicy{}, fmt.Errorf("robots.txt exceeds the size limit; skipping this site")
	}
	return parseRobots(string(body)), nil
}

func parseRobots(content string) robotsPolicy {
	policy := robotsPolicy{}
	var currentAgents []string
	var currentRules []robotsRule
	flush := func() {
		matches := false
		for _, agent := range currentAgents {
			if agent == "*" || strings.Contains(agent, strings.ToLower(crawlerUserAgent)) {
				matches = true
				break
			}
		}
		if matches {
			policy.rules = append(policy.rules, currentRules...)
		}
		currentAgents = nil
		currentRules = nil
	}
	groupHasRules := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		if line == "" {
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "user-agent":
			if groupHasRules {
				flush()
				groupHasRules = false
			}
			currentAgents = append(currentAgents, strings.ToLower(value))
		case "allow", "disallow":
			if len(currentAgents) == 0 || value == "" && key == "disallow" {
				continue
			}
			groupHasRules = true
			currentRules = append(currentRules, robotsRule{path: value, allowed: key == "allow"})
		}
	}
	flush()
	return policy
}

func (client *Client) fetchJobPage(ctx context.Context, pageURL *url.URL, country, state, area string, profile Profile) ([]JobResult, error) {
	var response *http.Response
	for redirects := 0; ; redirects++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("create career page request: %w", err)
		}
		request.Header.Set("User-Agent", crawlerUserAgent+"/0.1")
		request.Header.Set("Accept", "text/html,application/xhtml+xml")
		response, err = client.httpClient.Do(request)
		if err != nil {
			return nil, fmt.Errorf("fetch public career page: %w", err)
		}
		if !isRedirect(response.StatusCode) {
			break
		}
		location := response.Header.Get("Location")
		response.Body.Close()
		if redirects >= maxRedirects {
			return nil, fmt.Errorf("career page redirected too many times")
		}
		next, err := client.followableRedirect(ctx, pageURL, location)
		if err != nil {
			return nil, err
		}
		pageURL = next
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("career page returned HTTP %d", response.StatusCode)
	}
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if contentType != "" && !strings.Contains(contentType, "html") {
		return nil, fmt.Errorf("search result is not an HTML career page")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxPageBody+1))
	if err != nil {
		return nil, fmt.Errorf("read career page: %w", err)
	}
	if len(body) > maxPageBody {
		return nil, fmt.Errorf("career page exceeds the %d MiB size limit", maxPageBody>>20)
	}
	postings, err := extractJobPostings(body)
	if err != nil {
		return nil, err
	}
	results := make([]JobResult, 0, len(postings))
	for _, posting := range postings {
		if isExpired(posting.ValidThrough, time.Now()) {
			continue
		}
		if !isVerifiedPosting(pageURL.Hostname(), posting) {
			continue
		}
		location, remote := postingLocation(posting)
		if !matchesArea(location, remote, country, state, area) {
			continue
		}
		name := organizationName(posting.HiringOrganization)
		title := strings.TrimSpace(posting.Title)
		if name == "" || title == "" {
			continue
		}
		description := jobs.CleanHTML(posting.Description)
		score, tags := jobs.ScoreJob(title, description, location, jobsProfile(profile))
		canonicalURL := posting.URL
		if !sameOrATSListingURL(pageURL.Hostname(), canonicalURL) {
			canonicalURL = pageURL.String()
		}
		id := jobID(canonicalURL)
		results = append(results, JobResult{
			ID: id, Company: name, Role: title, Location: location,
			WorkMode: workMode(location, remote), Salary: salaryText(posting.BaseSalary),
			Posted: formatDate(posting.DatePosted), Source: "Verified public career page",
			URL: canonicalURL, Score: score, Tags: tags, Status: "New",
			Logo: firstLetter(name), LogoTone: "blue", SourceBoard: pageURL.Hostname(),
		})
	}
	return results, nil
}

const maxRedirects = 3

func isRedirect(status int) bool {
	switch status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	}
	return false
}

// followableRedirect accepts only public HTTPS targets on the same host or a
// recognized ATS host, and re-applies robots.txt to the target.
func (client *Client) followableRedirect(ctx context.Context, from *url.URL, location string) (*url.URL, error) {
	if strings.TrimSpace(location) == "" {
		return nil, fmt.Errorf("career page redirected without a location")
	}
	ref, err := url.Parse(location)
	if err != nil {
		return nil, fmt.Errorf("career page redirected to an invalid location")
	}
	target, err := validatePageURL(from.ResolveReference(ref).String())
	if err != nil {
		return nil, fmt.Errorf("career page redirected to a disallowed location: %w", err)
	}
	sameHost := strings.EqualFold(from.Hostname(), target.Hostname())
	if !sameHost && !(isRecognizedATSHost(from.Hostname()) && isRecognizedATSHost(target.Hostname())) {
		return nil, fmt.Errorf("career page redirected to an unrelated host")
	}
	policy, err := client.fetchRobots(ctx, target)
	if err != nil {
		return nil, err
	}
	path := target.EscapedPath()
	if target.RawQuery != "" {
		path += "?" + target.RawQuery
	}
	if !policy.Allowed(path) {
		return nil, fmt.Errorf("career page disallows automated access in robots.txt")
	}
	return target, nil
}

func isExpired(value string, now time.Time) bool {
	if value == "" {
		return false
	}
	if timestamp, err := time.Parse(time.RFC3339, value); err == nil {
		return timestamp.Before(now)
	}
	if date, err := time.Parse("2006-01-02", value); err == nil {
		return !now.Before(date.Add(24 * time.Hour))
	}
	return false
}

func extractJobPostings(body []byte) ([]jobPosting, error) {
	document, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("parse career page HTML: %w", err)
	}
	var postings []jobPosting
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "script" && attr(node, "type") == "application/ld+json" {
			var content strings.Builder
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				if child.Type == html.TextNode {
					content.WriteString(child.Data)
				}
			}
			var value any
			if json.Unmarshal([]byte(content.String()), &value) == nil {
				collectPostings(value, &postings)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(document)
	return postings, nil
}

func collectPostings(value any, postings *[]jobPosting) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			collectPostings(item, postings)
		}
	case map[string]any:
		if isJobPosting(typed["@type"]) {
			encoded, err := json.Marshal(typed)
			if err == nil {
				var posting jobPosting
				if json.Unmarshal(encoded, &posting) == nil {
					*postings = append(*postings, posting)
				}
			}
		}
		for key, child := range typed {
			if key == "@graph" || key == "mainEntity" || key == "itemListElement" || key == "hasPart" {
				collectPostings(child, postings)
			}
		}
	}
}

func isJobPosting(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.EqualFold(typed, "JobPosting")
	case []any:
		for _, item := range typed {
			if isJobPosting(item) {
				return true
			}
		}
	}
	return false
}

func isVerifiedPosting(pageHost string, posting jobPosting) bool {
	name := organizationName(posting.HiringOrganization)
	if name == "" {
		return false
	}
	if isRecognizedATSHost(pageHost) {
		return true
	}
	organizationURL := organizationURL(posting.HiringOrganization)
	parsed, err := url.Parse(organizationURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return false
	}
	pageDomain, pageErr := publicsuffix.EffectiveTLDPlusOne(strings.ToLower(pageHost))
	organizationDomain, organizationErr := publicsuffix.EffectiveTLDPlusOne(strings.ToLower(parsed.Hostname()))
	return pageErr == nil && organizationErr == nil && pageDomain == organizationDomain
}

func isRecognizedATSHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, suffix := range []string{
		"boards.greenhouse.io", "job-boards.greenhouse.io", "jobs.lever.co",
		"jobs.ashbyhq.com", "jobs.smartrecruiters.com", "myworkdayjobs.com",
		"jobs.jobvite.com", "jobs.personio.com", "careers.icims.com",
	} {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

func organizationName(value any) string {
	return valueString(value, "name")
}

func organizationURL(value any) string {
	return valueString(value, "url")
}

func valueString(value any, field string) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		if text, ok := typed[field].(string); ok {
			return strings.TrimSpace(text)
		}
		if text, ok := typed["@id"].(string); ok && field == "url" {
			return strings.TrimSpace(text)
		}
	case []any:
		for _, item := range typed {
			if text := valueString(item, field); text != "" {
				return text
			}
		}
	}
	return ""
}

func postingLocation(posting jobPosting) (string, bool) {
	locationValues := make([]string, 0, 8)
	collectAddress(posting.JobLocation, &locationValues)
	collectAddress(posting.ApplicantRequirements, &locationValues)
	location := strings.Join(uniqueStrings(locationValues), ", ")
	remote := strings.EqualFold(posting.JobLocationType, "TELECOMMUTE") ||
		strings.Contains(strings.ToLower(location), "remote")
	if remote && location == "" {
		location = "Remote"
	}
	return location, remote
}

func collectAddress(value any, values *[]string) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			collectAddress(item, values)
		}
	case map[string]any:
		for _, key := range []string{"addressLocality", "addressRegion", "addressCountry", "name"} {
			if text, ok := typed[key].(string); ok && strings.TrimSpace(text) != "" {
				*values = append(*values, strings.TrimSpace(text))
			}
		}
		if address, ok := typed["address"]; ok {
			collectAddress(address, values)
		}
	}
}

func matchesArea(location string, remote bool, country, state, area string) bool {
	location = strings.ToLower(location)
	if remote {
		return strings.TrimSpace(country) == "" || strings.Contains(location, strings.ToLower(country))
	}
	target := strings.TrimSpace(area)
	if target == "" {
		target = strings.TrimSpace(state)
	}
	if target == "" {
		target = strings.TrimSpace(country)
	}
	return target != "" && strings.Contains(location, strings.ToLower(target))
}

func sameOrATSListingURL(pageHost, rawURL string) bool {
	parsed, err := validatePageURL(rawURL)
	if err != nil {
		return false
	}
	listingHost := parsed.Hostname()
	if strings.EqualFold(pageHost, listingHost) || isRecognizedATSHost(pageHost) && isRecognizedATSHost(listingHost) {
		return true
	}
	pageDomain, pageErr := publicsuffix.EffectiveTLDPlusOne(pageHost)
	listingDomain, listingErr := publicsuffix.EffectiveTLDPlusOne(listingHost)
	return pageErr == nil && listingErr == nil && pageDomain == listingDomain
}

func jobsProfile(profile Profile) domain.CandidateProfile {
	return domain.CandidateProfile{
		Skills: profile.Skills, Title: profile.Title,
		Location: profile.Location, ResumeText: profile.ResumeText,
	}
}

func workMode(location string, remote bool) string {
	if remote {
		return "Remote"
	}
	return jobs.WorkMode(location)
}

func salaryText(value any) string {
	root, ok := value.(map[string]any)
	if !ok {
		return "Not listed"
	}
	valueNode, ok := root["value"].(map[string]any)
	if !ok {
		return "Not listed"
	}
	minimum := numberText(valueNode["minValue"])
	maximum := numberText(valueNode["maxValue"])
	currency, _ := root["currency"].(string)
	if minimum == "" && maximum == "" {
		return "Not listed"
	}
	separator := " - "
	switch {
	case minimum == "":
		return strings.TrimSpace(currency + " " + maximum)
	case maximum == "":
		return strings.TrimSpace(currency + " " + minimum)
	default:
		return strings.TrimSpace(currency + " " + minimum + separator + maximum)
	}
}

func numberText(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func formatDate(value string) string {
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if timestamp, err := time.Parse(layout, value); err == nil {
			days := int(time.Since(timestamp).Hours() / 24)
			switch {
			case days <= 0:
				return "Today"
			case days == 1:
				return "1 day ago"
			case days < 30:
				return fmt.Sprintf("%d days ago", days)
			default:
				return timestamp.Local().Format("Jan 2, 2006")
			}
		}
	}
	return "Date not listed"
}

func jobID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "web:" + hex.EncodeToString(sum[:12])
}

func firstLetter(name string) string {
	for _, character := range strings.TrimSpace(name) {
		return strings.ToUpper(string(character))
	}
	return "?"
}

func attr(node *html.Node, key string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == key {
			return strings.ToLower(strings.TrimSpace(attribute.Val))
		}
	}
	return ""
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		key := strings.ToLower(value)
		if !seen[key] {
			seen[key] = true
			result = append(result, value)
		}
	}
	return result
}

func deduplicateAndRank(values []JobResult) []JobResult {
	seen := make(map[string]bool, len(values))
	result := make([]JobResult, 0, len(values))
	for _, value := range values {
		if seen[value.ID] {
			continue
		}
		seen[value.ID] = true
		result = append(result, value)
	}
	for i := 1; i < len(result); i++ {
		for j := i; j > 0 && result[j].Score > result[j-1].Score; j-- {
			result[j], result[j-1] = result[j-1], result[j]
		}
	}
	if len(result) > 300 {
		result = result[:300]
	}
	return result
}

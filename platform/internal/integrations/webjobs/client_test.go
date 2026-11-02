package webjobs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func init() { searchPause = 0 }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (handler roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return handler(request)
}

func TestSearchFindsVerifiedPublicJobAndScoresLocally(t *testing.T) {
	client := NewClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Host + request.URL.Path {
		case "api.search.brave.com/res/v1/web/search":
			if request.Header.Get("X-Subscription-Token") != "test-api-key" {
				t.Fatalf("Brave API key missing from request header")
			}
			query := request.URL.Query().Get("q")
			if !strings.Contains(query, "Backend Engineer") || !strings.Contains(query, "Bengaluru") ||
				strings.Contains(query, "private resume phrase") {
				t.Fatalf("unexpected or overly sensitive search query: %q", query)
			}
			return jsonResponse(`{"web":{"results":[{"title":"Backend Engineer","url":"https://careers.acme.com/jobs/123"}]}}`), nil
		case "careers.acme.com/robots.txt":
			return textResponse(http.StatusOK, "User-agent: *\nAllow: /jobs/"), nil
		case "careers.acme.com/jobs/123":
			return textResponse(http.StatusOK, `<html><script type="application/ld+json">
				{"@context":"https://schema.org","@type":"JobPosting","title":"Backend Engineer",
				"description":"Go and Kubernetes services","datePosted":"2026-10-29",
				"url":"https://careers.acme.com/jobs/123",
				"hiringOrganization":{"@type":"Organization","name":"Acme Systems","url":"https://www.acme.com"},
				"jobLocation":{"address":{"addressLocality":"Bengaluru","addressRegion":"Karnataka","addressCountry":"India"}}}
			</script></html>`), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", request.URL)
		}
	})})

	result, err := client.Search(context.Background(), "test-api-key", "India", "Karnataka", "Bengaluru", Profile{
		Title: "Backend Engineer", Skills: "Go, Kubernetes", ResumeText: "private resume phrase Go",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.CheckedSources != 1 || len(result.Jobs) != 1 {
		t.Fatalf("expected one verified job, got %#v", result)
	}
	job := result.Jobs[0]
	if job.Company != "Acme Systems" || job.Score < 80 || job.Source != "Verified public career page" {
		t.Fatalf("unexpected normalized job: %#v", job)
	}
}

func TestSearchExcludesUnverifiedAggregatorsAndDisallowedPages(t *testing.T) {
	client := NewClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Host + request.URL.Path {
		case "api.search.brave.com/res/v1/web/search":
			return jsonResponse(`{"web":{"results":[
				{"url":"https://jobs.example.com/blocked"},
				{"url":"https://jobs.aggregator.example/listing"}
			]}}`), nil
		case "jobs.example.com/robots.txt":
			return textResponse(http.StatusOK, "User-agent: *\nDisallow: /blocked"), nil
		case "jobs.aggregator.example/robots.txt":
			return textResponse(http.StatusNotFound, ""), nil
		case "jobs.aggregator.example/listing":
			return textResponse(http.StatusOK, `<script type="application/ld+json">
				{"@type":"JobPosting","title":"Engineer","hiringOrganization":{"name":"Acme","url":"https://acme.com"}}
			</script>`), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", request.URL)
		}
	})})
	result, err := client.Search(context.Background(), "key", "India", "", "Bengaluru", Profile{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Jobs) != 0 || len(result.SourceErrors) != 2 ||
		!strings.Contains(result.SourceErrors[0].Error, "robots.txt") {
		t.Fatalf("expected blocked and unverified sources to be omitted: %#v", result)
	}
}

func TestSearchIncludesBraveErrorDetailWithoutExposingAPIKey(t *testing.T) {
	const apiKey = "private-test-api-key"
	client := NewClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "api.search.brave.com" {
			t.Fatalf("unexpected request host: %s", request.URL.Host)
		}
		return textResponse(http.StatusUnprocessableEntity, `{"error":"VALIDATION","detail":"invalid country parameter","token":"private-test-api-key"}`), nil
	})})

	_, err := client.Search(context.Background(), apiKey, "India", "Karnataka", "Bengaluru", Profile{})
	if err == nil || !strings.Contains(err.Error(), "invalid country parameter") {
		t.Fatalf("expected provider validation detail, got %v", err)
	}
	if strings.Contains(err.Error(), apiKey) {
		t.Fatal("provider error must not expose the API key")
	}
}

func TestRobotsPolicyUsesLongestMatchingRuleAndAllowTie(t *testing.T) {
	policy := parseRobots("User-agent: *\nDisallow: /jobs\nAllow: /jobs/public\n")
	if policy.Allowed("/jobs/hidden") || !policy.Allowed("/jobs/public/1") {
		t.Fatalf("robots policy did not apply the most-specific matching rule: %#v", policy)
	}
	wildcards := parseRobots("User-agent: *\nDisallow: /*?preview=1$\n")
	if wildcards.Allowed("/jobs/123?preview=1") || !wildcards.Allowed("/jobs/123?preview=0") {
		t.Fatal("robots wildcard or end-anchor rule was not applied")
	}
}

func TestValidatePageURLRejectsUnsafeTargets(t *testing.T) {
	for _, raw := range []string{
		"http://example.com/job",
		"https://127.0.0.1/job",
		"https://example.com:8443/job",
		"https://user:pass@example.com/job",
	} {
		if _, err := validatePageURL(raw); err == nil {
			t.Errorf("validatePageURL(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestExpiredJobPostingsAreExcluded(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	if !isExpired("2026-10-29", now) || isExpired("2026-10-30", now) || isExpired("", now) {
		t.Fatal("job posting expiry date was not handled as expected")
	}
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(body)),
	}
}

func textResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status, Header: http.Header{"Content-Type": []string{"text/html"}},
		Body: io.NopCloser(strings.NewReader(body)),
	}
}

func TestCountryCodeAndSearchQuery(t *testing.T) {
	if countryCode("India") != "IN" || countryCode("unknown country") != "" {
		t.Fatal("country code mapping is incorrect")
	}
	if got := buildSearchQuery("India", "Karnataka", "Bengaluru", Profile{Title: "Engineer"}); !strings.Contains(got, "official company careers") {
		t.Fatalf("search query does not target official career pages: %q", got)
	}
}

func TestIsBlockedAggregator(t *testing.T) {
	cases := map[string]bool{
		"in.indeed.com":        true,
		"www.glassdoor.co.in":  true,
		"in.linkedin.com":      true,
		"foundit.in":           true,
		"notindeed.com":        false,
		"boards.greenhouse.io": false,
		"career.luxoft.com":    false,
	}
	for host, want := range cases {
		if got := isBlockedAggregator(host); got != want {
			t.Errorf("isBlockedAggregator(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestSearchFollowsSameHostRedirectButNotUnrelatedHosts(t *testing.T) {
	client := NewClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Host + request.URL.Path {
		case "api.search.brave.com/res/v1/web/search":
			return jsonResponse(`{"web":{"results":[
				{"url":"https://careers.acme.com/jobs/old"},
				{"url":"https://careers.acme.com/jobs/evil"}
			]}}`), nil
		case "careers.acme.com/robots.txt":
			return textResponse(http.StatusNotFound, ""), nil
		case "careers.acme.com/jobs/old":
			response := textResponse(http.StatusMovedPermanently, "")
			response.Header.Set("Location", "/jobs/new")
			return response, nil
		case "careers.acme.com/jobs/evil":
			response := textResponse(http.StatusFound, "")
			response.Header.Set("Location", "https://evil.example/jobs/2")
			return response, nil
		case "careers.acme.com/jobs/new":
			return textResponse(http.StatusOK, `<script type="application/ld+json">
				{"@type":"JobPosting","title":"Backend Engineer","url":"https://careers.acme.com/jobs/new",
				"hiringOrganization":{"name":"Acme","url":"https://acme.com"},
				"jobLocation":{"address":{"addressLocality":"Bengaluru","addressCountry":"India"}}}
			</script>`), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", request.URL)
		}
	})})
	result, err := client.Search(context.Background(), "k", "India", "Karnataka", "Bengaluru", Profile{Title: "Backend Engineer"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Jobs) != 1 || len(result.SourceErrors) != 1 {
		t.Fatalf("expected one job and one rejected redirect, got %#v", result)
	}
}

func TestSearchReadsATSPostingsFromPublicAPIs(t *testing.T) {
	client := NewClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Host + request.URL.Path {
		case "api.search.brave.com/res/v1/web/search":
			return jsonResponse(`{"web":{"results":[
				{"url":"https://job-boards.greenhouse.io/coupang/jobs/7953042"},
				{"url":"https://jobs.smartrecruiters.com/SWIGGY/6000000001349356-backend"},
				{"url":"https://jobs.smartrecruiters.com/ACME/6000000000000001-backend"},
				{"url":"https://jobs.lever.co/Onehouse/2e0958a7-d86e-4b9e-9f81-b18bdf47f157/apply"}
			]}}`), nil
		case "job-boards.greenhouse.io/robots.txt", "jobs.smartrecruiters.com/robots.txt", "jobs.lever.co/robots.txt":
			return textResponse(http.StatusNotFound, ""), nil
		case "boards-api.greenhouse.io/v1/boards/coupang/jobs/7953042":
			return jsonResponse(`{"title":"Senior Backend Engineer","company_name":"Coupang","absolute_url":"https://www.coupang.jobs/en/jobs/?gh_jid=7953042","content":"Go services","updated_at":"2026-10-30T21:36:39-04:00","location":{"name":"Bengaluru"}}`), nil
		case "api.smartrecruiters.com/v1/companies/SWIGGY/postings/6000000001349356":
			return jsonResponse(`{"name":"Backend","active":false,"company":{"name":"Swiggy"},"location":{"city":"Bengaluru"}}`), nil
		case "api.smartrecruiters.com/v1/companies/ACME/postings/6000000000000001":
			return jsonResponse(`{"name":"Backend Engineer","active":true,"postingUrl":"https://jobs.smartrecruiters.com/ACME/6000000000000001-backend","company":{"name":"Acme"},"location":{"city":"Bengaluru","region":"KA","country":"in"}}`), nil
		case "api.lever.co/v0/postings/Onehouse/2e0958a7-d86e-4b9e-9f81-b18bdf47f157":
			return jsonResponse(`{"text":"Backend Engineer","hostedUrl":"https://jobs.lever.co/Onehouse/2e0958a7-d86e-4b9e-9f81-b18bdf47f157","createdAt":1742888853247,"workplaceType":"remote","categories":{"location":"India-Remote"}}`), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", request.URL)
		}
	})})
	result, err := client.Search(context.Background(), "k", "India", "Karnataka", "Bengaluru", Profile{Title: "Backend Engineer"})
	if err != nil {
		t.Fatal(err)
	}
	companies := map[string]bool{}
	for _, job := range result.Jobs {
		companies[job.Company] = true
	}
	if len(result.Jobs) != 3 || !companies["Coupang"] || !companies["Acme"] || !companies["Onehouse"] || companies["Swiggy"] {
		t.Fatalf("unexpected jobs: %#v errors: %#v", result.Jobs, result.SourceErrors)
	}
}

func TestClassifyError(t *testing.T) {
	cases := map[string]string{
		"career page returned HTTP 403":                        "auth_required",
		"robots.txt returned HTTP 401; skipping this site":     "auth_required",
		"career page returned HTTP 404":                        "closed",
		"smartrecruiters posting is no longer active":          "closed",
		"career page disallows automated access in robots.txt": "robots",
		"career page redirected to an unrelated host":          "redirect",
		"no verified structured job postings were found":       "no_data",
		"fetch public career page: timeout":                    "error",
	}
	for message, want := range cases {
		if got := classifyError(message); got != want {
			t.Errorf("classifyError(%q) = %q, want %q", message, got, want)
		}
	}
}

func TestSearchRunsLaterRoundsAndFiltersWeakResumeMatches(t *testing.T) {
	var queries []string
	client := NewClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Host + request.URL.Path {
		case "api.search.brave.com/res/v1/web/search":
			queries = append(queries, request.URL.Query().Get("q"))
			if len(queries) == 1 {
				return jsonResponse(`{"web":{"results":[{"url":"https://careers.acme.com/jobs/strong"},{"url":"https://careers.acme.com/jobs/login"}]}}`), nil
			}
			return jsonResponse(`{"web":{"results":[{"url":"https://careers.beta.com/jobs/weak"}]}}`), nil
		case "careers.acme.com/robots.txt", "careers.beta.com/robots.txt":
			return textResponse(http.StatusNotFound, ""), nil
		case "careers.acme.com/jobs/login":
			return textResponse(http.StatusForbidden, ""), nil
		case "careers.acme.com/jobs/strong":
			return textResponse(http.StatusOK, `<script type="application/ld+json">
				{"@type":"JobPosting","title":"Go Backend Engineer","description":"Go Kubernetes microservices","url":"https://careers.acme.com/jobs/strong",
				"hiringOrganization":{"name":"Acme","url":"https://acme.com"},
				"jobLocation":{"address":{"addressLocality":"Bengaluru","addressCountry":"India"}}}
			</script>`), nil
		case "careers.beta.com/jobs/weak":
			return textResponse(http.StatusOK, `<script type="application/ld+json">
				{"@type":"JobPosting","title":"Office Manager","description":"Facilities","url":"https://careers.beta.com/jobs/weak",
				"hiringOrganization":{"name":"Beta","url":"https://beta.com"},
				"jobLocation":{"address":{"addressLocality":"Bengaluru","addressCountry":"India"}}}
			</script>`), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", request.URL)
		}
	})})
	result, err := client.Search(context.Background(), "k", "India", "Karnataka", "Bengaluru", Profile{
		Title: "Go Backend Engineer", Skills: "Go, Kubernetes", ResumeText: "Go Kubernetes microservices",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) < 2 {
		t.Fatalf("expected later search rounds, got %d queries", len(queries))
	}
	if len(result.Jobs) != 1 || result.Jobs[0].Company != "Acme" || result.FilteredLowMatch != 1 {
		t.Fatalf("expected only the strong match, got %#v (filtered %d)", result.Jobs, result.FilteredLowMatch)
	}
	if len(result.SourceErrors) != 1 || result.SourceErrors[0].Category != "auth_required" {
		t.Fatalf("expected the 403 page reported as auth_required, got %#v", result.SourceErrors)
	}
}

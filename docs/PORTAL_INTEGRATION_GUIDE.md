# Job portal integration guide

This guide describes how to add permitted job-listing integrations to CareerPilot. Use the API version each provider officially supports; do not assume every portal has a v2 API. Keep provider-specific HTTP, authentication, and response formats inside `platform/internal/integrations/<provider>/`.

## Current support

| Provider | API used or status | What CareerPilot supports |
| --- | --- | --- |
| Greenhouse | Public Job Board API v1 | Reads published jobs and descriptions by board token. |
| Lever | Public Postings API v0 | Reads published jobs by company site. Lever also documents an apply endpoint, but CareerPilot does not submit applications. |
| Brave Search | Web Search API v1 | Discovers candidate public career pages for location-based job search; page content is accepted only when it contains structured job data and verifies to an employer domain or recognized ATS. |
| LinkedIn | OIDC `/v2/userinfo`; job APIs require approved partner access | Profile connection only. OIDC access does not provide job search. |
| Indeed | No public, general-purpose job-search API confirmed for this integration | Not implemented. Use only an official API and credentials made available to the app. |
| Naukri | No public, general-purpose job-search API confirmed for this integration | Not implemented. Use only an official API and credentials made available to the app. |
| Custom | Depends on the named provider and its official API | Configuration records only; user-supplied portal records do not trigger arbitrary URL fetching. |

Portal API availability can depend on product approval, contracts, and account eligibility. Re-check the provider documentation and access terms before implementing or enabling an adapter. Do not substitute browser scraping or undocumented endpoints when access is unavailable.

## Location-based public job discovery

The **Area job search** uses the official [Brave Search Web Search API](https://api.search.brave.com/app/documentation/web-search/get-started) to find candidate career pages from the selected location and target role. Save a Brave Search API key in Settings; the key is encrypted in local SQLite and is never returned by the settings API. The query contains the selected location, target role, and skills; parsed resume text remains local and is used only for match scoring.

The crawler has bounded result/body/time limits, rejects non-HTTPS and non-standard ports, blocks non-public IP destinations, does not follow redirects, and checks `robots.txt` before fetching each page. It extracts Schema.org `JobPosting` records and accepts them only when the page is on a recognized ATS host or the posting's hiring-organization URL shares the page's registered domain. It does not sign in, bypass access controls, or crawl links outside the search results. Unsupported pages, pages without structured listings, and pages disallowing crawling are skipped.

Search coverage is inherently incomplete: web search indexing, employer career pages, ATS coverage, location metadata, and robots policies vary. CareerPilot reports the pages checked and does not claim to find every employer or vacancy. Existing Greenhouse and Lever boards can also be scanned separately from Job matches → Sources. Google Places is not required for this workflow and does not provide job listings.

## Architecture

Job discovery depends on the provider contract in `platform/internal/integrations/jobs/provider.go`:

```go
type Provider interface {
    Name() domain.Provider
    Fetch(context.Context, domain.CareerSource, domain.CandidateProfile) ([]domain.Job, error)
}
```

The discovery service resolves providers through `jobs.Registry`. Provider clients normalize vendor responses into `domain.Job`; the discovery workflow and frontend do not depend on vendor-specific types.

An adapter should follow the established package layout:

```text
integrations/<provider>/
  client.go    HTTP client and provider configuration
  jobs.go      Provider interface and job retrieval
  response.go  Vendor response types
  mapper.go    Vendor response to domain.Job
  *_test.go    Contract, mapping, and error tests
```

Keep OAuth code separate when a provider requires it. Do not put credentials in a board ID, URL, notes field, source record, or frontend code. Persist user-managed OAuth client secrets through the existing encrypted integration-configuration storage.

## Adding a job-discovery adapter

1. **Confirm access and API terms.** Record the official API documentation, supported API version, required product approval, authentication method, rate limits, and permitted use. Do not proceed on the basis of an unofficial endpoint.
2. **Add a provider identifier.** Add a `domain.Provider` constant and update `jobs.ValidateSource` to accept it. Validate the provider-specific board or account identifier; do not accept arbitrary request URLs.
3. **Implement the adapter.** Implement `jobs.Provider`, use the shared HTTP helper where appropriate, honor context cancellation, set explicit timeouts, bound response sizes, and return errors for HTTP or decoding failures.
4. **Map provider data.** Keep vendor response structs private to the integration. Map stable IDs, company, role, location, work mode, salary, date, and canonical job URL into `domain.Job`. Preserve unknown optional values as empty rather than fabricating data.
5. **Register the provider.** Add the client to the registry in `platform/internal/app/application.go`. Keep provider setup out of discovery workflow code.
6. **Expose configuration consistently.** Update source validation and the source-management UI/API only when the adapter is usable. If credentials or partner access are missing, report that state instead of claiming the portal is searchable.
7. **Test without live credentials.** Use `httptest.Server` and representative fixtures. Verify the exact method, path, query, required headers, mapping, pagination, non-2xx responses, malformed JSON, oversized bodies, and cancellation. Add an API-level test for adding the source and running discovery.
8. **Keep application actions separate.** A read-only listings adapter must not submit applications. Any future submission capability needs an independently approved adapter and explicit per-application user confirmation.

For paginated endpoints, define a bounded page/result limit and stop when the provider indicates there are no more results. Handle rate limits according to provider guidance; do not retry non-idempotent operations automatically.

## Existing provider endpoints

### Greenhouse

- Official guide: [Greenhouse Job Board API](https://developers.greenhouse.io/job-board.html)
- Listing endpoint: `GET https://boards-api.greenhouse.io/v1/boards/{board_token}/jobs`
- CareerPilot requests `content=true` to include descriptions.
- Public job listing reads do not need credentials. Greenhouse documents authentication for its application-submission endpoint; that endpoint is not used by CareerPilot.
- The board token is the identifier in the employer's Greenhouse job-board URL.

### Lever

- Official guide: [Lever Postings API](https://github.com/Lever/postings-api)
- Global listing endpoint: `GET https://api.lever.co/v0/postings/{site}?mode=json`
- Lever also documents an EU API host. Add region support only when source configuration can represent and validate it explicitly; do not guess a region.
- The site is the employer's Lever postings identifier. Published listings are public.
- CareerPilot reads listings only; it does not call Lever's apply endpoint.

### LinkedIn

- Official job API overview: [LinkedIn Job Posting API](https://learn.microsoft.com/en-us/linkedin/talent/job-postings/api/overview)
- Official OIDC user info: [LinkedIn OpenID Connect](https://learn.microsoft.com/en-us/linkedin/consumer/integrations/self-serve/sign-in-with-linkedin-v2)
- The existing `/v2/userinfo` use is for identity/profile data. LinkedIn's Job Posting API is partner-restricted and is for posting jobs, not general job-seeker search. Do not infer job-search access from the LinkedIn sign-in scopes.
- The existing LinkedIn `JobsClient` is an isolated boundary for an approved endpoint and mapper; it is not registered as a job-search provider.

### Indeed, Naukri, and custom portals

Before adding an adapter, obtain the applicable official API documentation and access credentials/approval from the provider. Confirm that the API permits job discovery for this product and its users. Indeed's [developer documentation](https://docs.indeed.com/) is partner-oriented; no general public search endpoint has been verified for CareerPilot. No official public Naukri job-search API has been verified for CareerPilot. Until provider access is confirmed, these remain configuration-only records.

## Verification commands

From the repository root:

```bash
go test ./platform/internal/integrations/<provider> ./platform/internal/integrations/jobs ./platform/internal/services/discovery ./platform/internal/api
go test ./...
go vet ./...
```

Run the console production build after changing source configuration UI:

```bash
npm run build
```

## Official references

- [Greenhouse Job Board API](https://developers.greenhouse.io/job-board.html)
- [Lever Postings API](https://github.com/Lever/postings-api)
- [LinkedIn Job Posting API](https://learn.microsoft.com/en-us/linkedin/talent/job-postings/api/overview)
- [LinkedIn OpenID Connect](https://learn.microsoft.com/en-us/linkedin/consumer/integrations/self-serve/sign-in-with-linkedin-v2)
- [Indeed developer documentation](https://docs.indeed.com/)

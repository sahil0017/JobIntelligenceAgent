// Package cortex is the Go client for the Python Cortex AI service. It carries data across the
// process boundary and never interprets analysis results: those are passed through verbatim.
package cortex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	DefaultURL      = "http://127.0.0.1:8890"
	maxResponseBody = 4 << 20
)

// ErrUnavailable reports that the Cortex service could not be reached.
var ErrUnavailable = errors.New("the AI service is not running; start it with `npm run dev:ai`")

// ErrNotFound reports an unknown analysis run.
var ErrNotFound = errors.New("analysis run not found")

type Profile struct {
	Title      string `json:"title"`
	Skills     string `json:"skills"`
	Roles      string `json:"roles"`
	Experience string `json:"experience"`
}

type Job struct {
	ID          string   `json:"id"`
	Company     string   `json:"company"`
	Role        string   `json:"role"`
	Location    string   `json:"location"`
	Tags        []string `json:"tags"`
	Description string   `json:"description,omitempty"`
}

type AnalysisRequest struct {
	Profile    Profile `json:"profile"`
	ResumeText string  `json:"resumeText"`
	Jobs       []Job   `json:"jobs"`
}

// ApplicationRequest asks the Application Copilot to draft an application for one job.
type ApplicationRequest struct {
	Profile     Profile `json:"profile"`
	ResumeText  string  `json:"resumeText"`
	Job         Job     `json:"job"`
	URL         string  `json:"url,omitempty"`
	DraftAnyway bool    `json:"draftAnyway,omitempty"`
}

// Decision is the human's answer to a draft awaiting approval; nil fields keep the draft's text.
// PortalSearchRequest asks Cortex to list and rank jobs on a company's careers page.
type PortalSearchRequest struct {
	URL        string  `json:"url"`
	Keywords   string  `json:"keywords,omitempty"`
	Profile    Profile `json:"profile"`
	ResumeText string  `json:"resumeText"`
}

// Applicant holds the contact details the Apply agent pre-fills into a form.
type Applicant struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone,omitempty"`
	Location string `json:"location"`
}

type Decision struct {
	Approve         bool      `json:"approve"`
	TailoredBullets *[]string `json:"tailoredBullets,omitempty"`
	CoverNote       *string   `json:"coverNote,omitempty"`
	OutreachMessage *string   `json:"outreachMessage,omitempty"`
}

type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient returns a client for the given base URL; an empty URL uses CORTEX_URL or the default.
func NewClient(baseURL string, httpClient *http.Client) (*Client, error) {
	if baseURL == "" {
		baseURL = os.Getenv("CORTEX_URL")
	}
	if baseURL == "" {
		baseURL = DefaultURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("CORTEX_URL must be an http(s) URL, got %q", baseURL)
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(parsed.String(), "/"), http: httpClient}, nil
}

func (client *Client) Health(ctx context.Context) (json.RawMessage, error) {
	return client.do(ctx, http.MethodGet, "/health", nil)
}

func (client *Client) Pipeline(ctx context.Context) (json.RawMessage, error) {
	return client.do(ctx, http.MethodGet, "/v1/pipeline", nil)
}

func (client *Client) Start(ctx context.Context, request AnalysisRequest) (json.RawMessage, error) {
	return client.do(ctx, http.MethodPost, "/v1/analyses", request)
}

func (client *Client) List(ctx context.Context) (json.RawMessage, error) {
	return client.do(ctx, http.MethodGet, "/v1/analyses", nil)
}

func (client *Client) Get(ctx context.Context, id string) (json.RawMessage, error) {
	return client.do(ctx, http.MethodGet, "/v1/analyses/"+url.PathEscape(id), nil)
}

func (client *Client) Cancel(ctx context.Context, id string) (json.RawMessage, error) {
	return client.do(ctx, http.MethodPost, "/v1/analyses/"+url.PathEscape(id)+"/cancel", nil)
}

func (client *Client) ApplicationPipeline(ctx context.Context) (json.RawMessage, error) {
	return client.do(ctx, http.MethodGet, "/v1/application-pipeline", nil)
}

func (client *Client) StartApplication(ctx context.Context, request ApplicationRequest) (json.RawMessage, error) {
	return client.do(ctx, http.MethodPost, "/v1/applications", request)
}

func (client *Client) Applications(ctx context.Context) (json.RawMessage, error) {
	return client.do(ctx, http.MethodGet, "/v1/applications", nil)
}

func (client *Client) Application(ctx context.Context, id string) (json.RawMessage, error) {
	return client.do(ctx, http.MethodGet, "/v1/applications/"+url.PathEscape(id), nil)
}

func (client *Client) DecideApplication(ctx context.Context, id string, decision Decision) (json.RawMessage, error) {
	return client.do(ctx, http.MethodPost, "/v1/applications/"+url.PathEscape(id)+"/decision", decision)
}

func (client *Client) ApplyApplication(ctx context.Context, id string, applicant Applicant) (json.RawMessage, error) {
	return client.do(ctx, http.MethodPost, "/v1/applications/"+url.PathEscape(id)+"/apply", applicant)
}

type QuickApply struct {
	Applicant
	URL string `json:"url"`
}

func (client *Client) QuickApply(ctx context.Context, request QuickApply) (json.RawMessage, error) {
	return client.do(ctx, http.MethodPost, "/v1/apply", request)
}

func (client *Client) CancelApplication(ctx context.Context, id string) (json.RawMessage, error) {
	return client.do(ctx, http.MethodPost, "/v1/applications/"+url.PathEscape(id)+"/cancel", nil)
}

func (client *Client) PortalSearch(ctx context.Context, request PortalSearchRequest) (json.RawMessage, error) {
	return client.do(ctx, http.MethodPost, "/v1/portals/search", request)
}

func (client *Client) PortalSession(ctx context.Context) (json.RawMessage, error) {
	return client.do(ctx, http.MethodGet, "/v1/portals/session", nil)
}

func (client *Client) PortalSignIn(ctx context.Context, address string) (json.RawMessage, error) {
	return client.do(ctx, http.MethodPost, "/v1/portals/signin", map[string]string{"url": address})
}

func (client *Client) PortalSignOut(ctx context.Context) (json.RawMessage, error) {
	return client.do(ctx, http.MethodDelete, "/v1/portals/session", nil)
}

func (client *Client) do(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode AI service request: %w", err)
		}
		reader = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(ctx, method, client.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w (%v)", ErrUnavailable, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBody))
	if err != nil {
		return nil, fmt.Errorf("read AI service response: %w", err)
	}
	switch {
	case response.StatusCode == http.StatusNotFound:
		return nil, ErrNotFound
	case response.StatusCode == http.StatusConflict:
		return nil, fmt.Errorf("%w: %s", ErrConflict, detail(data))
	case response.StatusCode == http.StatusUnprocessableEntity:
		return nil, fmt.Errorf("%w: %s", ErrRejected, detail(data))
	case response.StatusCode >= 400:
		return nil, fmt.Errorf("AI service returned HTTP %d: %s", response.StatusCode, firstLine(data))
	}
	return json.RawMessage(data), nil
}

// ErrRejected reports a request the AI service understood but could not carry out, such as an unreadable page.
var ErrRejected = errors.New("the AI service could not complete that request")

// ErrConflict reports a request the run's current state cannot accept, such as deciding twice.
var ErrConflict = errors.New("the run cannot accept that request right now")

func detail(data []byte) string {
	var body struct {
		Detail string `json:"detail"`
	}
	if json.Unmarshal(data, &body) == nil && body.Detail != "" {
		return body.Detail
	}
	return firstLine(data)
}

func firstLine(data []byte) string {
	text := strings.TrimSpace(string(data))
	if len(text) > 300 {
		text = text[:300]
	}
	return text
}

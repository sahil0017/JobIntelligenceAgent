package greenhouse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"careerpilot/contracts/domain"
)

func TestClientFetchesAndMapsBoardListings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/boards/example/jobs" || request.URL.Query().Get("content") != "true" {
			t.Errorf("unexpected request URL %s", request.URL)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"jobs":[{"id":42,"title":"Backend Engineer","location":{"name":"Bengaluru, India"},"absolute_url":"https://boards.greenhouse.io/example/jobs/42","first_published":"2026-10-29T10:00:00Z","content":"<p>Java, Spring Boot and Kafka.</p>"}]}`))
	}))
	defer server.Close()

	client := NewClient(server.Client())
	client.httpClient.Transport = rewriteHostTransport{base: server.URL}
	source := domain.CareerSource{Provider: domain.ProviderGreenhouse, Company: "Example", BoardID: "example"}
	got, err := client.Fetch(context.Background(), source, domain.CandidateProfile{
		Skills: "Java, Spring Boot, Kafka", Title: "Backend Engineer", Location: "Bengaluru, India",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Score != 100 || len(got[0].Tags) != 3 {
		t.Fatalf("unexpected normalized jobs: %#v", got)
	}
}

type rewriteHostTransport struct {
	base string
}

func (transport rewriteHostTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	cloned := request.Clone(request.Context())
	target, err := request.URL.Parse(transport.base + request.URL.RequestURI())
	if err != nil {
		return nil, err
	}
	cloned.URL = target
	cloned.Host = target.Host
	return http.DefaultTransport.RoundTrip(cloned)
}

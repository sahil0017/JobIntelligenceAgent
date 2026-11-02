package lever

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"careerpilot/contracts/domain"
)

func TestClientFetchesAndMapsBoardListings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v0/postings/example" || request.URL.Query().Get("mode") != "json" {
			t.Errorf("unexpected request URL %s", request.URL)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`[{"id":"posting-1","text":"Backend Engineer","categories":{"location":"Remote - India"},"hostedUrl":"https://jobs.lever.co/example/posting-1","descriptionPlain":"Java, Spring Boot","createdAt":1793260800000}]`))
	}))
	defer server.Close()

	client := NewClient(server.Client())
	client.httpClient.Transport = rewriteHostTransport{base: server.URL}
	source := domain.CareerSource{Provider: domain.ProviderLever, Company: "Example", BoardID: "example"}
	got, err := client.Fetch(context.Background(), source, domain.CandidateProfile{Skills: "Java, Spring Boot"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].WorkMode != "Remote" || len(got[0].Tags) != 2 {
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

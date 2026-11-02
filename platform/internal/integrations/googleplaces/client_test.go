package googleplaces

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSearchMapsPlacesAndUsesAPIKeyHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/places:searchText" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("X-Goog-Api-Key") != "test-key" || request.Header.Get("X-Goog-FieldMask") != fieldMask {
			t.Fatalf("Google Places headers were not set: %#v", request.Header)
		}
		payload, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(payload), `"textQuery":"software companies in Bengaluru"`) ||
			!strings.Contains(string(payload), `"pageSize":2`) {
			t.Fatalf("unexpected query payload: %s", payload)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"places":[{"id":"place-1","displayName":{"text":"Example Labs"},"formattedAddress":"Bengaluru, India","websiteUri":"https://example.test","primaryTypeDisplayName":{"text":"Software company"},"googleMapsUri":"https://maps.google.test/place-1"}]}`)
	}))
	defer server.Close()
	client := NewClient(server.Client())
	client.endpoint = server.URL + "/v1/places:searchText"

	companies, err := client.Search(context.Background(), "test-key", " software companies in Bengaluru ", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(companies) != 1 || companies[0].ID != "place-1" ||
		companies[0].Name != "Example Labs" || companies[0].Website != "https://example.test" ||
		companies[0].Category != "Software company" {
		t.Fatalf("unexpected mapped company results: %#v", companies)
	}
}

func TestSearchRejectsInvalidInputsAndUpstreamErrors(t *testing.T) {
	client := NewClient(nil)
	if _, err := client.Search(context.Background(), "", "software companies", 5); err == nil {
		t.Fatal("expected missing API key to be rejected")
	}
	if _, err := client.Search(context.Background(), "test-key", "software companies", 21); err == nil {
		t.Fatal("expected oversized result limit to be rejected")
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	client = NewClient(server.Client())
	client.endpoint = server.URL
	if _, err := client.Search(context.Background(), "test-key", "software companies", 5); err == nil {
		t.Fatal("expected an upstream HTTP error")
	}
}

func TestSearchDoesNotForwardAPIKeyAcrossRedirects(t *testing.T) {
	var redirectTargetCalled atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		redirectTargetCalled.Store(true)
		if request.Header.Get("X-Goog-Api-Key") != "" {
			t.Error("API key was forwarded to a redirected host")
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Location", target.URL)
		writer.WriteHeader(http.StatusFound)
	}))
	defer redirect.Close()

	client := NewClient(redirect.Client())
	client.endpoint = redirect.URL
	if _, err := client.Search(context.Background(), "test-key", "software companies", 5); err == nil {
		t.Fatal("expected redirect response to be rejected")
	}
	if redirectTargetCalled.Load() {
		t.Fatal("Google Places client followed a redirect")
	}
}

package agents

import (
	"careerpilot/intelligence/workflow"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"careerpilot/contracts/domain"
	"careerpilot/intelligence/tools"
)

type testSources struct {
	sources []domain.CareerSource
}

func (sources testSources) List() ([]domain.CareerSource, error) {
	return sources.sources, nil
}

func TestDiscoveryAgentExecutesOnlyRegisteredReadOnlyTool(t *testing.T) {
	var modelCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/chat" {
			t.Errorf("unexpected Ollama endpoint: %s", request.URL.Path)
			http.NotFound(writer, request)
			return
		}
		call := modelCalls.Add(1)
		if call == 1 {
			var payload struct {
				Tools []struct {
					Function struct {
						Name string `json:"name"`
					} `json:"function"`
				} `json:"tools"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Error(err)
			} else if len(payload.Tools) != 1 || payload.Tools[0].Function.Name != "discover_jobs" {
				t.Errorf("model received an unexpected tool list: %#v", payload.Tools)
			}
		} else {
			_, _ = io.Copy(io.Discard, request.Body)
		}
		writer.Header().Set("Content-Type", "application/json")
		if call == 1 {
			_, _ = writer.Write([]byte(`{
				"model":"qwen2.5:7b",
				"message":{"role":"assistant","tool_calls":[{
					"function":{"name":"discover_jobs","arguments":{
						"skills":"Injected skills","title":"Injected role","location":"Injected place"
					}}
				}]},
				"done":true
			}`))
			return
		}
		_, _ = writer.Write([]byte(`{
			"model":"qwen2.5:7b",
			"message":{"role":"assistant","content":"I found one matching backend role."},
			"done":true
		}`))
	}))
	defer server.Close()

	var executions atomic.Int32
	registry := tools.NewRegistry()
	if err := registry.Register(tools.Tool{
		Name: "discover_jobs",
		Risk: tools.RiskReadOnly,
		Handler: func(_ context.Context, input json.RawMessage) (any, error) {
			executions.Add(1)
			var request workflow.DiscoveryRequest
			if err := json.Unmarshal(input, &request); err != nil {
				t.Fatal(err)
			}
			if len(request.Sources) != 1 || request.Profile.Title != "Backend Engineer" {
				t.Errorf("discovery tool received unexpected request: %#v", request)
			}
			return workflow.DiscoveryOutput{
				DiscoveryResult: domain.DiscoveryResult{
					Jobs: []domain.Job{{
						Company: "Example",
						Role:    "Backend Engineer",
						Score:   92,
						URL:     "https://jobs.example.test/1",
					}},
					CheckedSources: 1,
				},
			}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(tools.Tool{
		Name: "submit_application",
		Risk: tools.RiskExternalAction,
		Handler: func(context.Context, json.RawMessage) (any, error) {
			t.Error("the agent must not execute an external action")
			return nil, nil
		},
	}); err != nil {
		t.Fatal(err)
	}

	agent, err := NewDiscoveryAgent(
		server.URL,
		"qwen2.5:7b",
		testSources{sources: []domain.CareerSource{{Provider: domain.ProviderGreenhouse, Company: "Example", BoardID: "example"}}},
		registry,
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := agent.Run(context.Background(), DiscoveryRequest{
		Profile: domain.CandidateProfile{Skills: "Go", Title: "Backend Engineer", Location: "Remote"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Answer != "I found one matching backend role." {
		t.Fatalf("unexpected agent answer: %q", result.Answer)
	}
	if len(result.Jobs) != 1 || result.Jobs[0].Company != "Example" {
		t.Fatalf("unexpected discovery result: %#v", result.DiscoveryOutput)
	}
	if executions.Load() != 1 || modelCalls.Load() != 2 {
		t.Fatalf("tool/model calls = %d/%d, want 1/2", executions.Load(), modelCalls.Load())
	}
	if strings.Contains(result.Answer, "applied") {
		t.Fatalf("agent implied an application was submitted: %q", result.Answer)
	}
}

package agents

import (
	"careerpilot/intelligence/workflow"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"careerpilot/contracts/domain"
	"careerpilot/intelligence/tools"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/ollama"
)

type DiscoveryRequest struct {
	Profile domain.CandidateProfile `json:"profile"`
}

type DiscoveryResponse struct {
	Answer string `json:"answer"`
	workflow.DiscoveryOutput
}

type DiscoveryAgent struct {
	genkit  *genkit.Genkit
	model   ai.Model
	tool    *ai.ToolAction[toolInput, toolOutput]
	sources SourceLister
	tools   *tools.Registry
}

type SourceLister interface {
	List() ([]domain.CareerSource, error)
}

type toolInput struct {
	Skills   string `json:"skills"`
	Title    string `json:"title"`
	Location string `json:"location"`
}

type toolOutput struct {
	Jobs           []jobSummary         `json:"jobs"`
	SourceErrors   []domain.SourceError `json:"sourceErrors"`
	CheckedSources int                  `json:"checkedSources"`
}

type jobSummary struct {
	Company  string `json:"company"`
	Role     string `json:"role"`
	Location string `json:"location"`
	Score    int    `json:"score"`
	URL      string `json:"url"`
}

type runStateKey struct{}

type runState struct {
	mu        sync.Mutex
	remaining int
	profile   domain.CandidateProfile
	result    *workflow.DiscoveryOutput
}

func NewDiscoveryAgent(serverAddress, modelName string, sources SourceLister, registry *tools.Registry) (*DiscoveryAgent, error) {
	serverAddress = strings.TrimRight(strings.TrimSpace(serverAddress), "/")
	if err := validateOllamaURL(serverAddress); err != nil {
		return nil, err
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return nil, fmt.Errorf("Ollama model name is required")
	}
	if sources == nil || registry == nil {
		return nil, fmt.Errorf("discovery sources and tool registry are required")
	}

	plugin := &ollama.Ollama{ServerAddress: serverAddress, Timeout: 90}
	g := genkit.Init(context.Background(), genkit.WithPlugins(plugin))
	model := plugin.DefineModel(g, ollama.ModelDefinition{Name: modelName, Type: "chat"}, &ai.ModelOptions{
		Supports: &ai.ModelSupports{
			Multiturn:  true,
			SystemRole: true,
			Tools:      true,
			ToolChoice: true,
		},
	})
	agent := &DiscoveryAgent{genkit: g, model: model, sources: sources, tools: registry}
	agent.tool = genkit.DefineTool(g, "discover_jobs",
		"Search configured public Greenhouse and Lever boards for jobs matching the candidate. This is read-only.",
		agent.discoverJobs,
	)
	return agent, nil
}

func validateOllamaURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return fmt.Errorf("Ollama URL must be an absolute HTTP(S) URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("Ollama URL must not contain credentials, query parameters, or a fragment")
	}
	return nil
}

func (agent *DiscoveryAgent) Run(ctx context.Context, request DiscoveryRequest) (DiscoveryResponse, error) {
	if len(request.Profile.Skills) > 500 || len(request.Profile.Title) > 500 || len(request.Profile.Location) > 500 {
		return DiscoveryResponse{}, fmt.Errorf("profile fields must not exceed 500 characters")
	}
	state := &runState{remaining: 1, profile: request.Profile}
	ctx = context.WithValue(ctx, runStateKey{}, state)
	profileJSON, err := json.Marshal(request.Profile)
	if err != nil {
		return DiscoveryResponse{}, fmt.Errorf("encode candidate profile: %w", err)
	}
	response, err := genkit.Generate(ctx, agent.genkit,
		ai.WithModel(agent.model),
		ai.WithSystem("You are CareerPilot's job discovery assistant. Use the read-only discover_jobs tool exactly once. Summarize only the returned job data, do not invent listings, and never imply that you applied to a job."),
		ai.WithPrompt("Find current job matches for this candidate profile: %s", string(profileJSON)),
		ai.WithTools(agent.tool),
		ai.WithToolChoice("required"),
		ai.WithMaxTurns(1),
	)
	if err != nil {
		return DiscoveryResponse{}, fmt.Errorf("run job discovery agent: %w", err)
	}

	state.mu.Lock()
	result := state.result
	state.mu.Unlock()
	if result == nil {
		return DiscoveryResponse{}, fmt.Errorf("job discovery agent completed without running its required tool")
	}
	return DiscoveryResponse{Answer: response.Text(), DiscoveryOutput: *result}, nil
}

func (agent *DiscoveryAgent) discoverJobs(ctx *ai.ToolContext, _ toolInput) (toolOutput, error) {
	state, ok := ctx.Value(runStateKey{}).(*runState)
	if !ok {
		return toolOutput{}, fmt.Errorf("discovery tool can only run inside an agent request")
	}
	state.mu.Lock()
	if state.remaining == 0 {
		state.mu.Unlock()
		return toolOutput{}, fmt.Errorf("discovery tool call limit exceeded")
	}
	state.remaining--
	state.mu.Unlock()

	request := workflow.DiscoveryRequest{Profile: state.profile}
	sources, err := agent.sources.List()
	if err != nil {
		return toolOutput{}, fmt.Errorf("load configured job sources: %w", err)
	}
	request.Sources = sources
	payload, err := json.Marshal(request)
	if err != nil {
		return toolOutput{}, fmt.Errorf("encode discovery tool input: %w", err)
	}
	value, err := agent.tools.Execute(ctx, "discover_jobs", payload, false)
	if err != nil {
		return toolOutput{}, fmt.Errorf("execute read-only discovery tool: %w", err)
	}
	result, ok := value.(workflow.DiscoveryOutput)
	if !ok {
		return toolOutput{}, fmt.Errorf("discovery tool returned an unexpected result type")
	}

	state.mu.Lock()
	state.result = &result
	state.mu.Unlock()

	summaries := make([]jobSummary, 0, min(len(result.Jobs), 20))
	for _, job := range result.Jobs {
		if len(summaries) == 20 {
			break
		}
		summaries = append(summaries, jobSummary{
			Company:  job.Company,
			Role:     job.Role,
			Location: job.Location,
			Score:    job.Score,
			URL:      job.URL,
		})
	}
	return toolOutput{
		Jobs:           summaries,
		SourceErrors:   result.SourceErrors,
		CheckedSources: result.CheckedSources,
	}, nil
}

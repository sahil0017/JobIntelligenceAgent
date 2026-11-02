package domain

import "time"

// UserProfile is the signed-in user's editable career profile.
type UserProfile struct {
	Name       string `json:"name"`
	Title      string `json:"title"`
	Email      string `json:"email"`
	Location   string `json:"location"`
	Experience string `json:"experience"`
	Skills     string `json:"skills"`
	Roles      string `json:"roles"`
}

// Resume holds the parsed résumé text; the text is never serialized.
type Resume struct {
	Filename  string    `json:"filename"`
	Text      string    `json:"-"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// AgentConfig tunes the AI agents that run the area job search.
type AgentConfig struct {
	Model         string `json:"model"`
	OllamaURL     string `json:"ollamaUrl"`
	MaxRounds     int    `json:"maxRounds"`
	TargetMatches int    `json:"targetMatches"`
	MaxPages      int    `json:"maxPages"`
	MinScore      int    `json:"minScore"`
	LLMReview     bool   `json:"llmReview"`
}

func DefaultAgentConfig() AgentConfig {
	return AgentConfig{
		Model: "qwen2.5:7b", OllamaURL: "http://127.0.0.1:11434",
		MaxRounds: 4, TargetMatches: 10, MaxPages: 40, MinScore: 40, LLMReview: true,
	}
}

// SearchProfile is the subset of the profile sent to the public-web job search.
type SearchProfile struct {
	Title      string
	Skills     string
	Location   string
	ResumeText string
}

// SearchEvent reports agent progress; Stage is plan, search, verify or filter.
type SearchEvent struct {
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

// SearchOptions bounds the agent loop; zero values use the searcher's defaults.
type SearchOptions struct {
	MaxRounds     int
	TargetMatches int
	MaxPages      int
	MinScore      int
	OnEvent       func(SearchEvent)
}

type SearchJob struct {
	ID          string   `json:"id"`
	Company     string   `json:"company"`
	Role        string   `json:"role"`
	Location    string   `json:"location"`
	WorkMode    string   `json:"workMode"`
	Salary      string   `json:"salary"`
	Posted      string   `json:"posted"`
	Source      string   `json:"source"`
	URL         string   `json:"url"`
	Score       int      `json:"score"`
	Tags        []string `json:"tags"`
	Status      string   `json:"status"`
	Logo        string   `json:"logo"`
	LogoTone    string   `json:"logoTone"`
	SourceBoard string   `json:"sourceBoardId"`
}

type PageError struct {
	URL      string `json:"url"`
	Error    string `json:"error"`
	Category string `json:"category"`
}

type SearchResult struct {
	Jobs           []SearchJob `json:"jobs"`
	SourceErrors   []PageError `json:"sourceErrors"`
	CheckedSources int         `json:"checkedSources"`
	// FilteredLowMatch counts verified jobs dropped for a weak resume match.
	FilteredLowMatch int      `json:"filteredLowMatch"`
	Trace            []string `json:"trace"`
}

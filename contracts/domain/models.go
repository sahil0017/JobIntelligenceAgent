package domain

type Provider string

const (
	ProviderGreenhouse Provider = "greenhouse"
	ProviderLever      Provider = "lever"
)

type CareerSource struct {
	ID       string   `json:"id"`
	Provider Provider `json:"provider"`
	Company  string   `json:"company"`
	BoardID  string   `json:"boardId"`
}

type Company struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Address  string `json:"address"`
	Website  string `json:"website"`
	Category string `json:"category"`
	MapsURL  string `json:"mapsUrl"`
}

type CandidateProfile struct {
	Skills     string `json:"skills"`
	Title      string `json:"title"`
	Location   string `json:"location"`
	ResumeText string `json:"resumeText,omitempty"`
}

type Job struct {
	ID          string   `json:"id"`
	Company     string   `json:"company"`
	Role        string   `json:"role"`
	Location    string   `json:"location"`
	WorkMode    string   `json:"workMode"`
	Salary      string   `json:"salary"`
	Posted      string   `json:"posted"`
	Source      string   `json:"source"`
	SourceID    Provider `json:"sourceProvider,omitempty"`
	SourceBoard string   `json:"sourceBoardId,omitempty"`
	URL         string   `json:"url"`
	Score       int      `json:"score"`
	Tags        []string `json:"tags"`
	Status      string   `json:"status"`
	Logo        string   `json:"logo"`
	LogoTone    string   `json:"logoTone"`
}

type SourceError struct {
	Company string `json:"company"`
	Error   string `json:"error"`
}

type DiscoveryResult struct {
	Jobs           []Job         `json:"jobs"`
	SourceErrors   []SourceError `json:"sourceErrors"`
	CheckedSources int           `json:"checkedSources"`
}

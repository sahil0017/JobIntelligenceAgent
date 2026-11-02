package agents

import (
	"net/url"
	"strings"

	"careerpilot/contracts/domain"
)

type PreparationCheck struct {
	Name   string `json:"name"`
	Ready  bool   `json:"ready"`
	Detail string `json:"detail"`
}

type ApplicationPreparation struct {
	Job                  domain.Job         `json:"job"`
	ReadyToPrepare       bool               `json:"readyToPrepare"`
	SubmissionSupported  bool               `json:"submissionSupported"`
	RequiresUserApproval bool               `json:"requiresUserApproval"`
	Checks               []PreparationCheck `json:"checks"`
}

func PrepareApplication(profile domain.UserProfile, resume domain.Resume, job domain.Job) ApplicationPreparation {
	validJobURL := false
	parsedURL, err := url.ParseRequestURI(job.URL)
	if err == nil && parsedURL.Scheme == "https" && parsedURL.Host != "" && parsedURL.User == nil {
		validJobURL = true
	}
	checks := []PreparationCheck{
		{Name: "Contact details", Ready: strings.TrimSpace(profile.Name) != "" && strings.TrimSpace(profile.Email) != "", Detail: "Add your name and email address to the career profile."},
		{Name: "Career profile", Ready: strings.TrimSpace(profile.Title) != "" && strings.TrimSpace(profile.Skills) != "", Detail: "Add your current role and skills to the career profile."},
		{Name: "Parsed resume", Ready: strings.TrimSpace(resume.Text) != "", Detail: "Upload a text-based PDF or DOCX resume in the career profile."},
		{Name: "Verified job listing", Ready: strings.TrimSpace(job.Company) != "" && strings.TrimSpace(job.Role) != "" && validJobURL, Detail: "Use a live listing with an HTTPS application link."},
		{Name: "Portal application adapter", Ready: false, Detail: "Automated submission is not enabled. Open the employer's application page and submit it yourself."},
	}
	readyToPrepare := true
	for _, check := range checks[:len(checks)-1] {
		readyToPrepare = readyToPrepare && check.Ready
	}
	return ApplicationPreparation{
		Job:                  job,
		ReadyToPrepare:       readyToPrepare,
		SubmissionSupported:  false,
		RequiresUserApproval: true,
		Checks:               checks,
	}
}

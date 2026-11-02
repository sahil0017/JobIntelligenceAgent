package agents

import (
	"testing"

	"careerpilot/contracts/domain"
)

func TestPrepareApplicationRequiresProfileResumeAndManualApproval(t *testing.T) {
	job := domain.Job{
		Company: "Example Corp",
		Role:    "Backend Engineer",
		URL:     "https://jobs.example.test/1",
	}
	incomplete := PrepareApplication(domain.UserProfile{}, domain.Resume{}, job)
	if incomplete.ReadyToPrepare {
		t.Fatal("incomplete profile and resume should not be ready")
	}
	if incomplete.SubmissionSupported || !incomplete.RequiresUserApproval {
		t.Fatalf("unsafe application policy: %#v", incomplete)
	}

	complete := PrepareApplication(
		domain.UserProfile{Name: "User", Email: "user@example.test", Title: "Engineer", Skills: "Go"},
		domain.Resume{Text: "Experience with Go"},
		job,
	)
	if !complete.ReadyToPrepare {
		t.Fatalf("complete preparation inputs should be ready: %#v", complete.Checks)
	}
	if complete.SubmissionSupported || !complete.RequiresUserApproval {
		t.Fatalf("automated submission must stay disabled and approval required: %#v", complete)
	}
}

func TestPrepareApplicationRejectsUnsafeJobURLs(t *testing.T) {
	for _, jobURL := range []string{"http://jobs.example.test/1", "javascript:alert(1)", "https://user:pass@jobs.example.test/1"} {
		result := PrepareApplication(
			domain.UserProfile{Name: "User", Email: "user@example.test", Title: "Engineer", Skills: "Go"},
			domain.Resume{Text: "Resume"},
			domain.Job{Company: "Example", Role: "Engineer", URL: jobURL},
		)
		if result.ReadyToPrepare {
			t.Errorf("unsafe job URL %q was accepted", jobURL)
		}
	}
}

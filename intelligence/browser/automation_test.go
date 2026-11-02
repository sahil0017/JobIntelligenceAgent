package browser

import (
	"context"
	"testing"
	"time"
)

func TestSubmissionRequiresExplicitTimestampedApprovalBeforeStartingBrowser(t *testing.T) {
	automation := &testAutomation{}
	service, err := NewApplicationService(automation)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Submit(context.Background(), ApplicationForm{URL: "https://jobs.example/apply"}, Approval{})
	if err == nil {
		t.Fatal("expected submission to require approval")
	}
	if automation.started {
		t.Fatal("browser session started without approval")
	}
	_, err = service.Submit(context.Background(), ApplicationForm{URL: "https://jobs.example/apply"}, Approval{UserApproved: true, ApprovedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if !automation.started {
		t.Fatal("approved submission did not start browser session")
	}
}

type testAutomation struct {
	started bool
}

func (automation *testAutomation) NewSession(context.Context) (Session, error) {
	automation.started = true
	return testSession{}, nil
}

type testSession struct{}

func (testSession) Open(context.Context, string) error { return nil }
func (testSession) InspectApplicationForm(context.Context) (ApplicationForm, error) {
	return ApplicationForm{Fields: map[string]string{"full_name": ""}}, nil
}
func (testSession) FillForm(context.Context, ApplicationForm) error   { return nil }
func (testSession) UploadResume(context.Context, string) error        { return nil }
func (testSession) CaptureScreenshot(context.Context) (string, error) { return "", nil }
func (testSession) Submit(context.Context) (SubmissionEvidence, error) {
	return SubmissionEvidence{}, nil
}
func (testSession) Close() error { return nil }

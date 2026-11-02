package browser

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

type ApplicationForm struct {
	URL    string
	Fields map[string]string
	Resume string
}

type Approval struct {
	UserApproved bool
	ApprovedAt   time.Time
}

type SubmissionEvidence struct {
	ConfirmationURL string
	ApplicationID   string
	ScreenshotPath  string
	SubmittedAt     time.Time
}

type Session interface {
	Open(context.Context, string) error
	InspectApplicationForm(context.Context) (ApplicationForm, error)
	FillForm(context.Context, ApplicationForm) error
	UploadResume(context.Context, string) error
	CaptureScreenshot(context.Context) (string, error)
	Submit(context.Context) (SubmissionEvidence, error)
	Close() error
}

type Automation interface {
	NewSession(context.Context) (Session, error)
}

type ApplicationService struct {
	automation Automation
}

func NewApplicationService(automation Automation) (*ApplicationService, error) {
	if automation == nil {
		return nil, fmt.Errorf("browser automation implementation is required")
	}
	return &ApplicationService{automation: automation}, nil
}

func (service *ApplicationService) Submit(ctx context.Context, form ApplicationForm, approval Approval) (SubmissionEvidence, error) {
	if !approval.UserApproved || approval.ApprovedAt.IsZero() {
		return SubmissionEvidence{}, fmt.Errorf("application submission requires explicit, timestamped user approval")
	}
	applicationURL, err := url.ParseRequestURI(form.URL)
	if err != nil || applicationURL.Scheme != "https" || applicationURL.Host == "" {
		return SubmissionEvidence{}, fmt.Errorf("application URL must be an absolute HTTPS URL")
	}
	session, err := service.automation.NewSession(ctx)
	if err != nil {
		return SubmissionEvidence{}, fmt.Errorf("start browser session: %w", err)
	}
	defer session.Close()
	if err := session.Open(ctx, form.URL); err != nil {
		return SubmissionEvidence{}, fmt.Errorf("open application URL: %w", err)
	}
	observedForm, err := session.InspectApplicationForm(ctx)
	if err != nil {
		return SubmissionEvidence{}, fmt.Errorf("inspect application form: %w", err)
	}
	observedForm.URL = form.URL
	observedForm.Fields = form.Fields
	observedForm.Resume = form.Resume
	if err := session.FillForm(ctx, observedForm); err != nil {
		return SubmissionEvidence{}, fmt.Errorf("fill application form: %w", err)
	}
	if form.Resume != "" {
		if err := session.UploadResume(ctx, form.Resume); err != nil {
			return SubmissionEvidence{}, fmt.Errorf("upload resume: %w", err)
		}
	}
	screenshotPath, err := session.CaptureScreenshot(ctx)
	if err != nil {
		return SubmissionEvidence{}, fmt.Errorf("capture application review screenshot: %w", err)
	}
	evidence, err := session.Submit(ctx)
	if err != nil {
		return SubmissionEvidence{}, fmt.Errorf("submit approved application: %w", err)
	}
	if evidence.ScreenshotPath == "" {
		evidence.ScreenshotPath = screenshotPath
	}
	return evidence, nil
}

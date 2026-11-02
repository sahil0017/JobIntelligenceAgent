package monitor

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"careerpilot/contracts/domain"
	"careerpilot/platform/internal/integrations/jobs"
	"careerpilot/platform/internal/storage"
)

type testProvider struct {
	listings []domain.Job
	err      error
	calls    int
}

func (provider *testProvider) Name() domain.Provider {
	return domain.ProviderGreenhouse
}

func (provider *testProvider) Fetch(_ context.Context, _ domain.CareerSource, _ domain.CandidateProfile) ([]domain.Job, error) {
	provider.calls++
	return provider.listings, provider.err
}

type testNotifier struct {
	connected bool
	address   string
	messages  int
	recipient string
	subject   string
	body      string
}

func (notifier *testNotifier) Connected() bool {
	return notifier.connected
}

func (notifier *testNotifier) EmailAddress(context.Context) (string, error) {
	return notifier.address, nil
}

func (notifier *testNotifier) SendMessage(_ context.Context, recipient, subject, body string) (string, error) {
	notifier.messages++
	notifier.recipient = recipient
	notifier.subject = subject
	notifier.body = body
	return "message-id", nil
}

func TestMonitorRequiresThreeSuccessfulMissesAndSendsOneNotice(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "careerpilot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	applied := domain.Job{
		ID: "greenhouse:example:42", Company: "Example", Role: "Backend Engineer",
		URL: "https://boards.greenhouse.io/example/jobs/42", Status: "Applied",
		SourceID: domain.ProviderGreenhouse, SourceBoard: "example",
	}
	if err := store.SaveTrackedJob(applied, string(applied.SourceID), applied.SourceBoard); err != nil {
		t.Fatal(err)
	}
	provider := &testProvider{}
	registry, err := jobs.NewRegistry(provider)
	if err != nil {
		t.Fatal(err)
	}
	notifier := &testNotifier{connected: true, address: "candidate@example.test"}
	service := New(store, registry, notifier)
	fixedTime := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixedTime }

	provider.err = errors.New("temporary upstream failure")
	if err := service.ScanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	tracked, err := store.ListTrackedJobs()
	if err != nil || len(tracked) != 1 || tracked[0].MissingSuccessfulScans != 0 {
		t.Fatalf("failed fetch counted as a missing scan: %#v, %v", tracked, err)
	}
	provider.err = nil
	for scan := 1; scan <= 2; scan++ {
		if err := service.ScanOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
		if notifier.messages != 0 {
			t.Fatalf("closure notice sent after only %d successful misses", scan)
		}
	}
	if err := service.ScanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if notifier.messages != 1 || notifier.recipient != notifier.address ||
		notifier.subject != "Review closed applied job: Backend Engineer" {
		t.Fatalf("closure notice was not sent correctly: %#v", notifier)
	}
	tracked, err = store.ListTrackedJobs()
	if err != nil || len(tracked) != 1 || !tracked[0].ClosurePending || !tracked[0].NotificationSent {
		t.Fatalf("job was not left pending for confirmation: %#v, %v", tracked, err)
	}
	if err := service.ScanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if notifier.messages != 1 {
		t.Fatalf("closure notification was sent more than once: %d", notifier.messages)
	}
}

func TestMonitorDoesNotCloseJobThatReturns(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "careerpilot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	applied := domain.Job{
		ID: "greenhouse:example:42", Company: "Example", Role: "Backend Engineer",
		URL: "https://boards.greenhouse.io/example/jobs/42", Status: "Applied",
		SourceID: domain.ProviderGreenhouse, SourceBoard: "example",
	}
	if err := store.SaveTrackedJob(applied, string(applied.SourceID), applied.SourceBoard); err != nil {
		t.Fatal(err)
	}
	provider := &testProvider{}
	registry, err := jobs.NewRegistry(provider)
	if err != nil {
		t.Fatal(err)
	}
	service := New(store, registry, &testNotifier{connected: true, address: "candidate@example.test"})
	for range 2 {
		if err := service.ScanOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	provider.listings = []domain.Job{applied}
	if err := service.ScanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	tracked, err := store.ListTrackedJobs()
	if err != nil || len(tracked) != 1 || tracked[0].ClosurePending || tracked[0].MissingSuccessfulScans != 0 {
		t.Fatalf("returning listing was treated as closed: %#v, %v", tracked, err)
	}
}

func TestMonitorRetriesClosureNoticeWhenGmailConnectsLater(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "careerpilot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	applied := domain.Job{
		ID: "greenhouse:example:42", Company: "Example", Role: "Backend Engineer",
		URL: "https://boards.greenhouse.io/example/jobs/42", Status: "Applied",
		SourceID: domain.ProviderGreenhouse, SourceBoard: "example",
	}
	if err := store.SaveTrackedJob(applied, string(applied.SourceID), applied.SourceBoard); err != nil {
		t.Fatal(err)
	}
	provider := &testProvider{}
	registry, err := jobs.NewRegistry(provider)
	if err != nil {
		t.Fatal(err)
	}
	notifier := &testNotifier{address: "candidate@example.test"}
	service := New(store, registry, notifier)
	for range 3 {
		if err := service.ScanOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	tracked, err := store.ListTrackedJobs()
	if err != nil || len(tracked) != 1 || !tracked[0].ClosurePending || tracked[0].NotificationAttempted {
		t.Fatalf("closure should remain pending until Gmail is available: %#v, %v", tracked, err)
	}

	notifier.connected = true
	if err := service.ScanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	tracked, err = store.ListTrackedJobs()
	if err != nil || len(tracked) != 1 || !tracked[0].NotificationSent || notifier.messages != 1 {
		t.Fatalf("closure notice was not sent after Gmail connected: %#v, notifier=%#v, err=%v", tracked, notifier, err)
	}
}

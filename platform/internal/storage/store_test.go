package storage

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"careerpilot/contracts/domain"
)

func TestStorePersistsUserConfigurationAndEncryptsSensitiveValues(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "careerpilot.db")
	store, err := Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	profile := Profile{
		Name:     "Example User",
		Email:    "user@example.test",
		Title:    "Backend Engineer",
		Skills:   "Go, SQLite",
		Location: "Remote",
	}
	if err := store.SaveProfile(profile); err != nil {
		t.Fatal(err)
	}
	savedProfile, err := store.LoadProfile()
	if err != nil || savedProfile != profile {
		t.Fatalf("profile round-trip failed: %#v, %v", savedProfile, err)
	}

	config := IntegrationConfig{
		Provider:     "gmail",
		ClientID:     "client-id",
		ClientSecret: "private-client-secret",
		RedirectURL:  "http://localhost:8787/callback",
	}
	if err := store.SaveIntegration(config); err != nil {
		t.Fatal(err)
	}
	savedConfig, err := store.LoadIntegration("gmail")
	if err != nil || savedConfig.ClientSecret != config.ClientSecret || !savedConfig.SecretConfigured {
		t.Fatalf("integration secret round-trip failed: %#v, %v", savedConfig, err)
	}
	withoutSecret := config
	withoutSecret.ClientSecret = ""
	withoutSecret.ClientID = "updated-client-id"
	if err := store.SaveIntegration(withoutSecret); err != nil {
		t.Fatal(err)
	}
	savedConfig, err = store.LoadIntegration("gmail")
	if err != nil || savedConfig.ClientID != "updated-client-id" || savedConfig.ClientSecret != config.ClientSecret {
		t.Fatalf("omitting secret should preserve existing secret: %#v, %v", savedConfig, err)
	}

	resume := Resume{Filename: "resume.pdf", Text: "Private candidate resume text"}
	if err := store.SaveResume(resume); err != nil {
		t.Fatal(err)
	}
	savedResume, err := store.LoadResume()
	if err != nil || savedResume.Filename != resume.Filename || savedResume.Text != resume.Text {
		t.Fatalf("resume round-trip failed: %#v, %v", savedResume, err)
	}

	databaseContents, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, plaintext := range [][]byte{[]byte(config.ClientSecret), []byte(resume.Text)} {
		if bytes.Contains(databaseContents, plaintext) {
			t.Fatalf("database contains sensitive plaintext %q", plaintext)
		}
	}
	info, err := os.Stat(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("database permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestStoreCareerSourcesAndPortals(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "careerpilot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	sources, err := store.ListSources()
	if err != nil || len(sources) != 1 || sources[0].BoardID != "stripe" {
		t.Fatalf("default source missing: %#v, %v", sources, err)
	}
	source := domain.CareerSource{
		ID:       "lever-example",
		Provider: domain.ProviderLever,
		Company:  "Example",
		BoardID:  "example",
	}
	if err := store.AddSource(source); err != nil {
		t.Fatal(err)
	}
	if err := store.RemoveSource(source.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.RemoveSource(source.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing source error = %v, want ErrNotFound", err)
	}

	portal := PortalConfig{
		ID: "naukri-id", Name: "Naukri", Provider: "naukri", URL: "https://www.naukri.com/",
		Status: "adapter_unavailable", CreatedAt: "2030-01-01T00:00:00Z",
	}
	if err := store.AddPortal(portal); err != nil {
		t.Fatal(err)
	}
	portals, err := store.ListPortals()
	if err != nil || len(portals) != 1 || portals[0].Provider != portal.Provider {
		t.Fatalf("portal round-trip failed: %#v, %v", portals, err)
	}
	if err := store.RemovePortal(portal.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.RemovePortal(portal.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing portal error = %v, want ErrNotFound", err)
	}
}

func TestStoreOpensWithStableEncryptionKey(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "careerpilot.db")
	store, err := Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.SaveIntegration(IntegrationConfig{
		Provider: "linkedin", ClientID: "client", ClientSecret: "secret", RedirectURL: "http://localhost/callback",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	config, err := store.LoadIntegration("linkedin")
	if err != nil || config.ClientSecret != "secret" {
		t.Fatalf("could not decrypt after reopening: %#v, %v", config, err)
	}
	keyInfo, err := os.Stat(databasePath + ".key")
	if err != nil {
		t.Fatal(err)
	}
	if keyInfo.Mode().Perm() != 0o600 {
		t.Fatalf("key permissions = %o, want 600", keyInfo.Mode().Perm())
	}
}

func TestTrackedJobsDetectClosureOnlyAfterThreeSuccessfulMisses(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "careerpilot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	job := domain.Job{
		ID: "greenhouse:example:42", Company: "Example", Role: "Backend Engineer",
		URL: "https://boards.greenhouse.io/example/jobs/42", Status: "Applied",
		SourceID: domain.ProviderGreenhouse, SourceBoard: "example",
	}
	if err := store.SaveTrackedJob(job, string(job.SourceID), job.SourceBoard); err != nil {
		t.Fatal(err)
	}
	tracked, err := store.ListTrackedJobs()
	if err != nil || len(tracked) != 1 || tracked[0].Job.ID != job.ID || tracked[0].SourceBoardID != "example" {
		t.Fatalf("applied job was not persisted: %#v, %v", tracked, err)
	}
	for scan := 1; scan < 3; scan++ {
		pending, err := store.RecordTrackedJobCheck(job.ID, nil, time.Now())
		if err != nil || pending {
			t.Fatalf("job became pending after scan %d: pending=%v, error=%v", scan, pending, err)
		}
	}
	pending, err := store.RecordTrackedJobCheck(job.ID, nil, time.Now())
	if err != nil || !pending {
		t.Fatalf("third successful miss did not create pending deletion: pending=%v, error=%v", pending, err)
	}
	tracked, err = store.ListTrackedJobs()
	if err != nil || len(tracked) != 1 || !tracked[0].ClosurePending || tracked[0].MissingSuccessfulScans != 3 {
		t.Fatalf("closure status was not persisted: %#v, %v", tracked, err)
	}
	claimed, err := store.ClaimClosureNotification(job.ID)
	if err != nil || !claimed {
		t.Fatalf("closure notification was not claimed: %v, %v", claimed, err)
	}
	claimed, err = store.ClaimClosureNotification(job.ID)
	if err != nil || claimed {
		t.Fatalf("closure notification was claimed twice: %v, %v", claimed, err)
	}
	if err := store.MarkClosureNotificationSent(job.ID); err != nil {
		t.Fatal(err)
	}
	events, err := store.ListTrackedJobEvents(job.ID)
	if err != nil || len(events) != 2 || events[0].EventType != "JOB_CLOSED" {
		t.Fatalf("unexpected applied job events: %#v, %v", events, err)
	}
	if err := store.ConfirmDeleteTrackedJob(job.ID); err != nil {
		t.Fatalf("confirmed closed job could not be deleted: %v", err)
	}
	tracked, err = store.ListTrackedJobs()
	if err != nil || len(tracked) != 0 {
		t.Fatalf("confirmed deletion left tracked job behind: %#v, %v", tracked, err)
	}
	events, err = store.ListTrackedJobEvents(job.ID)
	if err != nil || len(events) != 0 {
		t.Fatalf("job event history was not deleted with job: %#v, %v", events, err)
	}
}

func TestTrackedJobReappearingCancelsPendingDeletion(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "careerpilot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	job := domain.Job{
		ID: "lever:example:posting-1", Company: "Example", Role: "Engineer",
		URL: "https://jobs.lever.co/example/posting-1", Status: "Applied",
	}
	if err := store.SaveTrackedJob(job, string(domain.ProviderLever), "example"); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if _, err := store.RecordTrackedJobCheck(job.ID, nil, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	foundAgain := job
	foundAgain.Location = "Remote"
	if _, err := store.RecordTrackedJobCheck(job.ID, &foundAgain, time.Now()); err != nil {
		t.Fatal(err)
	}
	tracked, err := store.ListTrackedJobs()
	if err != nil || len(tracked) != 1 || tracked[0].ClosurePending || tracked[0].MissingSuccessfulScans != 0 ||
		tracked[0].Job.Location != "Remote" {
		t.Fatalf("reappearing job was not restored: %#v, %v", tracked, err)
	}
}

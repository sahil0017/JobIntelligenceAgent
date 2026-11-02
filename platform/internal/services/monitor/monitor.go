package monitor

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"careerpilot/contracts/domain"
	"careerpilot/platform/internal/integrations/jobs"
	"careerpilot/platform/internal/storage"
)

const ScanInterval = 4 * time.Hour

type Store interface {
	ListTrackedJobs() ([]storage.TrackedJob, error)
	LoadProfile() (storage.Profile, error)
	RecordTrackedJobCheck(string, *domain.Job, time.Time) (bool, error)
	ClaimClosureNotification(string) (bool, error)
	MarkClosureNotificationSent(string) error
}

type GmailNotifier interface {
	Connected() bool
	EmailAddress(context.Context) (string, error)
	SendMessage(context.Context, string, string, string) (string, error)
}

type Service struct {
	store     Store
	providers *jobs.Registry
	gmail     GmailNotifier
	now       func() time.Time
}

func New(store Store, providers *jobs.Registry, gmail GmailNotifier) *Service {
	return &Service{store: store, providers: providers, gmail: gmail, now: time.Now}
}

func (service *Service) Run(ctx context.Context) {
	if err := service.ScanOnce(ctx); err != nil {
		log.Printf("initial applied-job monitoring scan failed: %v", err)
	}
	ticker := time.NewTicker(ScanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := service.ScanOnce(ctx); err != nil {
				log.Printf("applied-job monitoring scan failed: %v", err)
			}
		}
	}
}

func (service *Service) ScanOnce(ctx context.Context) error {
	trackedJobs, err := service.store.ListTrackedJobs()
	if err != nil {
		return fmt.Errorf("list applied jobs for monitoring: %w", err)
	}
	profile, err := service.store.LoadProfile()
	if err != nil {
		return fmt.Errorf("load candidate profile for monitoring: %w", err)
	}

	type board struct {
		provider domain.Provider
		boardID  string
		company  string
		jobs     []storage.TrackedJob
	}
	boards := make(map[string]*board)
	for _, tracked := range trackedJobs {
		if tracked.SourceProvider == "" || tracked.SourceBoardID == "" {
			continue
		}
		provider := domain.Provider(tracked.SourceProvider)
		if provider != domain.ProviderGreenhouse && provider != domain.ProviderLever {
			continue
		}
		key := string(provider) + ":" + tracked.SourceBoardID
		entry := boards[key]
		if entry == nil {
			entry = &board{provider: provider, boardID: tracked.SourceBoardID, company: tracked.Job.Company}
			boards[key] = entry
		}
		entry.jobs = append(entry.jobs, tracked)
	}

	for _, entry := range boards {
		provider, ok := service.providers.Get(entry.provider)
		if !ok {
			log.Printf("skip applied-job scan for unregistered provider %q", entry.provider)
			continue
		}
		source, err := jobs.ValidateSource(domain.CareerSource{
			Provider: entry.provider,
			Company:  entry.company,
			BoardID:  entry.boardID,
		})
		if err != nil {
			log.Printf("skip applied-job scan for invalid source %q: %v", entry.boardID, err)
			continue
		}
		listings, err := provider.Fetch(ctx, source, domain.CandidateProfile{
			Skills:   profile.Skills,
			Title:    profile.Title,
			Location: profile.Location,
		})
		if err != nil {
			log.Printf("applied-job scan for %s (%s) failed; closure state unchanged: %v", entry.company, entry.provider, err)
			continue
		}
		byID := make(map[string]domain.Job, len(listings))
		for _, listing := range listings {
			byID[listing.ID] = listing
		}
		for _, tracked := range entry.jobs {
			listing, found := byID[tracked.Job.ID]
			var updated *domain.Job
			if found {
				updated = &listing
			}
			newlyPending, err := service.store.RecordTrackedJobCheck(tracked.Job.ID, updated, service.now())
			if err != nil {
				if !errors.Is(err, storage.ErrNotFound) {
					log.Printf("record applied-job scan result for %q: %v", tracked.Job.ID, err)
				}
				continue
			}
			if newlyPending || (tracked.ClosurePending && !tracked.NotificationAttempted) {
				service.notifyClosure(ctx, tracked)
			}
		}
	}
	return nil
}

func (service *Service) notifyClosure(ctx context.Context, tracked storage.TrackedJob) {
	if service.gmail == nil || !service.gmail.Connected() {
		return
	}
	email, err := service.gmail.EmailAddress(ctx)
	if err != nil {
		log.Printf("get Gmail address for closure notice: %v", err)
		return
	}
	claimed, err := service.store.ClaimClosureNotification(tracked.Job.ID)
	if err != nil {
		log.Printf("claim closure notice for %q: %v", tracked.Job.ID, err)
		return
	}
	if !claimed {
		return
	}
	subject := fmt.Sprintf("Review closed applied job: %s", tracked.Job.Role)
	body := fmt.Sprintf(
		"CareerPilot could not find this applied job in three successful career-feed checks.\n\nCompany: %s\nRole: %s\nJob listing: %s\n\nThe job has not been deleted. Open CareerPilot and review the pending closure in Applications. Confirm deletion there, or choose Keep to retain it.",
		tracked.Job.Company, tracked.Job.Role, tracked.Job.URL,
	)
	if _, err := service.gmail.SendMessage(ctx, email, subject, body); err != nil {
		log.Printf("send applied-job closure notice for %q: %v", tracked.Job.ID, err)
		return
	}
	if err := service.store.MarkClosureNotificationSent(tracked.Job.ID); err != nil {
		log.Printf("mark applied-job closure notice sent for %q: %v", tracked.Job.ID, err)
	}
}

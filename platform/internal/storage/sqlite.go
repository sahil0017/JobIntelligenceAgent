package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"careerpilot/contracts/domain"
	_ "modernc.org/sqlite"
)

type Store struct {
	db  *sql.DB
	key []byte
}

var ErrNotFound = errors.New("configuration record not found")

type Profile = domain.UserProfile

type IntegrationConfig struct {
	Provider         string `json:"provider"`
	ClientID         string `json:"clientId"`
	ClientSecret     string `json:"-"`
	RedirectURL      string `json:"redirectUrl"`
	SecretConfigured bool   `json:"secretConfigured"`
}

type PortalConfig struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Provider  string `json:"provider"`
	URL       string `json:"url"`
	Notes     string `json:"notes"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

type Resume = domain.Resume

type TrackedJob struct {
	Job                    domain.Job `json:"job"`
	SourceProvider         string     `json:"sourceProvider"`
	SourceBoardID          string     `json:"sourceBoardId"`
	AppliedAt              time.Time  `json:"appliedAt"`
	LastCheckedAt          *time.Time `json:"lastCheckedAt,omitempty"`
	MissingSuccessfulScans int        `json:"missingSuccessfulScans"`
	ClosurePending         bool       `json:"closurePending"`
	NotificationAttempted  bool       `json:"notificationAttempted"`
	NotificationSent       bool       `json:"notificationSent"`
}

type JobEvent struct {
	ID         int64     `json:"id"`
	JobID      string    `json:"jobId"`
	EventType  string    `json:"eventType"`
	Detail     string    `json:"detail"`
	OccurredAt time.Time `json:"occurredAt"`
}

func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("database path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	key, err := loadOrCreateKey(path + ".key")
	if err != nil {
		return nil, err
	}
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	database.SetMaxOpenConns(1)
	store := &Store{db: database, key: key}
	if err := database.Ping(); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("connect to SQLite database: %w", err)
	}
	if err := store.migrate(); err != nil {
		_ = database.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("protect SQLite database file: %w", err)
	}
	return store, nil
}

func (store *Store) Close() error {
	if err := store.db.Close(); err != nil {
		return fmt.Errorf("close SQLite database: %w", err)
	}
	return nil
}

func (store *Store) migrate() error {
	statements := []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA foreign_keys = ON`,
		`CREATE TABLE IF NOT EXISTS job_sources (
			id TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			company TEXT NOT NULL,
			board_id TEXT NOT NULL,
			UNIQUE(provider, board_id)
		)`,
		`CREATE TABLE IF NOT EXISTS user_profiles (
			user_id TEXT PRIMARY KEY,
			payload TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS integration_configs (
			provider TEXT PRIMARY KEY,
			client_id TEXT NOT NULL,
			client_secret BLOB NOT NULL,
			redirect_url TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS portal_configs (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			provider TEXT NOT NULL,
			url TEXT NOT NULL,
			notes TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS resumes (
			user_id TEXT PRIMARY KEY,
			filename TEXT NOT NULL,
			parsed_text TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS tracked_jobs (
			job_id TEXT PRIMARY KEY,
			payload TEXT NOT NULL,
			source_provider TEXT NOT NULL,
			source_board_id TEXT NOT NULL,
			applied_at TEXT NOT NULL,
			last_checked_at TEXT NOT NULL DEFAULT '',
			missing_successful_scans INTEGER NOT NULL DEFAULT 0,
			closure_pending INTEGER NOT NULL DEFAULT 0,
			notification_attempted INTEGER NOT NULL DEFAULT 0,
			notification_sent INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS tracked_job_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id TEXT NOT NULL REFERENCES tracked_jobs(job_id) ON DELETE CASCADE,
			event_type TEXT NOT NULL,
			detail TEXT NOT NULL,
			occurred_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS agent_config (
			id TEXT PRIMARY KEY,
			payload TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`INSERT OR IGNORE INTO job_sources (id, provider, company, board_id)
			VALUES ('stripe-greenhouse', 'greenhouse', 'Stripe', 'stripe')`,
	}
	for _, statement := range statements {
		if _, err := store.db.Exec(statement); err != nil {
			return fmt.Errorf("initialize SQLite schema: %w", err)
		}
	}
	return nil
}

func (store *Store) SaveTrackedJob(job domain.Job, provider, boardID string) error {
	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("encode applied job: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	transaction, err := store.db.Begin()
	if err != nil {
		return fmt.Errorf("begin applied job save: %w", err)
	}
	defer transaction.Rollback()
	result, err := transaction.Exec(
		`INSERT INTO tracked_jobs (job_id, payload, source_provider, source_board_id, applied_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(job_id) DO UPDATE SET
		 payload = excluded.payload,
		 source_provider = excluded.source_provider,
		 source_board_id = excluded.source_board_id`,
		job.ID, string(payload), provider, boardID, now,
	)
	if err != nil {
		return fmt.Errorf("save applied job: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check applied job save: %w", err)
	}
	if rows > 0 {
		var count int
		if err := transaction.QueryRow(`SELECT COUNT(*) FROM tracked_job_events WHERE job_id = ?`, job.ID).Scan(&count); err != nil {
			return fmt.Errorf("check applied job event history: %w", err)
		}
		if count == 0 {
			if _, err := transaction.Exec(
				`INSERT INTO tracked_job_events (job_id, event_type, detail, occurred_at) VALUES (?, ?, ?, ?)`,
				job.ID, "JOB_APPLIED", "Job added to application follow-up", now,
			); err != nil {
				return fmt.Errorf("record applied job event: %w", err)
			}
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit applied job save: %w", err)
	}
	return nil
}

func (store *Store) ListTrackedJobs() ([]TrackedJob, error) {
	rows, err := store.db.Query(
		`SELECT payload, source_provider, source_board_id, applied_at, last_checked_at,
		 missing_successful_scans, closure_pending, notification_attempted, notification_sent
		 FROM tracked_jobs ORDER BY applied_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query applied jobs: %w", err)
	}
	defer rows.Close()
	jobs := make([]TrackedJob, 0)
	for rows.Next() {
		var tracked TrackedJob
		var payload, appliedAt, checkedAt string
		var pending, attempted, sent int
		if err := rows.Scan(
			&payload, &tracked.SourceProvider, &tracked.SourceBoardID, &appliedAt, &checkedAt,
			&tracked.MissingSuccessfulScans, &pending, &attempted, &sent,
		); err != nil {
			return nil, fmt.Errorf("read applied job: %w", err)
		}
		if err := json.Unmarshal([]byte(payload), &tracked.Job); err != nil {
			return nil, fmt.Errorf("decode applied job: %w", err)
		}
		tracked.AppliedAt, err = time.Parse(time.RFC3339Nano, appliedAt)
		if err != nil {
			return nil, fmt.Errorf("parse applied job timestamp: %w", err)
		}
		if checkedAt != "" {
			parsed, err := time.Parse(time.RFC3339Nano, checkedAt)
			if err != nil {
				return nil, fmt.Errorf("parse applied job check timestamp: %w", err)
			}
			tracked.LastCheckedAt = &parsed
		}
		tracked.ClosurePending = pending != 0
		tracked.NotificationAttempted = attempted != 0
		tracked.NotificationSent = sent != 0
		jobs = append(jobs, tracked)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applied jobs: %w", err)
	}
	return jobs, nil
}

func (store *Store) UpdateTrackedJob(job domain.Job) error {
	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("encode applied job update: %w", err)
	}
	result, err := store.db.Exec(`UPDATE tracked_jobs SET payload = ? WHERE job_id = ?`, string(payload), job.ID)
	if err != nil {
		return fmt.Errorf("update applied job: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check applied job update: %w", err)
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (store *Store) RecordTrackedJobCheck(jobID string, found *domain.Job, checkedAt time.Time) (bool, error) {
	transaction, err := store.db.Begin()
	if err != nil {
		return false, fmt.Errorf("begin applied job check: %w", err)
	}
	defer transaction.Rollback()
	var payload string
	var missing, pending int
	if err := transaction.QueryRow(
		`SELECT payload, missing_successful_scans, closure_pending FROM tracked_jobs WHERE job_id = ?`,
		jobID,
	).Scan(&payload, &missing, &pending); errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	} else if err != nil {
		return false, fmt.Errorf("load applied job for scan: %w", err)
	}
	now := checkedAt.UTC().Format(time.RFC3339Nano)
	newlyPending := false
	eventType, detail := "", ""
	if found != nil {
		var previous domain.Job
		if err := json.Unmarshal([]byte(payload), &previous); err != nil {
			return false, fmt.Errorf("decode applied job before scan: %w", err)
		}
		found.Status = previous.Status
		updatedPayload, err := json.Marshal(found)
		if err != nil {
			return false, fmt.Errorf("encode refreshed applied job: %w", err)
		}
		if previous.Role != found.Role || previous.Location != found.Location ||
			previous.Salary != found.Salary || previous.URL != found.URL {
			eventType, detail = "JOB_UPDATED", "Listing details changed"
		}
		if pending != 0 && eventType == "" {
			eventType, detail = "JOB_REOPENED", "Listing returned to the public feed"
		}
		if _, err := transaction.Exec(
			`UPDATE tracked_jobs SET payload = ?, missing_successful_scans = 0, closure_pending = 0,
			 notification_attempted = 0, notification_sent = 0, last_checked_at = ? WHERE job_id = ?`,
			string(updatedPayload), now, jobID,
		); err != nil {
			return false, fmt.Errorf("update found applied job: %w", err)
		}
	} else {
		missing++
		if pending == 0 && missing >= 3 {
			pending = 1
			newlyPending = true
			eventType, detail = "JOB_CLOSED", "Missing from three successful career-feed checks"
		}
		if _, err := transaction.Exec(
			`UPDATE tracked_jobs SET missing_successful_scans = ?, closure_pending = ?, last_checked_at = ? WHERE job_id = ?`,
			missing, pending, now, jobID,
		); err != nil {
			return false, fmt.Errorf("update missing applied job: %w", err)
		}
	}
	if eventType != "" {
		if _, err := transaction.Exec(
			`INSERT INTO tracked_job_events (job_id, event_type, detail, occurred_at) VALUES (?, ?, ?, ?)`,
			jobID, eventType, detail, now,
		); err != nil {
			return false, fmt.Errorf("record applied job change event: %w", err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return false, fmt.Errorf("commit applied job scan: %w", err)
	}
	return newlyPending, nil
}

func (store *Store) ClaimClosureNotification(jobID string) (bool, error) {
	result, err := store.db.Exec(
		`UPDATE tracked_jobs SET notification_attempted = 1
		 WHERE job_id = ? AND closure_pending = 1 AND notification_attempted = 0`,
		jobID,
	)
	if err != nil {
		return false, fmt.Errorf("claim job closure notification: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check job closure notification claim: %w", err)
	}
	return count == 1, nil
}

func (store *Store) MarkClosureNotificationSent(jobID string) error {
	if _, err := store.db.Exec(`UPDATE tracked_jobs SET notification_sent = 1 WHERE job_id = ?`, jobID); err != nil {
		return fmt.Errorf("mark job closure notification sent: %w", err)
	}
	return nil
}

func (store *Store) ConfirmDeleteTrackedJob(jobID string) error {
	result, err := store.db.Exec(`DELETE FROM tracked_jobs WHERE job_id = ? AND closure_pending = 1`, jobID)
	if err != nil {
		return fmt.Errorf("delete closed applied job: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check closed applied job deletion: %w", err)
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (store *Store) KeepTrackedJob(jobID string) error {
	result, err := store.db.Exec(
		`UPDATE tracked_jobs SET closure_pending = 0, missing_successful_scans = 0,
		 notification_attempted = 0, notification_sent = 0 WHERE job_id = ? AND closure_pending = 1`,
		jobID,
	)
	if err != nil {
		return fmt.Errorf("keep closed applied job: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check kept applied job: %w", err)
	}
	if count == 0 {
		return ErrNotFound
	}
	if err := store.addJobEvent(jobID, "JOB_RETAINED", "User chose to keep the job in the application tracker", time.Now().UTC()); err != nil {
		return err
	}
	return nil
}

func (store *Store) ListTrackedJobEvents(jobID string) ([]JobEvent, error) {
	rows, err := store.db.Query(
		`SELECT id, job_id, event_type, detail, occurred_at FROM tracked_job_events
		 WHERE job_id = ? ORDER BY occurred_at DESC`,
		jobID,
	)
	if err != nil {
		return nil, fmt.Errorf("query applied job events: %w", err)
	}
	defer rows.Close()
	events := make([]JobEvent, 0)
	for rows.Next() {
		var event JobEvent
		var occurredAt string
		if err := rows.Scan(&event.ID, &event.JobID, &event.EventType, &event.Detail, &occurredAt); err != nil {
			return nil, fmt.Errorf("read applied job event: %w", err)
		}
		event.OccurredAt, err = time.Parse(time.RFC3339Nano, occurredAt)
		if err != nil {
			return nil, fmt.Errorf("parse applied job event timestamp: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applied job events: %w", err)
	}
	return events, nil
}

func (store *Store) addJobEvent(jobID, eventType, detail string, occurredAt time.Time) error {
	if _, err := store.db.Exec(
		`INSERT INTO tracked_job_events (job_id, event_type, detail, occurred_at) VALUES (?, ?, ?, ?)`,
		jobID, eventType, detail, occurredAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("record applied job event: %w", err)
	}
	return nil
}

func (store *Store) ListSources() ([]domain.CareerSource, error) {
	rows, err := store.db.Query(`SELECT id, provider, company, board_id FROM job_sources ORDER BY rowid`)
	if err != nil {
		return nil, fmt.Errorf("query career sources: %w", err)
	}
	defer rows.Close()
	sources := make([]domain.CareerSource, 0)
	for rows.Next() {
		var source domain.CareerSource
		if err := rows.Scan(&source.ID, &source.Provider, &source.Company, &source.BoardID); err != nil {
			return nil, fmt.Errorf("read career source: %w", err)
		}
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate career sources: %w", err)
	}
	return sources, nil
}

func (store *Store) AddSource(source domain.CareerSource) error {
	_, err := store.db.Exec(
		`INSERT INTO job_sources (id, provider, company, board_id) VALUES (?, ?, ?, ?)`,
		source.ID, source.Provider, source.Company, source.BoardID,
	)
	if err != nil {
		return fmt.Errorf("insert career source: %w", err)
	}
	return nil
}

func (store *Store) RemoveSource(id string) error {
	result, err := store.db.Exec(`DELETE FROM job_sources WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("remove career source: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check career source removal: %w", err)
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (store *Store) SaveProfile(profile Profile) error {
	payload, err := json.Marshal(profile)
	if err != nil {
		return fmt.Errorf("encode user profile: %w", err)
	}
	_, err = store.db.Exec(
		`INSERT INTO user_profiles (user_id, payload, updated_at) VALUES ('local', ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET payload = excluded.payload, updated_at = excluded.updated_at`,
		string(payload), time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("save user profile: %w", err)
	}
	return nil
}

func (store *Store) LoadProfile() (Profile, error) {
	var payload string
	err := store.db.QueryRow(`SELECT payload FROM user_profiles WHERE user_id = 'local'`).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, nil
	}
	if err != nil {
		return Profile{}, fmt.Errorf("load user profile: %w", err)
	}
	var profile Profile
	if err := json.Unmarshal([]byte(payload), &profile); err != nil {
		return Profile{}, fmt.Errorf("decode user profile: %w", err)
	}
	return profile, nil
}

func (store *Store) SaveIntegration(config IntegrationConfig) error {
	encryptedSecret, err := store.encrypt([]byte(config.ClientSecret))
	if err != nil {
		return fmt.Errorf("encrypt integration secret: %w", err)
	}
	if encryptedSecret == nil {
		encryptedSecret = []byte{}
	}
	_, err = store.db.Exec(
		`INSERT INTO integration_configs (provider, client_id, client_secret, redirect_url, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(provider) DO UPDATE SET
		 client_id = excluded.client_id,
		 client_secret = CASE WHEN length(excluded.client_secret) = 0 THEN integration_configs.client_secret ELSE excluded.client_secret END,
		 redirect_url = excluded.redirect_url,
		 updated_at = excluded.updated_at`,
		config.Provider, config.ClientID, encryptedSecret, config.RedirectURL, time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("save %s integration configuration: %w", config.Provider, err)
	}
	return nil
}

func (store *Store) LoadIntegration(provider string) (IntegrationConfig, error) {
	var config IntegrationConfig
	var encryptedSecret []byte
	err := store.db.QueryRow(
		`SELECT provider, client_id, client_secret, redirect_url FROM integration_configs WHERE provider = ?`,
		provider,
	).Scan(&config.Provider, &config.ClientID, &encryptedSecret, &config.RedirectURL)
	if errors.Is(err, sql.ErrNoRows) {
		return IntegrationConfig{Provider: provider}, nil
	}
	if err != nil {
		return IntegrationConfig{}, fmt.Errorf("load %s integration configuration: %w", provider, err)
	}
	secret, err := store.decrypt(encryptedSecret)
	if err != nil {
		return IntegrationConfig{}, fmt.Errorf("decrypt %s integration secret: %w", provider, err)
	}
	config.ClientSecret = string(secret)
	config.SecretConfigured = config.ClientSecret != ""
	return config, nil
}

func (store *Store) DeleteIntegration(provider string) error {
	if _, err := store.db.Exec(`DELETE FROM integration_configs WHERE provider = ?`, provider); err != nil {
		return fmt.Errorf("delete %s integration configuration: %w", provider, err)
	}
	return nil
}

func (store *Store) ListPortals() ([]PortalConfig, error) {
	rows, err := store.db.Query(`SELECT id, name, provider, url, notes, status, created_at FROM portal_configs ORDER BY created_at, name`)
	if err != nil {
		return nil, fmt.Errorf("query job portals: %w", err)
	}
	defer rows.Close()
	portals := make([]PortalConfig, 0)
	for rows.Next() {
		var portal PortalConfig
		if err := rows.Scan(&portal.ID, &portal.Name, &portal.Provider, &portal.URL, &portal.Notes, &portal.Status, &portal.CreatedAt); err != nil {
			return nil, fmt.Errorf("read job portal: %w", err)
		}
		portals = append(portals, portal)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate job portals: %w", err)
	}
	return portals, nil
}

func (store *Store) AddPortal(portal PortalConfig) error {
	_, err := store.db.Exec(
		`INSERT INTO portal_configs (id, name, provider, url, notes, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		portal.ID, portal.Name, portal.Provider, portal.URL, portal.Notes, portal.Status, portal.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("save job portal: %w", err)
	}
	return nil
}

func (store *Store) RemovePortal(id string) error {
	result, err := store.db.Exec(`DELETE FROM portal_configs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("remove job portal: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check job portal removal: %w", err)
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (store *Store) SaveResume(resume Resume) error {
	encryptedText, err := store.encrypt([]byte(resume.Text))
	if err != nil {
		return fmt.Errorf("encrypt parsed resume: %w", err)
	}
	_, err = store.db.Exec(
		`INSERT INTO resumes (user_id, filename, parsed_text, updated_at) VALUES ('local', ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		 filename = excluded.filename, parsed_text = excluded.parsed_text, updated_at = excluded.updated_at`,
		resume.Filename, encryptedText, time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("save parsed resume: %w", err)
	}
	return nil
}

func (store *Store) LoadResume() (Resume, error) {
	var resume Resume
	var encryptedText []byte
	var updatedAt string
	err := store.db.QueryRow(
		`SELECT filename, parsed_text, updated_at FROM resumes WHERE user_id = 'local'`,
	).Scan(&resume.Filename, &encryptedText, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Resume{}, nil
	}
	if err != nil {
		return Resume{}, fmt.Errorf("load parsed resume: %w", err)
	}
	parsedText, err := store.decrypt(encryptedText)
	if err != nil {
		return Resume{}, fmt.Errorf("decrypt parsed resume: %w", err)
	}
	resume.Text = string(parsedText)
	resume.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return Resume{}, fmt.Errorf("parse resume timestamp: %w", err)
	}
	return resume, nil
}

func (store *Store) DeleteResume() error {
	if _, err := store.db.Exec(`DELETE FROM resumes WHERE user_id = 'local'`); err != nil {
		return fmt.Errorf("delete parsed resume: %w", err)
	}
	return nil
}

func (store *Store) encrypt(plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, nil
	}
	block, err := aes.NewCipher(store.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return append(nonce, gcm.Seal(nil, nonce, plaintext, nil)...), nil
}

func (store *Store) decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, nil
	}
	block, err := aes.NewCipher(store.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return nil, fmt.Errorf("encrypted value is truncated")
	}
	nonce, value := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	return gcm.Open(nil, nonce, value, nil)
}

func loadOrCreateKey(path string) ([]byte, error) {
	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) != 32 {
			return nil, fmt.Errorf("database encryption key has invalid length")
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return nil, fmt.Errorf("protect database encryption key: %w", err)
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read database encryption key: %w", err)
	}
	key = make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate database encryption key: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		key, err = os.ReadFile(path)
		if err != nil || len(key) != 32 {
			return nil, fmt.Errorf("read database encryption key after concurrent creation: %w", err)
		}
		return key, nil
	}
	if err != nil {
		return nil, fmt.Errorf("create database encryption key: %w", err)
	}
	if _, err := file.Write(key); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("write database encryption key: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("sync database encryption key: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close database encryption key: %w", err)
	}
	return key, nil
}

func NewID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate configuration ID: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

// AgentConfig holds the tunable limits and model settings for the job-search agent.
type AgentConfig = domain.AgentConfig

func DefaultAgentConfig() AgentConfig { return domain.DefaultAgentConfig() }

func (store *Store) SaveAgentConfig(config AgentConfig) error {
	payload, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("encode agent configuration: %w", err)
	}
	_, err = store.db.Exec(
		`INSERT INTO agent_config (id, payload, updated_at) VALUES ('local', ?, ?)
		 ON CONFLICT(id) DO UPDATE SET payload = excluded.payload, updated_at = excluded.updated_at`,
		string(payload), time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("save agent configuration: %w", err)
	}
	return nil
}

// LoadAgentConfig returns the saved configuration, or the defaults when none is saved.
func (store *Store) LoadAgentConfig() (AgentConfig, error) {
	var payload string
	err := store.db.QueryRow(`SELECT payload FROM agent_config WHERE id = 'local'`).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultAgentConfig(), nil
	}
	if err != nil {
		return AgentConfig{}, fmt.Errorf("load agent configuration: %w", err)
	}
	config := DefaultAgentConfig()
	if err := json.Unmarshal([]byte(payload), &config); err != nil {
		return AgentConfig{}, fmt.Errorf("decode agent configuration: %w", err)
	}
	return config, nil
}

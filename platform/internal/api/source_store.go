package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"careerpilot/contracts/domain"
	"careerpilot/platform/internal/integrations/jobs"
	"careerpilot/platform/internal/storage"
)

const maxSources = 50

type SourceStore struct {
	mu    sync.Mutex
	store *storage.Store
}

func (store *SourceStore) Database() *storage.Store {
	return store.store
}

func NewSourceStore(path string) (*SourceStore, error) {
	legacyPath := ""
	databasePath := path
	if strings.EqualFold(filepath.Ext(path), ".json") {
		legacyPath = path
		databasePath = strings.TrimSuffix(path, filepath.Ext(path)) + ".db"
	} else {
		legacyPath = filepath.Join(filepath.Dir(path), "sources.json")
	}
	database, err := storage.Open(databasePath)
	if err != nil {
		return nil, err
	}
	store := &SourceStore{store: database}
	if legacyPath != "" {
		if err := store.importLegacySources(legacyPath); err != nil {
			_ = database.Close()
			return nil, err
		}
	}
	return store, nil
}

func (store *SourceStore) List() ([]domain.CareerSource, error) {
	return store.store.ListSources()
}

func (store *SourceStore) Add(source domain.CareerSource) (domain.CareerSource, error) {
	source, err := jobs.ValidateSource(source)
	if err != nil {
		return domain.CareerSource{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	sources, err := store.store.ListSources()
	if err != nil {
		return domain.CareerSource{}, err
	}
	for _, current := range sources {
		if current.Provider == source.Provider && strings.EqualFold(current.BoardID, source.BoardID) {
			return domain.CareerSource{}, errDuplicateSource
		}
	}
	if len(sources) >= maxSources {
		return domain.CareerSource{}, errSourceLimit
	}
	source.ID, err = storage.NewID()
	if err != nil {
		return domain.CareerSource{}, err
	}
	if err := store.store.AddSource(source); err != nil {
		return domain.CareerSource{}, err
	}
	return source, nil
}

func (store *SourceStore) Remove(id string) ([]domain.CareerSource, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.store.RemoveSource(id); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, errSourceNotFound
		}
		return nil, err
	}
	return store.store.ListSources()
}

func (store *SourceStore) importLegacySources(path string) error {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read legacy career source file: %w", err)
	}
	var sources []domain.CareerSource
	if err := json.Unmarshal(content, &sources); err != nil {
		return fmt.Errorf("decode legacy career source file: %w", err)
	}
	current, err := store.store.ListSources()
	if err != nil {
		return err
	}
	seen := make(map[string]bool, len(current))
	for _, source := range current {
		seen[strings.ToLower(string(source.Provider)+":"+source.BoardID)] = true
	}
	for _, source := range sources {
		source, err = jobs.ValidateSource(source)
		if err != nil {
			return fmt.Errorf("legacy career source is invalid: %w", err)
		}
		key := strings.ToLower(string(source.Provider) + ":" + source.BoardID)
		if seen[key] {
			continue
		}
		if source.ID == "" {
			source.ID, err = storage.NewID()
			if err != nil {
				return err
			}
		}
		if err := store.store.AddSource(source); err != nil {
			return err
		}
		seen[key] = true
	}
	return nil
}

var (
	errDuplicateSource = errors.New("this career board is already configured")
	errSourceLimit     = errors.New("a maximum of 50 career sources is supported")
	errSourceNotFound  = errors.New("source not found")
)

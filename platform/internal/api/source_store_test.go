package api

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"careerpilot/contracts/domain"
)

func TestSourceStorePersistsAndRejectsDuplicateBoards(t *testing.T) {
	store, err := NewSourceStore(filepath.Join(t.TempDir(), "sources.json"))
	if err != nil {
		t.Fatal(err)
	}

	sources, err := store.List()
	if err != nil || len(sources) != 1 || sources[0].BoardID != "stripe" {
		t.Fatalf("unexpected defaults: %#v, %v", sources, err)
	}
	added, err := store.Add(domain.CareerSource{
		Provider: domain.ProviderLever,
		Company:  "Example",
		BoardID:  "example",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add(domain.CareerSource{
		Provider: domain.ProviderLever,
		Company:  "Another Name",
		BoardID:  "EXAMPLE",
	}); !errors.Is(err, errDuplicateSource) {
		t.Fatalf("expected duplicate board error, got %v", err)
	}
	sources, err = store.Remove(added.ID)
	if err != nil || len(sources) != 1 {
		t.Fatalf("could not remove source: %#v, %v", sources, err)
	}
}

func TestSourceStoreImportsLegacyJSONIntoSQLite(t *testing.T) {
	directory := t.TempDir()
	legacyPath := filepath.Join(directory, "sources.json")
	legacySources := []domain.CareerSource{
		{ID: "legacy-lever", Provider: domain.ProviderLever, Company: "Example", BoardID: "example"},
	}
	content, err := json.Marshal(legacySources)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewSourceStore(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 || sources[1].ID != "legacy-lever" {
		t.Fatalf("legacy sources were not migrated: %#v", sources)
	}
	if _, err := os.Stat(filepath.Join(directory, "sources.db")); err != nil {
		t.Fatalf("SQLite database was not created: %v", err)
	}
}

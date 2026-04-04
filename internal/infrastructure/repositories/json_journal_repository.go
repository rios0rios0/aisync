package repositories

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

const journalFileName = "journal.json"

// JSONJournalRepository reads and writes the atomic apply journal as JSON.
type JSONJournalRepository struct {
	basePath string
}

// NewJSONJournalRepository creates a new JSONJournalRepository that stores
// the journal file at <basePath>/journal.json.
func NewJSONJournalRepository(basePath string) *JSONJournalRepository {
	return &JSONJournalRepository{basePath: basePath}
}

// Load reads and parses the journal file.
func (r *JSONJournalRepository) Load() (*entities.Journal, error) {
	path := filepath.Join(r.basePath, journalFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read journal file: %w", err)
	}

	var journal entities.Journal
	if err := json.Unmarshal(data, &journal); err != nil {
		return nil, fmt.Errorf("failed to parse journal file: %w", err)
	}

	return &journal, nil
}

// Save writes the journal to <basePath>/journal.json, creating the directory
// if it does not exist.
func (r *JSONJournalRepository) Save(journal *entities.Journal) error {
	if err := os.MkdirAll(r.basePath, 0755); err != nil {
		return fmt.Errorf("failed to create journal directory: %w", err)
	}

	data, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal journal: %w", err)
	}

	path := filepath.Join(r.basePath, journalFileName)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write journal file: %w", err)
	}

	return nil
}

// Exists checks if a journal file exists at the configured path.
func (r *JSONJournalRepository) Exists() bool {
	path := filepath.Join(r.basePath, journalFileName)
	_, err := os.Stat(path)
	return err == nil
}

// Clear removes the journal file and the staging directory referenced in the
// journal. If the journal cannot be loaded, only the journal file is removed.
func (r *JSONJournalRepository) Clear() error {
	path := filepath.Join(r.basePath, journalFileName)

	journal, err := r.Load()
	if err == nil && journal.StagingDir != "" {
		if removeErr := os.RemoveAll(journal.StagingDir); removeErr != nil {
			return fmt.Errorf("failed to remove staging directory %s: %w", journal.StagingDir, removeErr)
		}
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove journal file: %w", err)
	}

	return nil
}

package repositories

import "github.com/rios0rios0/aisync/internal/domain/entities"

// JournalRepository defines the contract for persisting the atomic apply journal.
type JournalRepository interface {
	Load() (*entities.Journal, error)
	Save(journal *entities.Journal) error
	Exists() bool
	Clear() error
}

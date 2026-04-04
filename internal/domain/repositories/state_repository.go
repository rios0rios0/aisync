package repositories

import "github.com/rios0rios0/aisync/internal/domain/entities"

// StateRepository defines the contract for reading and writing the sync state.
type StateRepository interface {
	Load(repoPath string) (*entities.State, error)
	Save(repoPath string, state *entities.State) error
	Exists(repoPath string) bool
}

package repositories

import "github.com/rios0rios0/aisync/internal/domain/entities"

// DiffService defines the contract for computing how files incoming from
// external sources differ from what is on disk. Personal files are compared
// by the three-way PersonalSyncer instead.
type DiffService interface {
	// ComputeSharedDiff compares the current files in tool directories against
	// the incoming shared files from external sources.
	ComputeSharedDiff(
		config *entities.Config,
		repoPath string,
		incomingFiles map[string][]byte,
	) ([]entities.FileChange, error)
}

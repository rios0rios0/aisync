package repositories

import "github.com/rios0rios0/aisync/internal/domain/entities"

// DiffService defines the contract for computing differences between local
// disk state, sync repo state, and remote state.
type DiffService interface {
	// ComputeSharedDiff compares the current files in tool directories against
	// the incoming shared files from external sources.
	ComputeSharedDiff(
		config *entities.Config,
		repoPath string,
		incomingFiles map[string][]byte,
	) ([]entities.FileChange, error)

	// ComputeLocalDiff compares files in tool directories against the sync repo
	// to find uncommitted personal changes.
	ComputeLocalDiff(
		config *entities.Config,
		repoPath string,
	) ([]entities.FileChange, error)

	// ComputePersonalDiff detects incoming personal changes from other devices.
	// It walks the sync repo's personal/<tool>/ directories and finds files that
	// do not exist on the local disk or whose repo version differs from local.
	ComputePersonalDiff(
		config *entities.Config,
		repoPath string,
	) ([]entities.FileChange, error)
}

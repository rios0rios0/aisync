package repositories

import "github.com/rios0rios0/aisync/internal/domain/entities"

// ApplyService defines the contract for atomically staging and applying files.
type ApplyService interface {
	// Stage writes all files to a temporary staging directory and returns a
	// journal that tracks the pending operations. The files map keys are final
	// target paths and values are the file contents.
	Stage(files map[string][]byte) (*entities.Journal, error)

	// StageVerbatim stages files like Stage but writes their bytes exactly
	// as given, for content that must round-trip unchanged (personal files
	// and conversations), where rewritten line endings would read as a local
	// change on the next sync.
	StageVerbatim(files map[string][]byte) (*entities.Journal, error)

	// Apply moves all pending staged files from the staging directory to their
	// final target paths, updating the journal after each successful move.
	Apply(journal *entities.Journal) error

	// Recover checks for an incomplete journal from a previous interrupted
	// apply and resumes it. If the staging directory is missing, it clears the
	// corrupt journal state.
	Recover() error
}

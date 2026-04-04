package repositories

import "github.com/rios0rios0/aisync/internal/domain/entities"

// ConflictDetector defines the contract for detecting and resolving file
// conflicts between devices during pull operations.
type ConflictDetector interface {
	// DetectConflicts compares local files in toolDir against incoming personal
	// files from the sync repo. It uses the manifest to determine whether both
	// sides diverged since the last sync. When a conflict is found, a
	// .conflict.<device> file is written alongside the local file.
	DetectConflicts(
		toolDir string,
		incomingFiles map[string][]byte,
		manifest *entities.Manifest,
		deviceName string,
	) ([]entities.Conflict, error)

	// ResolveConflict resolves a single conflict by applying the user's choice.
	// The choice parameter must be "local" or "remote".
	ResolveConflict(toolDir string, conflict entities.Conflict, choice string) error
}

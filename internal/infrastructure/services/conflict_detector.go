package services

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

// ConflictDetector detects and resolves conflicting personal files between
// devices. A conflict occurs when both the local version and the incoming
// version differ from the last synced state recorded in the manifest.
type ConflictDetector struct{}

// NewConflictDetector creates a new ConflictDetector.
func NewConflictDetector() *ConflictDetector {
	return &ConflictDetector{}
}

// DetectConflicts compares local personal files against incoming personal files
// from the sync repo (pulled from another device). Returns conflicts where both
// versions differ from the last synced state.
func (d *ConflictDetector) DetectConflicts(
	toolDir string,
	incomingPersonalFiles map[string][]byte,
	manifest *entities.Manifest,
	deviceName string,
) ([]entities.Conflict, error) {
	var conflicts []entities.Conflict

	for relPath, incomingContent := range incomingPersonalFiles {
		localPath := filepath.Join(toolDir, relPath)

		localContent, err := os.ReadFile(localPath)
		if err != nil {
			// Local file does not exist; no conflict — it is a new file
			// from the remote device.
			continue
		}

		localChecksum := checksumContent(localContent)
		incomingChecksum := checksumContent(incomingContent)

		// If both sides have the same content, there is no conflict.
		if localChecksum == incomingChecksum {
			continue
		}

		// Check whether the local file was modified since the last sync.
		// If the manifest records a checksum and the local file still
		// matches it, then only the remote side changed — no conflict.
		if mf, ok := manifest.Files[relPath]; ok {
			if localChecksum == mf.Checksum {
				// Local unchanged, remote changed — safe to overwrite.
				continue
			}
		}

		// Both sides diverged from the last known state — this is a conflict.
		conflict := entities.Conflict{
			Path:          relPath,
			LocalDevice:   manifest.Device,
			RemoteDevice:  deviceName,
			LocalContent:  localContent,
			RemoteContent: incomingContent,
		}

		// Write the .conflict.<device> file so the user can inspect it.
		conflictPath := filepath.Join(toolDir, conflict.ConflictFileName())
		conflictDir := filepath.Dir(conflictPath)
		if err := os.MkdirAll(conflictDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create conflict directory %s: %w", conflictDir, err)
		}
		if err := os.WriteFile(conflictPath, incomingContent, 0644); err != nil {
			return nil, fmt.Errorf("failed to write conflict file %s: %w", conflictPath, err)
		}

		conflicts = append(conflicts, conflict)
	}

	return conflicts, nil
}

// ResolveConflict resolves a conflict by choosing either the local or remote
// version. The choice parameter must be "local" or "remote".
//   - "local": the conflict file is deleted and the local version is kept.
//   - "remote": the local file is replaced with the incoming content and the
//     conflict file is deleted.
func (d *ConflictDetector) ResolveConflict(toolDir string, conflict entities.Conflict, choice string) error {
	conflictPath := filepath.Join(toolDir, conflict.ConflictFileName())
	localPath := filepath.Join(toolDir, conflict.Path)

	switch choice {
	case "local":
		if err := os.Remove(conflictPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove conflict file %s: %w", conflictPath, err)
		}
	case "remote":
		localDir := filepath.Dir(localPath)
		if err := os.MkdirAll(localDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", localDir, err)
		}
		if err := os.WriteFile(localPath, conflict.RemoteContent, 0644); err != nil {
			return fmt.Errorf("failed to write resolved file %s: %w", localPath, err)
		}
		if err := os.Remove(conflictPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove conflict file %s: %w", conflictPath, err)
		}
	default:
		return fmt.Errorf("invalid choice %q: must be \"local\" or \"remote\"", choice)
	}

	return nil
}

func checksumContent(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", h)
}

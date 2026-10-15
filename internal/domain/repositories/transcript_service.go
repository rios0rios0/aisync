package repositories

import (
	"errors"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

// ErrSnapshotMoved is returned by [TranscriptService.Publish] when another
// device published to the transcript branch after the snapshot was fetched.
// The caller fetches again and redoes the merge instead of overwriting the
// other device's conversations.
var ErrSnapshotMoved = errors.New("the transcript branch changed since it was fetched")

// TranscriptService stores conversation transcripts on a branch of the sync
// repo that never accumulates history: every publish replaces the branch with
// a single commit holding the current snapshot, so the repository stays the
// size of the conversations being synced. File contents and names are
// encrypted; only changed files are uploaded.
type TranscriptService interface {
	// Fetch returns the snapshot currently on branch, or an empty snapshot
	// with no lease when the branch does not exist yet.
	Fetch(repoPath, branch, identityPath string) (*entities.TranscriptSnapshot, error)

	// Read returns the decrypted content of one file of the snapshot fetched
	// at lease.
	Read(repoPath, lease string, file entities.TranscriptFile, identityPath string) ([]byte, error)

	// Publish replaces branch with snapshot as a single parentless commit.
	// Files carrying Content are encrypted and stored; every other file
	// reuses its stored blob. It returns [ErrSnapshotMoved] when the branch
	// no longer points at snapshot.Lease.
	Publish(
		repoPath, branch string,
		snapshot *entities.TranscriptSnapshot,
		recipients []string,
		identityPath, message string,
	) error
}

// TranscriptBaseRepository persists, per device, the checksum each
// transcript file had when this device last synced it: the base of the
// three-way merge. It is device-local state and never synced.
type TranscriptBaseRepository interface {
	Load(repoPath string) (map[string]string, error)
	Save(repoPath string, base map[string]string) error
}

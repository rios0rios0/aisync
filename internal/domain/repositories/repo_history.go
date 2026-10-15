package repositories

import "time"

// RepoHistory reports where the sync repo's copy of a file came from, so a
// conflict prompt can say when and by which device the repo side changed.
type RepoHistory interface {
	// LastChange returns the commit time and subject of the most recent
	// commit that touched any of relPaths (slash-separated, relative to the
	// repository root at repoPath), skipping commits that carry the
	// entities.ChangeTrailerKey trailer with entities.ChangeEncryptionOnly,
	// since those only changed how a file is stored. It returns a zero time
	// and an empty subject when the paths have no such history or the history
	// is unavailable.
	LastChange(repoPath string, relPaths ...string) (time.Time, string, error)
}

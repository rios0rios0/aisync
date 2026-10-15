package repositories

import "time"

// RepoHistory reports where the sync repo's copy of a file came from, so a
// conflict prompt can say when and by which device the repo side changed.
type RepoHistory interface {
	// LastChange returns the commit time and subject of the most recent
	// commit that touched relPath (slash-separated, relative to the repository
	// root at repoPath). It returns a zero time and an empty subject when the
	// path has no history or the history is unavailable.
	LastChange(repoPath, relPath string) (time.Time, string, error)
}

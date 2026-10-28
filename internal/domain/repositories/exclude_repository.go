package repositories

import "github.com/rios0rios0/aisync/internal/domain/entities"

// ExcludeRepository loads this device's exclusions: the synced paths it opts
// out of. They live outside the sync repo, so they are never committed or
// synced, and every other device goes on syncing those paths.
type ExcludeRepository interface {
	// Load returns the exclusions. A missing file yields none; one that
	// exists but cannot be read is an error, so a sync never runs as if the
	// device had opted out of nothing.
	Load() (*entities.ExcludePatterns, error)

	// Path returns the file Load reads.
	Path() string
}

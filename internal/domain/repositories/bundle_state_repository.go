package repositories

import "github.com/rios0rios0/aisync/internal/domain/entities"

// BundleStateRepository persists the per-device cache of last-seen
// bundle hashes. Implementations must keep the on-disk file at 0600 and
// return an empty state (rather than an error) when the file does not
// yet exist, since first-run pulls have no prior cache to compare to.
type BundleStateRepository interface {
	Load() (*entities.BundleState, error)
	Save(state *entities.BundleState) error
}

package entities

import "time"

// BundleStateEntry records what aisync remembers about a single bundle
// from the most recent successful pull. Pull-side deletion detection
// computes the difference between the cached set of hashes and what is
// present in the freshly-pulled tree; anything that disappeared is a
// candidate for the user-confirmation prompt.
type BundleStateEntry struct {
	OriginalName string    `json:"original_name"`
	Tool         string    `json:"tool"`
	Target       string    `json:"target"`
	LastSeen     time.Time `json:"last_seen"`
}

// BundleState is the per-device cache of last-seen bundles, keyed by the
// 16-hex-character bundle hash so it survives the original directory
// name being intentionally hidden in the git tree.
type BundleState struct {
	Bundles map[string]BundleStateEntry `json:"bundles"`
}

// NewBundleState constructs an empty BundleState ready for population.
func NewBundleState() *BundleState {
	return &BundleState{Bundles: make(map[string]BundleStateEntry)}
}

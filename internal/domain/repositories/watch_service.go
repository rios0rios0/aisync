package repositories

import (
	"time"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

// FileEvent represents a filesystem change detected by the watch service.
type FileEvent struct {
	Path string
	Op   string // "create", "write", "remove", "rename"
}

// WatchedTree carries enough context for the watch service to apply the
// per-tool allowlist when filtering filesystem events. Each tree is a
// separate root that the watcher monitors, and events are allowed through
// only when their tool-relative path is syncable under that tool's rules.
type WatchedTree struct {
	ToolName       string
	Dir            string
	ExtraAllowlist []string
}

// WatchService defines the contract for monitoring AI tool directories
// for file changes in real-time.
type WatchService interface {
	// Watch starts monitoring the given tool trees for file changes. Each
	// tree is filtered through [entities.IsSyncable] with the tree's tool
	// name and extra_allowlist, so the watcher never emits events for
	// paths outside the allowlist. The callback is invoked for each
	// surviving event.
	Watch(trees []WatchedTree, callback func(event FileEvent)) error

	// Stop stops the file watcher.
	Stop()

	// SetIgnorePatterns configures an additional user-level ignore filter
	// applied on top of the allowlist (subtractive).
	SetIgnorePatterns(patterns *entities.IgnorePatterns)

	// SetInterval updates the polling interval. Only effective for polling-based
	// implementations; fsnotify-based implementations ignore this.
	SetInterval(d time.Duration)
}

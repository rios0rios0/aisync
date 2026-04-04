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

// WatchService defines the contract for monitoring AI tool directories
// for file changes in real-time.
type WatchService interface {
	// Watch starts monitoring the given directories for file changes.
	// The callback is invoked for each detected change (after filtering).
	Watch(dirs []string, callback func(event FileEvent)) error

	// Stop stops the file watcher.
	Stop()

	// SetIgnorePatterns configures the ignore patterns used to filter events.
	SetIgnorePatterns(patterns *entities.IgnorePatterns)

	// SetInterval updates the polling interval. Only effective for polling-based
	// implementations; fsnotify-based implementations ignore this.
	SetInterval(d time.Duration)
}

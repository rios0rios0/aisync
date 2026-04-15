package services

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// FSNotifyWatchService watches directories using OS-native filesystem events.
type FSNotifyWatchService struct {
	watcher        *fsnotify.Watcher
	stopCh         chan struct{}
	stopped        bool
	mu             sync.Mutex
	ignorePatterns *entities.IgnorePatterns
	trees          []repositories.WatchedTree
}

// NewFSNotifyWatchService creates a new FSNotifyWatchService.
func NewFSNotifyWatchService() *FSNotifyWatchService {
	return &FSNotifyWatchService{
		stopCh: make(chan struct{}),
	}
}

// SetIgnorePatterns configures the ignore patterns used to filter watched events.
func (s *FSNotifyWatchService) SetIgnorePatterns(patterns *entities.IgnorePatterns) {
	s.ignorePatterns = patterns
}

// SetInterval is a no-op for fsnotify-based watching (only applies to polling).
func (s *FSNotifyWatchService) SetInterval(_ time.Duration) {}

// Watch starts monitoring the given tool trees for file changes.
func (s *FSNotifyWatchService) Watch(
	trees []repositories.WatchedTree,
	callback func(event repositories.FileEvent),
) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	s.watcher = watcher
	s.trees = trees

	for _, tree := range trees {
		if addErr := s.addRecursive(tree); addErr != nil {
			logger.Warnf("failed to watch %s: %v", tree.Dir, addErr)
		}
	}

	go s.eventLoop(callback)
	return nil
}

// Stop stops the file watcher.
func (s *FSNotifyWatchService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.stopped = true
	close(s.stopCh)
	if s.watcher != nil {
		_ = s.watcher.Close()
	}
}

func (s *FSNotifyWatchService) eventLoop(callback func(event repositories.FileEvent)) {
	for {
		select {
		case <-s.stopCh:
			return
		case event, ok := <-s.watcher.Events:
			if !ok {
				return
			}
			s.handleFSEvent(event, callback)
		case err, ok := <-s.watcher.Errors:
			if !ok {
				return
			}
			logger.Warnf("watch error: %v", err)
		}
	}
}

// handleFSEvent processes a single filesystem event, filtering out paths
// that are not in the allowlist for the tree they belong to, applying the
// user-level ignore patterns, invoking the callback, and watching newly
// created directories.
func (s *FSNotifyWatchService) handleFSEvent(
	event fsnotify.Event,
	callback func(event repositories.FileEvent),
) {
	tree, relPath, ok := s.lookupTree(event.Name)
	if !ok {
		return
	}
	if !entities.IsSyncable(tree.ToolName, relPath, tree.ExtraAllowlist) {
		return
	}
	if s.ignorePatterns != nil && s.ignorePatterns.Matches(relPath) {
		return
	}

	fe := repositories.FileEvent{
		Path: event.Name,
		Op:   mapOp(event.Op),
	}
	callback(fe)

	if event.Op&fsnotify.Create != 0 {
		info, statErr := os.Stat(event.Name)
		if statErr == nil && info.IsDir() {
			if addErr := s.addRecursive(tree); addErr != nil {
				logger.Warnf("failed to watch new directory %s: %v", event.Name, addErr)
			}
		}
	}
}

// lookupTree finds which watched tree owns the given absolute path and
// returns the tree plus the tool-relative path for allowlist checks.
func (s *FSNotifyWatchService) lookupTree(absPath string) (repositories.WatchedTree, string, bool) {
	for _, tree := range s.trees {
		relPath, err := filepath.Rel(tree.Dir, absPath)
		if err != nil {
			continue
		}
		// filepath.Rel returns "../..." when absPath is outside tree.Dir.
		if relPath == "" || relPath == "." || strings.HasPrefix(relPath, "..") {
			continue
		}
		return tree, relPath, true
	}
	return repositories.WatchedTree{}, "", false
}

// addRecursive registers every directory under tree.Dir with the underlying
// fsnotify watcher. Directory-level pruning via the allowlist would be
// unsafe: patterns like "rules/**" match files whose first segment is
// "rules" but do NOT match the bare directory segment "rules" itself, so
// pruning by IsSyncable here would refuse to watch any subdir. Instead we
// keep all directories watched and rely on per-file filtering in
// handleFSEvent; only the user-level ignore patterns prune whole subtrees.
func (s *FSNotifyWatchService) addRecursive(tree repositories.WatchedTree) error {
	return filepath.WalkDir(tree.Dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if s.ignorePatterns != nil {
			relPath, relErr := filepath.Rel(tree.Dir, path)
			if relErr == nil && s.ignorePatterns.Matches(relPath) {
				return filepath.SkipDir
			}
		}
		return s.watcher.Add(path)
	})
}

// PollingWatchService watches directories by polling at a configurable interval.
// This is the fallback for platforms without inotify (e.g., Termux/Android).
type PollingWatchService struct {
	interval       time.Duration
	stopCh         chan struct{}
	stopped        bool
	mu             sync.Mutex
	state          map[string]time.Time
	ignorePatterns *entities.IgnorePatterns
	trees          []repositories.WatchedTree
}

// NewPollingWatchService creates a new PollingWatchService.
func NewPollingWatchService(interval time.Duration) *PollingWatchService {
	return &PollingWatchService{
		interval: interval,
		stopCh:   make(chan struct{}),
		state:    make(map[string]time.Time),
	}
}

// SetIgnorePatterns configures the ignore patterns used to filter polled events.
func (s *PollingWatchService) SetIgnorePatterns(patterns *entities.IgnorePatterns) {
	s.ignorePatterns = patterns
}

// SetInterval updates the polling interval.
func (s *PollingWatchService) SetInterval(d time.Duration) {
	s.interval = d
}

// Watch starts polling the given tool trees for file changes.
func (s *PollingWatchService) Watch(
	trees []repositories.WatchedTree,
	callback func(event repositories.FileEvent),
) error {
	s.trees = trees

	// Build initial state
	for _, tree := range trees {
		s.scanDir(tree)
	}

	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				for _, tree := range trees {
					s.pollDir(tree, callback)
				}
			}
		}
	}()

	return nil
}

// Stop stops the polling watcher.
func (s *PollingWatchService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.stopped = true
	close(s.stopCh)
}

func (s *PollingWatchService) scanDir(tree repositories.WatchedTree) {
	_ = filepath.WalkDir(tree.Dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		relPath, relErr := filepath.Rel(tree.Dir, path)
		if relErr != nil {
			return nil //nolint:nilerr // return nil to continue WalkDir traversal
		}
		if !entities.IsSyncable(tree.ToolName, relPath, tree.ExtraAllowlist) {
			return nil
		}
		if s.ignorePatterns != nil && s.ignorePatterns.Matches(relPath) {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr == nil {
			s.state[path] = info.ModTime()
		}
		return nil
	})
}

func (s *PollingWatchService) pollDir(tree repositories.WatchedTree, callback func(event repositories.FileEvent)) {
	current := make(map[string]time.Time)

	_ = filepath.WalkDir(tree.Dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		relPath, relErr := filepath.Rel(tree.Dir, path)
		if relErr != nil {
			return nil //nolint:nilerr // return nil to continue WalkDir traversal
		}
		if !entities.IsSyncable(tree.ToolName, relPath, tree.ExtraAllowlist) {
			return nil
		}
		if s.ignorePatterns != nil && s.ignorePatterns.Matches(relPath) {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr == nil {
			current[path] = info.ModTime()
		}
		return nil
	})

	// Detect new or modified files
	for path, modTime := range current {
		oldTime, existed := s.state[path]
		if !existed {
			callback(repositories.FileEvent{Path: path, Op: "create"})
		} else if modTime.After(oldTime) {
			callback(repositories.FileEvent{Path: path, Op: "write"})
		}
	}

	// Detect removed files
	for path := range s.state {
		if _, exists := current[path]; !exists {
			callback(repositories.FileEvent{Path: path, Op: "remove"})
		}
	}

	s.state = current
}

func mapOp(op fsnotify.Op) string {
	switch {
	case op&fsnotify.Create != 0:
		return "create"
	case op&fsnotify.Write != 0:
		return "write"
	case op&fsnotify.Remove != 0:
		return "remove"
	case op&fsnotify.Rename != 0:
		return "rename"
	default:
		return "unknown"
	}
}

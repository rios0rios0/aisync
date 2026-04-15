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

// handleFSEvent processes a single filesystem event.
//
// Directory events (create on a new subdirectory) are handled BEFORE the
// file-level allowlist check, because we want to add a watcher for any
// new subtree that could contain allowlisted content — even if the bare
// directory name is a plain literal. For the allowlisted-subtree check
// we still use [entities.IsSyncable] because the gitwildmatch matcher
// correctly handles the bare-directory case via "**" collapsing to zero
// segments (e.g. "rules/**" matches the bare segment "rules"). A directory
// create never invokes the callback — directories themselves are not
// pushed; only the file events under them matter.
//
// File events (create/write/remove on a regular file) flow through the
// allowlist + ignore filter and then the user callback.
func (s *FSNotifyWatchService) handleFSEvent(
	event fsnotify.Event,
	callback func(event repositories.FileEvent),
) {
	tree, relPath, ok := s.lookupTree(event.Name)
	if !ok {
		return
	}

	if event.Op&fsnotify.Create != 0 {
		if info, statErr := os.Stat(event.Name); statErr == nil && info.IsDir() {
			s.watchNewSubtree(tree, event.Name, relPath)
			return
		}
	}

	if !entities.IsSyncable(tree.ToolName, relPath, tree.ExtraAllowlist) {
		return
	}
	if s.ignorePatterns != nil && s.ignorePatterns.Matches(relPath) {
		return
	}

	callback(repositories.FileEvent{
		Path: event.Name,
		Op:   mapOp(event.Op),
	})
}

// watchNewSubtree registers watches for a newly-created directory and its
// descendants, pruning non-allowlisted subtrees the same way the initial
// walk does. Only the new subtree is traversed — not the whole tool root —
// so a busy tool directory does not trigger an O(N) re-walk on every mkdir.
func (s *FSNotifyWatchService) watchNewSubtree(
	tree repositories.WatchedTree,
	newDir, newDirRelPath string,
) {
	if !entities.IsSyncable(tree.ToolName, newDirRelPath, tree.ExtraAllowlist) {
		return
	}
	if s.ignorePatterns != nil && s.ignorePatterns.Matches(newDirRelPath) {
		return
	}
	if addErr := s.addRecursiveFrom(tree, newDir); addErr != nil {
		logger.Warnf("failed to watch new directory %s: %v", newDir, addErr)
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

// addRecursive walks tree.Dir and registers watches for every directory
// whose tool-relative path is allowlisted (plus the tool root itself).
// Non-allowlisted subtrees like ~/.claude/projects/ are pruned via
// [filepath.SkipDir] so a busy tool home does not burn thousands of
// inotify watches on runtime/cache directories whose events would be
// filtered out anyway. Directory pruning is safe because the allowlist
// matcher correctly handles the bare-directory case: a pattern like
// "rules/**" matches the bare segment "rules" via "**" collapsing to
// zero segments, so descending into allowlisted subtrees still works.
func (s *FSNotifyWatchService) addRecursive(tree repositories.WatchedTree) error {
	return s.addRecursiveFrom(tree, tree.Dir)
}

// addRecursiveFrom walks the subtree rooted at `root` (which may be
// `tree.Dir` itself or a newly-created child) and adds watches for each
// directory whose tool-relative path passes the allowlist check. The tool
// root is always watched so top-level files like settings.json are seen.
// User-level ignore patterns cause a SkipDir on matching subtrees.
func (s *FSNotifyWatchService) addRecursiveFrom(tree repositories.WatchedTree, root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		relPath, relErr := filepath.Rel(tree.Dir, path)
		if relErr != nil {
			return nil //nolint:nilerr // return nil to continue WalkDir traversal
		}
		// Always watch the tool root (and any ancestor-rooted walk re-entry
		// whose relative path is "." or ""). Root-level files like
		// settings.json are discovered via this watch.
		if relPath == "." || relPath == "" {
			return s.watcher.Add(path)
		}
		if s.ignorePatterns != nil && s.ignorePatterns.Matches(relPath) {
			return filepath.SkipDir
		}
		if !entities.IsSyncable(tree.ToolName, relPath, tree.ExtraAllowlist) {
			return filepath.SkipDir
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

package services

import (
	"os"
	"path/filepath"
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

// Watch starts monitoring the given directories for file changes.
func (s *FSNotifyWatchService) Watch(dirs []string, callback func(event repositories.FileEvent)) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	s.watcher = watcher

	for _, dir := range dirs {
		if addErr := s.addRecursive(dir); addErr != nil {
			logger.Warnf("failed to watch %s: %v", dir, addErr)
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

// handleFSEvent processes a single filesystem event, filtering denied and ignored
// paths, invoking the callback, and watching newly created directories.
func (s *FSNotifyWatchService) handleFSEvent(
	event fsnotify.Event,
	callback func(event repositories.FileEvent),
) {
	if entities.IsDenied(event.Name) {
		return
	}
	if s.ignorePatterns != nil && s.ignorePatterns.Matches(event.Name) {
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
			if addErr := s.addRecursive(event.Name); addErr != nil {
				logger.Warnf("failed to watch new directory %s: %v", event.Name, addErr)
			}
		}
	}
}

func (s *FSNotifyWatchService) addRecursive(dir string) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if entities.IsDenied(path) {
				return filepath.SkipDir
			}
			if s.ignorePatterns != nil && s.ignorePatterns.Matches(path) {
				return filepath.SkipDir
			}
			return s.watcher.Add(path)
		}
		return nil
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

// Watch starts polling the given directories for file changes.
func (s *PollingWatchService) Watch(dirs []string, callback func(event repositories.FileEvent)) error {
	// Build initial state
	for _, dir := range dirs {
		s.scanDir(dir)
	}

	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				for _, dir := range dirs {
					s.pollDir(dir, callback)
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

func (s *PollingWatchService) scanDir(dir string) {
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if entities.IsDenied(path) {
			return nil
		}
		if s.ignorePatterns != nil && s.ignorePatterns.Matches(path) {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr == nil {
			s.state[path] = info.ModTime()
		}
		return nil
	})
}

func (s *PollingWatchService) pollDir(dir string, callback func(event repositories.FileEvent)) {
	current := make(map[string]time.Time)

	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if entities.IsDenied(path) {
			return nil
		}
		if s.ignorePatterns != nil && s.ignorePatterns.Matches(path) {
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

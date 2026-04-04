package commands

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// WatchCommand monitors AI tool directories for file changes.
type WatchCommand struct {
	configRepo   repositories.ConfigRepository
	watchService repositories.WatchService
	pushCmd      *PushCommand
}

// NewWatchCommand creates a new WatchCommand.
func NewWatchCommand(
	configRepo repositories.ConfigRepository,
	watchService repositories.WatchService,
	pushCmd *PushCommand,
) *WatchCommand {
	return &WatchCommand{
		configRepo:   configRepo,
		watchService: watchService,
		pushCmd:      pushCmd,
	}
}

// Execute starts watching AI tool directories for changes.
func (c *WatchCommand) Execute(configPath, repoPath string, autoPush bool, debounce time.Duration) error {
	config, err := c.configRepo.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	var dirs []string
	for _, tool := range config.Tools {
		if !tool.Enabled {
			continue
		}
		dir := ExpandHome(tool.Path)
		if _, err := os.Stat(dir); err == nil {
			dirs = append(dirs, dir)
		}
	}

	if len(dirs) == 0 {
		return fmt.Errorf("no enabled AI tool directories found")
	}

	// Load ignore patterns from both .aisyncignore and config.Watch.IgnoredPatterns.
	var allPatterns []string

	// Patterns from .aisyncignore file at repo root
	ignorePath := filepath.Join(repoPath, ".aisyncignore")
	if content, readErr := os.ReadFile(ignorePath); readErr == nil {
		filePatterns := entities.ParseIgnorePatterns(content)
		allPatterns = append(allPatterns, filePatterns.Patterns...)
	}

	// Patterns from config.yaml watch.ignored_patterns
	allPatterns = append(allPatterns, config.Watch.IgnoredPatterns...)

	if len(allPatterns) > 0 {
		combined := &entities.IgnorePatterns{Patterns: allPatterns}
		c.watchService.SetIgnorePatterns(combined)
		logger.Infof("loaded %d ignore patterns (%d from .aisyncignore, %d from config)",
			len(allPatterns), len(allPatterns)-len(config.Watch.IgnoredPatterns), len(config.Watch.IgnoredPatterns))
	}

	fmt.Printf("Watching %d directories for changes...\n", len(dirs))
	if autoPush {
		fmt.Printf("Auto-push enabled (debounce: %s)\n", debounce)
	}

	var mu sync.Mutex
	var pending []repositories.FileEvent
	var debounceTimer *time.Timer

	callback := func(event repositories.FileEvent) {
		logger.Infof("detected: %s %s", event.Op, event.Path)

		if !autoPush {
			return
		}

		mu.Lock()
		defer mu.Unlock()

		pending = append(pending, event)

		if debounceTimer != nil {
			debounceTimer.Stop()
		}
		debounceTimer = time.AfterFunc(debounce, func() {
			mu.Lock()
			events := pending
			pending = nil
			mu.Unlock()

			if len(events) == 0 {
				return
			}

			logger.Infof("debounce window closed, pushing %d changes", len(events))
			msg := fmt.Sprintf("sync(watch): %d file(s) changed", len(events))
			if pushErr := c.pushCmd.Execute(configPath, repoPath, msg, false, false); pushErr != nil {
				logger.Warnf("auto-push failed: %v", pushErr)
			}
		})
	}

	if err := c.watchService.Watch(dirs, callback); err != nil {
		return fmt.Errorf("failed to start watcher: %w", err)
	}

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nStopping watcher...")
	c.watchService.Stop()
	return nil
}

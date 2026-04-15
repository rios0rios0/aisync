package commands

import (
	"errors"
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
func (c *WatchCommand) Execute(
	configPath, repoPath string,
	autoPush bool,
	debounce, pollingInterval time.Duration,
) error {
	config, err := c.configRepo.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if pollingInterval > 0 {
		c.watchService.SetInterval(pollingInterval)
	}

	trees := c.collectWatchedTrees(config)
	if len(trees) == 0 {
		return errors.New("no enabled AI tool directories found")
	}

	c.loadAndApplyIgnorePatterns(config, repoPath)

	fmt.Fprintf(os.Stdout, "Watching %d directories for changes...\n", len(trees))
	if autoPush && c.pushCmd == nil {
		return errors.New("auto-push is enabled but no push command is configured")
	}

	if autoPush {
		fmt.Fprintf(os.Stdout, "Auto-push enabled (debounce: %s)\n", debounce)
	}

	callback := c.buildWatchCallback(configPath, repoPath, autoPush, debounce)

	if err = c.watchService.Watch(trees, callback); err != nil {
		return fmt.Errorf("failed to start watcher: %w", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Fprintln(os.Stdout, "\nStopping watcher...")
	c.watchService.Stop()
	return nil
}

// collectWatchedTrees returns a WatchedTree per existing enabled tool, each
// carrying the tool name and user extra_allowlist so the watch service can
// apply [entities.IsSyncable] to each event.
func (c *WatchCommand) collectWatchedTrees(config *entities.Config) []repositories.WatchedTree {
	var trees []repositories.WatchedTree
	for name, tool := range config.Tools {
		if !tool.Enabled {
			continue
		}
		dir := ExpandHome(tool.Path)
		if _, err := os.Stat(dir); err == nil {
			trees = append(trees, repositories.WatchedTree{
				ToolName:       name,
				Dir:            dir,
				ExtraAllowlist: tool.ExtraAllowlist,
			})
		}
	}
	return trees
}

// loadAndApplyIgnorePatterns combines ignore patterns from .aisyncignore and
// config.Watch.IgnoredPatterns, then applies them to the watch service.
func (c *WatchCommand) loadAndApplyIgnorePatterns(config *entities.Config, repoPath string) {
	var allPatterns []string

	ignorePath := filepath.Join(repoPath, ".aisyncignore")
	if content, readErr := os.ReadFile(ignorePath); readErr == nil {
		filePatterns := entities.ParseIgnorePatterns(content)
		allPatterns = append(allPatterns, filePatterns.Patterns...)
	}

	allPatterns = append(allPatterns, config.Watch.IgnoredPatterns...)

	if len(allPatterns) > 0 {
		combined := &entities.IgnorePatterns{Patterns: allPatterns}
		c.watchService.SetIgnorePatterns(combined)
		logger.Infof("loaded %d ignore patterns (%d from .aisyncignore, %d from config)",
			len(allPatterns), len(allPatterns)-len(config.Watch.IgnoredPatterns), len(config.Watch.IgnoredPatterns))
	}
}

// buildWatchCallback creates the callback function for the watch service,
// including debounced auto-push when enabled.
func (c *WatchCommand) buildWatchCallback(
	configPath, repoPath string,
	autoPush bool,
	debounce time.Duration,
) func(event repositories.FileEvent) {
	var mu sync.Mutex
	var pending []repositories.FileEvent
	var debounceTimer *time.Timer

	return func(event repositories.FileEvent) {
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
}

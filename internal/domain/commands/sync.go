package commands

import (
	"fmt"

	logger "github.com/sirupsen/logrus"
)

// SyncCommand orchestrates a full pull-then-push cycle.
type SyncCommand struct {
	pullCmd *PullCommand
	pushCmd *PushCommand
}

// NewSyncCommand creates a new SyncCommand.
func NewSyncCommand(pullCmd *PullCommand, pushCmd *PushCommand) *SyncCommand {
	return &SyncCommand{
		pullCmd: pullCmd,
		pushCmd: pushCmd,
	}
}

// Execute runs a full sync: pull shared files from sources, then push personal
// files to the sync repository. When dryRun is true, only the pull phase runs.
func (c *SyncCommand) Execute(configPath, repoPath, commitMsg string, dryRun bool) error {
	logger.Info("starting sync: pull phase")
	pullOpts := PullOptions{
		DryRun: dryRun,
		Force:  true, // sync always forces the pull phase (no interactive prompt)
	}
	if err := c.pullCmd.Execute(configPath, repoPath, pullOpts); err != nil {
		return fmt.Errorf("pull phase failed: %w", err)
	}

	if dryRun {
		logger.Info("dry-run mode: skipping push phase")
		return nil
	}

	logger.Info("starting sync: push phase")
	if err := c.pushCmd.Execute(configPath, repoPath, commitMsg, PushOptions{}); err != nil {
		return fmt.Errorf("push phase failed: %w", err)
	}

	logger.Info("sync complete")
	return nil
}

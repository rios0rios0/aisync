package commands

import (
	"fmt"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

// SyncOptions holds the flags for the sync command.
type SyncOptions struct {
	DryRun         bool
	Preference     entities.SyncPreference
	SkipSecretScan bool
	SkipNDAScan    bool
}

// SyncCommand orchestrates a pull followed by a push.
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

// Execute runs the pull phase (sync repo update, external sources, shared
// files, bundles) and then the push phase, which reconciles personal files
// in both directions in a single pass, so every conflict is asked about
// exactly once, and then commits and pushes. Personal files are never
// overwritten without a prompt or an explicit --prefer.
func (c *SyncCommand) Execute(configPath, repoPath, commitMsg string, opts SyncOptions) error {
	logger.Info("starting sync: pull phase")
	pullOpts := PullOptions{
		DryRun:       opts.DryRun,
		Force:        true, // shared files from external sources apply without prompting
		SkipPersonal: true, // personal files are reconciled once, in the push phase
	}
	if err := c.pullCmd.Execute(configPath, repoPath, pullOpts); err != nil {
		return fmt.Errorf("pull phase failed: %w", err)
	}

	logger.Info("starting sync: push phase")
	pushOpts := PushOptions{
		DryRun:         opts.DryRun,
		Preference:     opts.Preference,
		Direction:      entities.SyncBoth,
		SkipFetch:      true, // the pull phase just updated the sync repo
		SkipSecretScan: opts.SkipSecretScan,
		SkipNDAScan:    opts.SkipNDAScan,
	}
	if err := c.pushCmd.Execute(configPath, repoPath, commitMsg, pushOpts); err != nil {
		return fmt.Errorf("push phase failed: %w", err)
	}

	logger.Info("sync complete")
	return nil
}

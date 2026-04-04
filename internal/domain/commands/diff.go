package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// DiffCommand previews changes without applying them.
type DiffCommand struct {
	configRepo  repositories.ConfigRepository
	sourceRepo  repositories.SourceRepository
	diffService repositories.DiffService
	formatter   entities.Formatter
}

// NewDiffCommand creates a new DiffCommand.
func NewDiffCommand(
	configRepo repositories.ConfigRepository,
	sourceRepo repositories.SourceRepository,
	diffService repositories.DiffService,
	formatter entities.Formatter,
) *DiffCommand {
	return &DiffCommand{
		configRepo:  configRepo,
		sourceRepo:  sourceRepo,
		diffService: diffService,
		formatter:   formatter,
	}
}

// DiffOptions holds the flags for the diff command.
type DiffOptions struct {
	SourceFilter string
	Personal     bool
	Shared       bool
	Summary      bool
	Reverse      bool
	Tool         string
}

// Execute computes and displays the diff.
func (c *DiffCommand) Execute(configPath, repoPath string, opts DiffOptions) error {
	config, err := c.configRepo.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	result := &entities.DiffResult{}

	if opts.Reverse {
		// Reverse mode: show what would be pushed (local changes relative
		// to the sync repo), swapping the perspective.
		local, diffErr := c.diffService.ComputeLocalDiff(config, repoPath)
		if diffErr == nil {
			result.LocalUncommitted = local
		}
	} else {
		// Compute shared changes (from external sources)
		if !opts.Personal {
			allFiles := make(map[string][]byte)
			for _, source := range config.Sources {
				if opts.SourceFilter != "" && source.Name != opts.SourceFilter {
					continue
				}
				fetched, fetchErr := c.sourceRepo.Fetch(&source, repositories.CacheHints{})
				if fetchErr != nil || fetched == nil {
					continue
				}
				for k, v := range fetched.Files {
					allFiles[k] = v
				}
			}

			shared, diffErr := c.diffService.ComputeSharedDiff(config, repoPath, allFiles)
			if diffErr == nil {
				result.SharedChanges = shared
			}
		}

		// Compute personal changes from other devices (incoming from sync repo).
		if !opts.Shared {
			personal, personalErr := c.diffService.ComputePersonalDiff(config, repoPath)
			if personalErr == nil {
				result.PersonalChanges = personal
			}
		}

		// Compute local uncommitted changes
		if !opts.Shared {
			local, diffErr := c.diffService.ComputeLocalDiff(config, repoPath)
			if diffErr == nil {
				result.LocalUncommitted = local
			}
		}
	}

	if !result.HasChanges() {
		fmt.Println("No changes detected.")
		return nil
	}

	if opts.Reverse {
		fmt.Println("(reverse: showing what would be pushed)")
	}

	if opts.Summary {
		printSummary(result, c.formatter)
	} else {
		printDetailed(result, opts.Tool, c.formatter)
	}

	return nil
}

func printSummary(result *entities.DiffResult, f entities.Formatter) {
	if len(result.SharedChanges) > 0 {
		fmt.Println(f.Bold("External sources:"))
		for _, ch := range result.SharedChanges {
			fmt.Printf("  %s %s\n", f.DiffSymbol(string(ch.Direction)), f.FilePath(ch.Path))
		}
	}
	if len(result.PersonalChanges) > 0 {
		fmt.Println(f.Bold("Personal (from other devices):"))
		for _, ch := range result.PersonalChanges {
			fmt.Printf("  %s %s\n", f.DiffSymbol(string(ch.Direction)), f.FilePath(ch.Path))
		}
	}
	if len(result.LocalUncommitted) > 0 {
		fmt.Println(f.Bold("Local uncommitted changes:"))
		for _, ch := range result.LocalUncommitted {
			fmt.Printf("  %s %s\n", ch.Direction, ch.Path)
		}
	}
}

func printDetailed(result *entities.DiffResult, tool string, f entities.Formatter) {
	if len(result.SharedChanges) > 0 {
		fmt.Println(f.Bold("External sources:"))
		for _, ch := range result.SharedChanges {
			printChange(ch, f)
			launchExternalTool(tool, ch)
		}
		fmt.Println()
	}
	if len(result.PersonalChanges) > 0 {
		fmt.Println(f.Bold("Personal (from other devices):"))
		for _, ch := range result.PersonalChanges {
			printChange(ch, f)
			launchExternalTool(tool, ch)
		}
		fmt.Println()
	}
	if len(result.LocalUncommitted) > 0 {
		fmt.Println(f.Bold("Local uncommitted changes:"))
		for _, ch := range result.LocalUncommitted {
			printChange(ch, f)
			launchExternalTool(tool, ch)
		}
		fmt.Println()
	}
}

func printChange(ch entities.FileChange, f entities.Formatter) {
	var details []string

	details = append(details, fmt.Sprintf("source: %s", ch.Source))

	switch ch.Direction {
	case entities.ChangeAdded:
		details = append(details, "new file")
		if ch.RemoteSize > 0 {
			details = append(details, formatSize(ch.RemoteSize))
		} else if ch.LocalSize > 0 {
			details = append(details, formatSize(ch.LocalSize))
		}
	case entities.ChangeModified:
		details = append(details, fmt.Sprintf("local: %s -> remote: %s (%+d B)",
			formatSize(ch.LocalSize), formatSize(ch.RemoteSize), ch.SizeDelta()))
		if ch.HasClockSkew() {
			details = append(details, "clock skew detected -- timestamps match but content differs, manual review recommended")
		} else if !ch.LocalTimestamp.IsZero() && !ch.RemoteTimestamp.IsZero() {
			if ch.IsRemoteNewer() {
				details = append(details, "remote is newer")
			} else {
				details = append(details, "local is newer")
			}
		}
	case entities.ChangeRemoved:
		details = append(details, "removed upstream")
	}

	if ch.Encrypted {
		details = append(details, "encrypted")
	}

	fmt.Printf("  %s %s\n", f.DiffSymbol(string(ch.Direction)), f.FilePath(ch.Path))
	fmt.Printf("      %s\n", f.Subtle(strings.Join(details, " | ")))
}

func formatSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
}

// launchExternalTool invokes an external diff tool for modified files that have
// both local and remote content available. Added/removed files are skipped.
func launchExternalTool(tool string, ch entities.FileChange) {
	if tool == "" {
		return
	}
	if ch.Direction != entities.ChangeModified {
		return
	}
	if ch.LocalContent == nil || ch.RemoteContent == nil {
		return
	}

	tmpDir, err := os.MkdirTemp("", "aisync-diff-*")
	if err != nil {
		logger.Warnf("failed to create temp dir for diff tool: %v", err)
		return
	}
	defer os.RemoveAll(tmpDir)

	base := filepath.Base(ch.Path)
	localFile := filepath.Join(tmpDir, "local-"+base)
	remoteFile := filepath.Join(tmpDir, "remote-"+base)

	if err := os.WriteFile(localFile, ch.LocalContent, 0644); err != nil {
		logger.Warnf("failed to write local temp file: %v", err)
		return
	}
	if err := os.WriteFile(remoteFile, ch.RemoteContent, 0644); err != nil {
		logger.Warnf("failed to write remote temp file: %v", err)
		return
	}

	cmd := exec.Command(tool, localFile, remoteFile) //nolint:gosec
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		// Many diff tools exit with code 1 when files differ; only warn on
		// unexpected failures.
		logger.Debugf("external diff tool exited: %v", err)
	}
}

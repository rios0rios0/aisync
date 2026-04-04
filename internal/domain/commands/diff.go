package commands

import (
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// DiffViewer displays diff results interactively (e.g., bubbletea viewport).
type DiffViewer interface {
	Show(result *entities.DiffResult, f entities.Formatter) error
}

// DiffCommand previews changes without applying them.
type DiffCommand struct {
	configRepo  repositories.ConfigRepository
	sourceRepo  repositories.SourceRepository
	diffService repositories.DiffService
	formatter   entities.Formatter
	viewer      DiffViewer
}

// NewDiffCommand creates a new DiffCommand.
func NewDiffCommand(
	configRepo repositories.ConfigRepository,
	sourceRepo repositories.SourceRepository,
	diffService repositories.DiffService,
	formatter entities.Formatter,
	viewer DiffViewer,
) *DiffCommand {
	return &DiffCommand{
		configRepo:  configRepo,
		sourceRepo:  sourceRepo,
		diffService: diffService,
		formatter:   formatter,
		viewer:      viewer,
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

	result := c.computeDiff(config, repoPath, opts)

	if !result.HasChanges() {
		fmt.Fprintln(os.Stdout, "No changes detected.")
		return nil
	}

	if opts.Reverse {
		fmt.Fprintln(os.Stdout, "(reverse: showing what would be pushed)")
	}

	c.renderOutput(result, opts)
	return nil
}

// computeDiff gathers all change data based on the diff options.
func (c *DiffCommand) computeDiff(config *entities.Config, repoPath string, opts DiffOptions) *entities.DiffResult {
	result := &entities.DiffResult{}

	if opts.Reverse {
		local, diffErr := c.diffService.ComputeLocalDiff(config, repoPath)
		if diffErr == nil {
			result.LocalUncommitted = local
		}
		return result
	}

	if !opts.Personal {
		result.SharedChanges = c.fetchSharedChanges(config, repoPath, opts.SourceFilter)
	}
	if !opts.Shared {
		if personal, err := c.diffService.ComputePersonalDiff(config, repoPath); err == nil {
			result.PersonalChanges = personal
		}
		if local, err := c.diffService.ComputeLocalDiff(config, repoPath); err == nil {
			result.LocalUncommitted = local
		}
	}
	return result
}

// fetchSharedChanges fetches files from external sources and computes the shared diff.
func (c *DiffCommand) fetchSharedChanges(config *entities.Config, repoPath, sourceFilter string) []entities.FileChange {
	allFiles := make(map[string][]byte)
	for _, source := range config.Sources {
		if sourceFilter != "" && source.Name != sourceFilter {
			continue
		}
		fetched, fetchErr := c.sourceRepo.Fetch(&source, repositories.CacheHints{})
		if fetchErr != nil || fetched == nil {
			continue
		}
		maps.Copy(allFiles, fetched.Files)
	}

	shared, diffErr := c.diffService.ComputeSharedDiff(config, repoPath, allFiles)
	if diffErr != nil {
		return nil
	}
	return shared
}

// renderOutput displays the diff result using the appropriate renderer.
func (c *DiffCommand) renderOutput(result *entities.DiffResult, opts DiffOptions) {
	if opts.Summary {
		printSummary(result, c.formatter)
		return
	}
	if c.viewer != nil && opts.Tool == "" {
		if err := c.viewer.Show(result, c.formatter); err == nil {
			return
		}
	}
	printDetailed(result, opts.Tool, c.formatter)
}

func printSummary(result *entities.DiffResult, f entities.Formatter) {
	if len(result.SharedChanges) > 0 {
		fmt.Fprintln(os.Stdout, f.Bold("External sources:"))
		for _, ch := range result.SharedChanges {
			fmt.Fprintf(os.Stdout, "  %s %s\n", f.DiffSymbol(string(ch.Direction)), f.FilePath(ch.Path))
		}
	}
	if len(result.PersonalChanges) > 0 {
		fmt.Fprintln(os.Stdout, f.Bold("Personal (from other devices):"))
		for _, ch := range result.PersonalChanges {
			fmt.Fprintf(os.Stdout, "  %s %s\n", f.DiffSymbol(string(ch.Direction)), f.FilePath(ch.Path))
		}
	}
	if len(result.LocalUncommitted) > 0 {
		fmt.Fprintln(os.Stdout, f.Bold("Local uncommitted changes:"))
		for _, ch := range result.LocalUncommitted {
			fmt.Fprintf(os.Stdout, "  %s %s\n", ch.Direction, ch.Path)
		}
	}
}

func printDetailed(result *entities.DiffResult, tool string, f entities.Formatter) {
	if len(result.SharedChanges) > 0 {
		fmt.Fprintln(os.Stdout, f.Bold("External sources:"))
		for _, ch := range result.SharedChanges {
			printChange(ch, f)
			launchExternalTool(tool, ch)
		}
		fmt.Fprintln(os.Stdout)
	}
	if len(result.PersonalChanges) > 0 {
		fmt.Fprintln(os.Stdout, f.Bold("Personal (from other devices):"))
		for _, ch := range result.PersonalChanges {
			printChange(ch, f)
			launchExternalTool(tool, ch)
		}
		fmt.Fprintln(os.Stdout)
	}
	if len(result.LocalUncommitted) > 0 {
		fmt.Fprintln(os.Stdout, f.Bold("Local uncommitted changes:"))
		for _, ch := range result.LocalUncommitted {
			printChange(ch, f)
			launchExternalTool(tool, ch)
		}
		fmt.Fprintln(os.Stdout)
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
			details = append(
				details,
				"clock skew detected -- timestamps match but content differs, manual review recommended",
			)
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

	fmt.Fprintf(os.Stdout, "  %s %s\n", f.DiffSymbol(string(ch.Direction)), f.FilePath(ch.Path))
	fmt.Fprintf(os.Stdout, "      %s\n", f.Subtle(strings.Join(details, " | ")))
}

const bytesPerKB = 1024

func formatSize(bytes int64) string {
	if bytes < bytesPerKB {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < bytesPerKB*bytesPerKB {
		return fmt.Sprintf("%.1f KB", float64(bytes)/bytesPerKB)
	}
	return fmt.Sprintf("%.1f MB", float64(bytes)/(bytesPerKB*bytesPerKB))
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

	if err = os.WriteFile(localFile, ch.LocalContent, 0600); err != nil {
		logger.Warnf("failed to write local temp file: %v", err)
		return
	}
	if err = os.WriteFile(remoteFile, ch.RemoteContent, 0600); err != nil {
		logger.Warnf("failed to write remote temp file: %v", err)
		return
	}

	cmd := exec.CommandContext(context.Background(), tool, localFile, remoteFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err = cmd.Run(); err != nil {
		// Many diff tools exit with code 1 when files differ; only warn on
		// unexpected failures.
		logger.Debugf("external diff tool exited: %v", err)
	}
}

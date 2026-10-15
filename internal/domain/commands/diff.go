package commands

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// DiffViewer pages long diff output interactively (e.g., a bubbletea viewport).
type DiffViewer interface {
	// ShowText pages content, or returns an error without showing anything
	// when the caller should print it instead (no terminal, or it fits).
	ShowText(content string) error
}

// DiffCommand previews changes without applying them: files arriving from
// external sources, and every personal file that differs between this device
// and the sync repo, with a unified diff for each, like `chezmoi diff`.
type DiffCommand struct {
	configRepo     repositories.ConfigRepository
	sourceRepo     repositories.SourceRepository
	diffService    repositories.DiffService
	personalSyncer *PersonalSyncer
	differ         repositories.TextDiffer
	formatter      entities.Formatter
	viewer         DiffViewer
}

// NewDiffCommand creates a new DiffCommand.
func NewDiffCommand(
	configRepo repositories.ConfigRepository,
	sourceRepo repositories.SourceRepository,
	diffService repositories.DiffService,
	personalSyncer *PersonalSyncer,
	differ repositories.TextDiffer,
	formatter entities.Formatter,
	viewer DiffViewer,
) *DiffCommand {
	return &DiffCommand{
		configRepo:     configRepo,
		sourceRepo:     sourceRepo,
		diffService:    diffService,
		personalSyncer: personalSyncer,
		differ:         differ,
		formatter:      formatter,
		viewer:         viewer,
	}
}

// DiffOptions holds the flags for the diff command.
type DiffOptions struct {
	SourceFilter string
	Personal     bool   // only personal files
	Shared       bool   // only files from external sources
	Summary      bool   // file names only, no content diff
	Reverse      bool   // only what a push would send (and conflicts)
	Tool         string // external diff tool to open each changed file pair with
}

// Execute computes and displays the diff.
func (c *DiffCommand) Execute(configPath, repoPath string, opts DiffOptions) error {
	config, err := c.configRepo.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	var shared []entities.FileChange
	if !opts.Personal && !opts.Reverse {
		shared = c.fetchSharedChanges(config, repoPath, opts.SourceFilter)
	}
	var personal []PersonalChange
	if !opts.Shared {
		personal = c.personalChanges(config, repoPath, opts.Reverse)
	}

	if len(shared) == 0 && len(personal) == 0 {
		fmt.Fprintln(os.Stdout, "No changes detected.")
		return nil
	}
	if opts.Reverse {
		fmt.Fprintln(os.Stdout, "(reverse: showing only what a push would send, plus conflicts)")
	}

	if opts.Tool != "" {
		c.openInTool(shared, personal, opts.Tool)
		return nil
	}

	var b strings.Builder
	c.render(&b, shared, personal, opts)
	if opts.Summary || c.viewer == nil || c.viewer.ShowText(b.String()) != nil {
		fmt.Fprint(os.Stdout, b.String())
	}
	return nil
}

// personalChanges lists the personal files that differ; in reverse mode only
// those a push would send, plus conflicts.
func (c *DiffCommand) personalChanges(config *entities.Config, repoPath string, reverse bool) []PersonalChange {
	changes := c.personalSyncer.Changes(config, repoPath)
	if !reverse {
		return changes
	}
	outgoing := changes[:0]
	for _, change := range changes {
		if change.Entry.Action.IsOutgoing() || change.Entry.Action == entities.SyncActionConflict {
			outgoing = append(outgoing, change)
		}
	}
	return outgoing
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

// render writes both sections: external sources, then personal files with
// when each side last changed and, unless summarizing, the colored diff.
func (c *DiffCommand) render(w io.Writer, shared []entities.FileChange, personal []PersonalChange, opts DiffOptions) {
	f := c.formatter
	if len(shared) > 0 {
		fmt.Fprintln(w, f.Bold("External sources:"))
		for _, change := range shared {
			writeChange(w, change, f)
		}
		fmt.Fprintln(w)
	}

	if len(personal) == 0 {
		return
	}
	fmt.Fprintln(w, f.Bold("Personal files (this device compared with the sync repo):"))
	for _, change := range personal {
		label, description := describeEntry(change.Entry)
		fmt.Fprintf(w, "  %-8s %s  %s\n", label, f.FilePath(change.Tool+"/"+change.Entry.Path), description)
		if opts.Summary {
			continue
		}
		fmt.Fprintf(w, "           %s\n", f.Subtle(describeTimestamps(entities.ConflictDetail{
			LocalModified:  change.LocalModified,
			RemoteModified: change.RemoteModified,
			RemoteSummary:  change.RemoteSummary,
		})))
		fmt.Fprint(w, c.unifiedDiff(change))
		fmt.Fprintln(w)
	}
}

// unifiedDiff renders the change as a colored unified diff oriented the way
// the change would travel: incoming changes read as this device becoming the
// repo version, outgoing changes and conflicts as the repo becoming this
// device's version.
func (c *DiffCommand) unifiedDiff(change PersonalChange) string {
	name := change.Tool + "/" + change.Entry.Path
	from, to := change.RemoteContent, change.LocalContent
	fromLabel, toLabel := "repo: "+name, "local: "+name
	if change.Entry.Action.IsIncoming() {
		from, to = change.LocalContent, change.RemoteContent
		fromLabel, toLabel = "local: "+name, "repo: "+name
	}

	if isBinary(from) || isBinary(to) {
		return "           (binary content, no text diff)\n"
	}
	return entities.ColorizeDiff(c.differ.Unified(fromLabel, toLabel, from, to), c.formatter)
}

// openInTool prints each change and opens the pairs that exist on both sides
// in the external diff tool.
func (c *DiffCommand) openInTool(shared []entities.FileChange, personal []PersonalChange, tool string) {
	for _, change := range shared {
		writeChange(os.Stdout, change, c.formatter)
		launchExternalTool(tool, change)
	}
	for _, change := range personal {
		label, description := describeEntry(change.Entry)
		fmt.Fprintf(os.Stdout, "  %-8s %s/%s  %s\n", label, change.Tool, change.Entry.Path, description)
		launchExternalTool(tool, entities.FileChange{
			Path:          change.Entry.Path,
			Direction:     entities.ChangeModified,
			LocalContent:  change.LocalContent,
			RemoteContent: change.RemoteContent,
		})
	}
}

// writeChange prints one external-source change with its details.
func writeChange(w io.Writer, ch entities.FileChange, f entities.Formatter) {
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
	case entities.ChangeRemoved:
		details = append(details, "removed upstream")
	}

	if ch.Encrypted {
		details = append(details, "encrypted")
	}

	fmt.Fprintf(w, "  %s %s\n", f.DiffSymbol(string(ch.Direction)), f.FilePath(ch.Path))
	fmt.Fprintf(w, "      %s\n", f.Subtle(strings.Join(details, " | ")))
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

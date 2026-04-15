package commands

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

const namespaceShared = "shared"

// knownSharedEntry records the source metadata for a file whose checksum matches
// content fetched from an external source.
type knownSharedEntry struct {
	sourceName string
	sourceRepo string
	branch     string
}

// MigrateCommand scans existing AI tool directories and moves files into
// the sync repo's shared/ or personal/ namespaces.
type MigrateCommand struct {
	configRepo   repositories.ConfigRepository
	manifestRepo repositories.ManifestRepository
	sourceRepo   repositories.SourceRepository
}

// NewMigrateCommand creates a new MigrateCommand.
func NewMigrateCommand(
	configRepo repositories.ConfigRepository,
	manifestRepo repositories.ManifestRepository,
	sourceRepo repositories.SourceRepository,
) *MigrateCommand {
	return &MigrateCommand{
		configRepo:   configRepo,
		manifestRepo: manifestRepo,
		sourceRepo:   sourceRepo,
	}
}

// migrateToolResult holds the per-tool migration outcome.
type migrateToolResult struct {
	migrated int
	matched  map[string]knownSharedEntry
}

// Execute scans tool directories for existing files and migrates them
// into the sync repo structure. When dryRun is true, it prints what would
// be migrated without modifying any files.
func (c *MigrateCommand) Execute(configPath, repoPath string, dryRun bool) error {
	config, err := c.configRepo.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	knownShared := c.buildKnownSharedMap(config)
	matchedSources := make(map[string]knownSharedEntry)
	totalMigrated := 0

	for toolName, tool := range config.Tools {
		if !tool.Enabled {
			continue
		}

		result := c.migrateTool(toolName, tool, repoPath, knownShared, dryRun)
		totalMigrated += result.migrated
		maps.Copy(matchedSources, result.matched)
	}

	c.printMigrationSummary(totalMigrated, dryRun, matchedSources)
	return nil
}

// buildKnownSharedMap fetches all external sources and builds a map of checksum
// to source metadata for classifying files during migration.
func (c *MigrateCommand) buildKnownSharedMap(config *entities.Config) map[string]knownSharedEntry {
	knownShared := make(map[string]knownSharedEntry)
	for _, source := range config.Sources {
		fetched, fetchErr := c.sourceRepo.Fetch(&source, repositories.CacheHints{})
		if fetchErr != nil || fetched == nil {
			logger.Debugf("skipping source %s during migrate: %v", source.Name, fetchErr)
			continue
		}
		for _, content := range fetched.Files {
			cs := checksumBytes(content)
			knownShared[cs] = knownSharedEntry{
				sourceName: source.Name,
				sourceRepo: source.Repo,
				branch:     source.Branch,
			}
		}
	}
	return knownShared
}

// migrateTool walks a single tool directory and classifies each file as shared or
// personal based on the known shared checksums. Returns the count of migrated files
// and any matched source entries.
func (c *MigrateCommand) migrateTool(
	toolName string,
	tool entities.Tool,
	repoPath string,
	knownShared map[string]knownSharedEntry,
	dryRun bool,
) migrateToolResult {
	result := migrateToolResult{matched: make(map[string]knownSharedEntry)}

	toolDir := ExpandHome(tool.Path)
	if _, err := os.Stat(toolDir); err != nil {
		return result
	}

	hostname, _ := os.Hostname()
	manifest := entities.NewManifest("0.1.0", hostname)

	err := filepath.WalkDir(toolDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		n, matched := c.migrateFile(
			path,
			toolDir,
			toolName,
			repoPath,
			knownShared,
			manifest,
			dryRun,
			tool.ExtraAllowlist,
		)
		result.migrated += n
		maps.Copy(result.matched, matched)
		return nil
	})

	if err != nil {
		logger.Warnf("error scanning %s: %v", toolDir, err)
		return result
	}

	if result.migrated > 0 {
		if !dryRun {
			if saveErr := c.manifestRepo.Save(toolDir, manifest); saveErr != nil {
				logger.Warnf("failed to save manifest for %s: %v", toolName, saveErr)
			}
		}
		logger.Infof("migrated %d files from %s to personal/%s/", result.migrated, toolDir, toolName)
	}

	return result
}

// migrateFile classifies and migrates a single file. Returns 1 if the file was
// migrated, 0 otherwise, and any matched source entry.
func (c *MigrateCommand) migrateFile(
	path, toolDir, toolName, repoPath string,
	knownShared map[string]knownSharedEntry,
	manifest *entities.Manifest,
	dryRun bool,
	extraAllowlist []string,
) (int, map[string]knownSharedEntry) {
	matched := make(map[string]knownSharedEntry)

	relPath, _ := filepath.Rel(toolDir, path)
	if relPath == ".aisync-manifest.json" {
		return 0, matched
	}

	if !entities.IsSyncable(toolName, relPath, extraAllowlist) {
		return 0, matched
	}

	content, readErr := os.ReadFile(path)
	if readErr != nil {
		return 0, matched
	}

	checksum := checksumBytes(content)
	namespace := "personal"
	sourceName := "personal"
	if entry, ok := knownShared[checksum]; ok {
		namespace = namespaceShared
		sourceName = entry.sourceName
		matched[entry.sourceName] = entry
	}

	if dryRun {
		fmt.Fprintf(os.Stdout, "  [%s] %s/%s (source: %s)\n", namespace, toolName, relPath, sourceName)
		return 1, matched
	}

	destDir := filepath.Clean(filepath.Join(repoPath, namespace, toolName, filepath.Dir(relPath)))
	destPath := filepath.Clean(filepath.Join(repoPath, namespace, toolName, relPath))

	if err := os.MkdirAll(destDir, 0700); err != nil {
		logger.Warnf("failed to create %s: %v", destDir, err)
		return 0, matched
	}

	if err := os.WriteFile(destPath, content, 0600); err != nil { //nolint:gosec // destPath is filepath.Clean'd above
		logger.Warnf("failed to write %s: %v", destPath, err)
		return 0, matched
	}

	manifest.SetFile(relPath, sourceName, namespace, checksum)
	return 1, matched
}

// printMigrationSummary prints the final migration summary and source suggestions.
func (c *MigrateCommand) printMigrationSummary(
	totalMigrated int,
	dryRun bool,
	matchedSources map[string]knownSharedEntry,
) {
	switch {
	case totalMigrated == 0:
		fmt.Fprintln(os.Stdout, "No files found to migrate.")
	case dryRun:
		fmt.Fprintf(os.Stdout,
			"\n[dry-run] Would migrate %d files into shared/ and personal/ namespaces.\n",
			totalMigrated,
		)
	default:
		fmt.Fprintf(
			os.Stdout,
			"Migration complete: %d files classified into shared/ and personal/ namespaces.\n",
			totalMigrated,
		)
		fmt.Fprintln(os.Stdout, "Files matching external sources were placed in shared/; the rest in personal/.")
	}

	if len(matchedSources) > 0 {
		fmt.Fprintln(os.Stdout)
		for _, entry := range matchedSources {
			fmt.Fprintf(os.Stdout, "Detected files from source '%s'. To configure it:\n", entry.sourceName)
			fmt.Fprintf(os.Stdout,
				"  aisync source add %s --source-repo %s --branch %s\n",
				entry.sourceName,
				entry.sourceRepo,
				entry.branch,
			)
		}
	}
}

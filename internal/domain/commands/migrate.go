package commands

import (
	"fmt"
	"os"
	"path/filepath"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

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

// Execute scans tool directories for existing files and migrates them
// into the sync repo structure.
func (c *MigrateCommand) Execute(configPath, repoPath string) error {
	config, err := c.configRepo.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Build a map of known shared content by fetching all external sources
	// and computing their checksums.
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

	// Track which sources were matched during migration for suggestions.
	matchedSources := make(map[string]knownSharedEntry)

	totalMigrated := 0

	for toolName, tool := range config.Tools {
		if !tool.Enabled {
			continue
		}

		toolDir := ExpandHome(tool.Path)
		if _, err := os.Stat(toolDir); err != nil {
			continue
		}

		hostname, _ := os.Hostname()
		manifest := entities.NewManifest("0.1.0", hostname)

		migrated := 0
		err := filepath.WalkDir(toolDir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}

			if entities.IsDenied(path) {
				return nil
			}

			relPath, _ := filepath.Rel(toolDir, path)

			// Skip the manifest file itself
			if relPath == ".aisync-manifest.json" {
				return nil
			}

			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}

			checksum := checksumBytes(content)

			// Classify: if the file checksum matches known external source
			// content, place it in shared/; otherwise in personal/.
			namespace := "personal"
			sourceName := "personal"
			if entry, ok := knownShared[checksum]; ok {
				namespace = "shared"
				sourceName = entry.sourceName
				matchedSources[entry.sourceName] = entry
			}

			destDir := filepath.Join(repoPath, namespace, toolName, filepath.Dir(relPath))
			destPath := filepath.Join(repoPath, namespace, toolName, relPath)

			if err := os.MkdirAll(destDir, 0755); err != nil {
				logger.Warnf("failed to create %s: %v", destDir, err)
				return nil
			}

			if err := os.WriteFile(destPath, content, 0644); err != nil {
				logger.Warnf("failed to write %s: %v", destPath, err)
				return nil
			}

			manifest.SetFile(relPath, sourceName, namespace, checksum)
			migrated++

			return nil
		})

		if err != nil {
			logger.Warnf("error scanning %s: %v", toolDir, err)
			continue
		}

		if migrated > 0 {
			if err := c.manifestRepo.Save(toolDir, manifest); err != nil {
				logger.Warnf("failed to save manifest for %s: %v", toolName, err)
			}
			logger.Infof("migrated %d files from %s to personal/%s/", migrated, toolDir, toolName)
			totalMigrated += migrated
		}
	}

	if totalMigrated == 0 {
		fmt.Println("No files found to migrate.")
	} else {
		fmt.Printf("Migration complete: %d files classified into shared/ and personal/ namespaces.\n", totalMigrated)
		fmt.Println("Files matching external sources were placed in shared/; the rest in personal/.")
	}

	// Suggest source configuration commands for matched sources.
	if len(matchedSources) > 0 {
		fmt.Println()
		for _, entry := range matchedSources {
			fmt.Printf("Detected files from source '%s'. To configure it:\n", entry.sourceName)
			fmt.Printf("  aisync source add %s --source-repo %s --branch %s\n", entry.sourceName, entry.sourceRepo, entry.branch)
		}
	}

	return nil
}

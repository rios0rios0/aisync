package commands

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// PullOptions holds the flags that modify pull behavior.
type PullOptions struct {
	DryRun       bool
	Force        bool
	SourceFilter string
}

// PullCommand fetches from external sources, merges single-file configs,
// applies personal overrides, and atomically writes files to AI tool directories.
type PullCommand struct {
	configRepo        repositories.ConfigRepository
	stateRepo         repositories.StateRepository
	sourceRepo        repositories.SourceRepository
	manifestRepo      repositories.ManifestRepository
	gitRepo           repositories.GitRepository
	encryptionService repositories.EncryptionService
	conflictDetector  repositories.ConflictDetector
	hooksMerger       repositories.Merger
	settingsMerger    repositories.Merger
	sectionMerger     repositories.Merger
	applyService      repositories.ApplyService
	promptService     repositories.PromptService
}

// NewPullCommand creates a new PullCommand.
func NewPullCommand(
	configRepo repositories.ConfigRepository,
	stateRepo repositories.StateRepository,
	sourceRepo repositories.SourceRepository,
	manifestRepo repositories.ManifestRepository,
	gitRepo repositories.GitRepository,
	encryptionService repositories.EncryptionService,
	conflictDetector repositories.ConflictDetector,
	hooksMerger repositories.Merger,
	settingsMerger repositories.Merger,
	sectionMerger repositories.Merger,
	applyService repositories.ApplyService,
	promptService repositories.PromptService,
) *PullCommand {
	return &PullCommand{
		configRepo:        configRepo,
		stateRepo:         stateRepo,
		sourceRepo:        sourceRepo,
		manifestRepo:      manifestRepo,
		gitRepo:           gitRepo,
		encryptionService: encryptionService,
		conflictDetector:  conflictDetector,
		hooksMerger:       hooksMerger,
		settingsMerger:    settingsMerger,
		sectionMerger:     sectionMerger,
		applyService:      applyService,
		promptService:     promptService,
	}
}

// fileEntry tracks a single file's content, provenance, and checksum.
type fileEntry struct {
	content  []byte
	source   string
	checksum string
}

// Execute pulls files from configured external sources, merges single-file
// configs, applies personal overrides, detects deletions, and atomically
// writes files to AI tool directories.
func (c *PullCommand) Execute(configPath, repoPath string, opts PullOptions) error {
	config, err := c.configRepo.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Set hooks exclude rules from config on the hooks merger.
	if len(config.HooksExclude) > 0 {
		if ea, ok := c.hooksMerger.(repositories.ExcludeAware); ok {
			ea.SetExcludes(config.HooksExclude)
		}
	}

	if len(config.Sources) == 0 {
		fmt.Println("No external sources configured. Add one with: aisync source add")
		return nil
	}

	// Step 1: Pull personal changes from the sync repo (other devices).
	if err := c.pullGitRepo(repoPath); err != nil {
		logger.Warnf("git pull skipped: %v", err)
	}

	// Step 2: Recover any incomplete atomic apply from a previous run.
	if err := c.applyService.Recover(); err != nil {
		logger.Warnf("failed to recover incomplete apply: %v", err)
	}

	// Step 3: Load state for ETag caching.
	state := c.loadOrCreateState(repoPath)

	// Step 4: Fetch files from all configured sources in config order.
	// Snapshot old ETags before fetching so we can detect force-push scenarios.
	oldETags := make(map[string]string)
	for _, source := range config.Sources {
		if etag := state.GetETag(source.Name); etag != "" {
			oldETags[source.Name] = etag
		}
	}

	allFiles, sourceFileMap := c.fetchSources(config, state, opts.SourceFilter)

	// Step 5: Save updated ETags back to state.
	if err := c.stateRepo.Save(repoPath, state); err != nil {
		logger.Warnf("failed to save state after fetching sources: %v", err)
	}

	if len(allFiles) == 0 {
		fmt.Println("All sources are up to date.")
		return nil
	}

	// Step 5b: Per-file checksum verification against previous sync repo contents.
	// Warn when a file's content changed but the source ETag did not, which may
	// indicate a force-push or silent upstream modification.
	if err := c.verifyFileChecksums(repoPath, allFiles, oldETags, state, opts.Force); err != nil {
		return err
	}

	// Step 6: Write fetched files to the sync repo shared/ directory.
	c.writeToSyncRepo(repoPath, allFiles)

	// Step 7: Load encrypt patterns for personal file decryption.
	encryptPatterns := c.loadEncryptPatterns(repoPath)

	// Step 8: Apply files to each enabled AI tool directory.
	for toolName, tool := range config.Tools {
		if !tool.Enabled {
			continue
		}

		if err := c.applyToToolDir(
			config, repoPath, toolName, tool, allFiles, sourceFileMap,
			encryptPatterns, opts,
		); err != nil {
			logger.Warnf("failed to apply to tool %s: %v", toolName, err)
		}
	}

	// Step 9: Update state timestamps after successful pull.
	state.LastPull = time.Now()
	hostname, _ := os.Hostname()
	if device := state.FindDevice(hostname); device != nil {
		device.LastSync = time.Now()
	}
	if err := c.stateRepo.Save(repoPath, state); err != nil {
		logger.Warnf("failed to update state after pull: %v", err)
	}

	fmt.Println("Pull complete.")
	return nil
}

// pullGitRepo opens the sync repo and pulls the latest changes from remote.
func (c *PullCommand) pullGitRepo(repoPath string) error {
	if err := c.gitRepo.Open(repoPath); err != nil {
		return fmt.Errorf("failed to open git repo: %w", err)
	}

	if !c.gitRepo.HasRemote() {
		return fmt.Errorf("no remote configured")
	}

	if err := c.gitRepo.Pull(); err != nil {
		return fmt.Errorf("git pull failed: %w", err)
	}

	logger.Info("pulled latest changes from sync repo")
	return nil
}

// loadOrCreateState loads the existing state or creates a new one.
func (c *PullCommand) loadOrCreateState(repoPath string) *entities.State {
	if c.stateRepo.Exists(repoPath) {
		state, err := c.stateRepo.Load(repoPath)
		if err == nil {
			return state
		}
		logger.Warnf("failed to load state, creating new: %v", err)
	}

	hostname, _ := os.Hostname()
	return entities.NewState(hostname)
}

// fetchSources iterates over sources in config order, fetches files, and
// returns the merged file map plus a per-file per-source map for merge
// strategies. Last source wins on conflict; conflicts are warned.
func (c *PullCommand) fetchSources(
	config *entities.Config,
	state *entities.State,
	sourceFilter string,
) (map[string]fileEntry, map[string]map[string][]byte) {
	allFiles := make(map[string]fileEntry)
	// sourceFileMap tracks per-source content for single-file merge strategies.
	// Key: relative path, Value: map of source name -> content.
	sourceFileMap := make(map[string]map[string][]byte)

	for _, source := range config.Sources {
		if sourceFilter != "" && source.Name != sourceFilter {
			continue
		}

		logger.Infof("fetching source '%s' (%s@%s)", source.Name, source.Repo, source.Branch)

		hints := repositories.CacheHints{
			ETag:         state.GetETag(source.Name),
			LastModified: state.GetLastModified(source.Name),
		}
		result, fetchErr := c.sourceRepo.Fetch(&source, hints)
		if fetchErr != nil {
			logger.Warnf("failed to fetch source '%s': %v", source.Name, fetchErr)
			continue
		}
		if result == nil {
			logger.Infof("source '%s' is up to date (304 Not Modified)", source.Name)
			continue
		}

		// Store returned cache headers for future conditional requests.
		if result.ETag != "" {
			state.SetETag(source.Name, result.ETag)
		}
		if result.LastModified != "" {
			state.SetLastModified(source.Name, result.LastModified)
		}

		for relPath, content := range result.Files {
			// Warn when two sources map a file to the same target.
			if existing, exists := allFiles[relPath]; exists {
				logger.Warnf(
					"file '%s' provided by source '%s' overwrites source '%s' (last source wins)",
					relPath, source.Name, existing.source,
				)
			}

			allFiles[relPath] = fileEntry{
				content:  content,
				source:   source.Name,
				checksum: checksumBytes(content),
			}

			// Track per-source content for merge strategies.
			if sourceFileMap[relPath] == nil {
				sourceFileMap[relPath] = make(map[string][]byte)
			}
			sourceFileMap[relPath][source.Name] = content
		}

		logger.Infof("source '%s': fetched %d files", source.Name, len(result.Files))
	}

	return allFiles, sourceFileMap
}

// verifyFileChecksums compares each fetched file against the existing copy in
// the sync repo. If a file's content changed but the source's ETag did not
// change (the new ETag equals the old cached ETag), it warns that the source
// may have been force-pushed or silently modified.
func (c *PullCommand) verifyFileChecksums(
	repoPath string,
	allFiles map[string]fileEntry,
	oldETags map[string]string,
	state *entities.State,
	force bool,
) error {
	for relPath, entry := range allFiles {
		existingPath := filepath.Join(repoPath, relPath)
		existingContent, err := os.ReadFile(existingPath)
		if err != nil {
			continue
		}

		existingChecksum := checksumBytes(existingContent)
		if existingChecksum == entry.checksum {
			continue
		}

		oldETag, hadOldETag := oldETags[entry.source]
		if !hadOldETag {
			continue
		}

		newETag := state.GetETag(entry.source)
		if oldETag == newETag {
			logger.Warnf(
				"file '%s' changed unexpectedly — source '%s' may have been force-pushed (ETag unchanged)",
				relPath, entry.source,
			)
			if !force {
				msg := fmt.Sprintf("Source '%s' may have been force-pushed. Continue?", entry.source)
				if !c.promptService.PromptConfirmation(msg) {
					return fmt.Errorf("aborted: suspected force-push on source '%s'", entry.source)
				}
			}
		}
	}
	return nil
}

// writeToSyncRepo writes all fetched files to the sync repo shared/ directory,
// skipping files that are unchanged.
func (c *PullCommand) writeToSyncRepo(repoPath string, allFiles map[string]fileEntry) {
	applied := 0
	skipped := 0

	for relPath, entry := range allFiles {
		destPath := filepath.Join(repoPath, relPath)
		destDir := filepath.Dir(destPath)

		if err := os.MkdirAll(destDir, 0755); err != nil {
			logger.Warnf("failed to create directory %s: %v", destDir, err)
			continue
		}

		existing, readErr := os.ReadFile(destPath)
		if readErr == nil && checksumBytes(existing) == entry.checksum {
			skipped++
			continue
		}

		if err := os.WriteFile(destPath, entry.content, 0644); err != nil {
			logger.Warnf("failed to write %s: %v", destPath, err)
			continue
		}
		applied++
	}

	logger.Infof("sync repo: applied %d files, skipped %d unchanged", applied, skipped)
}

// applyToToolDir applies shared and personal files to a single AI tool directory.
// It handles merge strategies, personal file precedence, deletion detection,
// dry-run preview, force/confirmation prompts, and atomic apply via journal.
func (c *PullCommand) applyToToolDir(
	config *entities.Config,
	repoPath, toolName string,
	tool entities.Tool,
	allFiles map[string]fileEntry,
	sourceFileMap map[string]map[string][]byte,
	encryptPatterns *entities.EncryptPatterns,
	opts PullOptions,
) error {
	toolDir := ExpandHome(tool.Path)
	prefix := "shared/" + toolName + "/"
	hostname, _ := os.Hostname()

	// Load existing manifest for deletion detection.
	var oldManifest *entities.Manifest
	if c.manifestRepo.Exists(toolDir) {
		loaded, err := c.manifestRepo.Load(toolDir)
		if err == nil {
			oldManifest = loaded
		}
	}

	manifest := entities.NewManifest("0.1.0", hostname)

	// Collect files destined for this tool, respecting merge strategies.
	pendingFiles := make(map[string][]byte)

	for relPath, entry := range allFiles {
		if !strings.HasPrefix(relPath, prefix) {
			continue
		}

		localRel := strings.TrimPrefix(relPath, prefix)
		destPath := filepath.Join(toolDir, localRel)

		if entities.IsDenied(destPath) {
			continue
		}

		// Determine if this file needs a merge strategy.
		content, mergeErr := c.applyMergeStrategy(
			config, repoPath, toolName, localRel, relPath,
			entry, sourceFileMap, encryptPatterns,
		)
		if mergeErr != nil {
			logger.Warnf("merge failed for %s: %v", relPath, mergeErr)
			content = entry.content
		}

		// Check for personal file override from the sync repo.
		personalContent := c.readPersonalFile(
			repoPath, toolName, localRel, encryptPatterns, config,
		)
		if personalContent != nil {
			logger.Warnf("personal file shadows shared file: %s/%s", toolName, localRel)
			content = personalContent
		}

		// Recency check: warn if local file differs from incoming.
		if !opts.Force {
			if existing, readErr := os.ReadFile(destPath); readErr == nil {
				if checksumBytes(existing) != checksumBytes(content) {
					msg := fmt.Sprintf("Local file '%s' differs from incoming. Overwrite?", localRel)
					if !c.promptService.PromptConfirmation(msg) {
						logger.Infof("kept local version of '%s'", localRel)
						continue
					}
				}
			}
		}

		pendingFiles[destPath] = content
		manifest.SetFile(localRel, entry.source, "shared", checksumBytes(content))
	}

	// Also apply personal-only files (files in personal/ that have no shared counterpart).
	c.applyPersonalOnlyFiles(
		repoPath, toolName, toolDir, prefix, allFiles,
		pendingFiles, manifest, encryptPatterns, config,
	)

	// Conflict detection: compare incoming personal files against the local tool dir.
	// If both local and incoming changed since last sync, prompt the user to resolve.
	if c.conflictDetector != nil && oldManifest != nil {
		incomingPersonal := c.collectIncomingPersonalFiles(repoPath, toolName, encryptPatterns, config)
		if len(incomingPersonal) > 0 {
			conflicts, detectErr := c.conflictDetector.DetectConflicts(toolDir, incomingPersonal, oldManifest, hostname)
			if detectErr != nil {
				logger.Warnf("conflict detection failed for %s: %v", toolName, detectErr)
			} else if len(conflicts) > 0 {
				resolved := c.resolveConflicts(toolDir, conflicts, opts.Force)
				// Apply resolved remote choices to pending files.
				for _, rc := range resolved {
					destPath := filepath.Join(toolDir, rc.Path)
					pendingFiles[destPath] = rc.RemoteContent
					manifest.SetFile(rc.Path, "personal", "personal", checksumBytes(rc.RemoteContent))
				}
			}
		}
	}

	// Deletion detection: files in old manifest but not in the new set.
	deletions := c.detectDeletions(oldManifest, manifest, toolDir)

	if len(pendingFiles) == 0 && len(deletions) == 0 {
		logger.Infof("tool %s: no changes", toolName)
		return nil
	}

	// Dry-run mode: print what would change and return.
	if opts.DryRun {
		c.printDryRun(toolName, pendingFiles, deletions, toolDir)
		return nil
	}

	// Confirmation prompt when not forced.
	if !opts.Force {
		c.printDiffSummary(toolName, pendingFiles, deletions, toolDir)
		for {
			action := c.promptService.PromptToolAction(toolName)
			switch action {
			case "skip":
				fmt.Printf("Skipped tool %s.\n", toolName)
				return nil
			case "abort":
				return fmt.Errorf("aborted by user")
			case "diff":
				c.printDryRun(toolName, pendingFiles, deletions, toolDir)
				continue
			case "apply":
				// Break out of the loop to proceed with apply.
			}
			break
		}
	}

	// Per-file skip: allow the user to exclude individual files.
	if !opts.Force && len(pendingFiles) > 1 {
		for destPath := range pendingFiles {
			relPath, _ := filepath.Rel(toolDir, destPath)
			action := c.promptService.PromptFileAction(relPath, "incoming")
			if action == "skip" {
				delete(pendingFiles, destPath)
				logger.Infof("skipped file '%s' (user choice)", relPath)
			}
		}
	}

	// Atomic apply: stage files, then move to final destinations.
	if len(pendingFiles) > 0 {
		journal, stageErr := c.applyService.Stage(pendingFiles)
		if stageErr != nil {
			return fmt.Errorf("failed to stage files for %s: %w", toolName, stageErr)
		}

		if applyErr := c.applyService.Apply(journal); applyErr != nil {
			return fmt.Errorf("failed to apply staged files for %s: %w", toolName, applyErr)
		}
	}

	// Process deletions.
	for _, delPath := range deletions {
		if err := os.Remove(delPath); err != nil && !os.IsNotExist(err) {
			logger.Warnf("failed to delete %s: %v", delPath, err)
		} else {
			logger.Infof("deleted: %s", delPath)
		}
	}

	// Save updated manifest.
	if err := c.manifestRepo.Save(toolDir, manifest); err != nil {
		logger.Warnf("failed to save manifest for %s: %v", toolName, err)
	}

	logger.Infof("tool %s: applied %d files, deleted %d files", toolName, len(pendingFiles), len(deletions))
	return nil
}

// applyMergeStrategy detects single-file merge targets by filename and routes
// to the appropriate merger. Returns the merged content or the original content
// if no merge strategy applies.
func (c *PullCommand) applyMergeStrategy(
	config *entities.Config,
	repoPath, toolName, localRel, relPath string,
	entry fileEntry,
	sourceFileMap map[string]map[string][]byte,
	encryptPatterns *entities.EncryptPatterns,
) ([]byte, error) {
	baseName := filepath.Base(localRel)
	merger := c.selectMerger(baseName)
	if merger == nil {
		// No merge strategy: direct overwrite.
		return entry.content, nil
	}

	// Collect content from each source in config order.
	sharedSources := c.collectSourceContents(config, relPath, sourceFileMap)

	// Check if personal/ has the same file.
	personalContent := c.readPersonalFile(repoPath, toolName, localRel, encryptPatterns, config)

	var personal []byte
	if personalContent != nil {
		personal = personalContent
	}

	merged, err := merger.Merge(sharedSources, personal)
	if err != nil {
		return nil, fmt.Errorf("merge strategy failed for %s: %w", localRel, err)
	}

	return merged, nil
}

// selectMerger returns the appropriate merger for a given filename, or nil
// if the file should be directly overwritten.
func (c *PullCommand) selectMerger(baseName string) repositories.Merger {
	if strings.HasSuffix(baseName, "hooks.json") {
		return c.hooksMerger
	}
	if strings.HasSuffix(baseName, "settings.json") {
		return c.settingsMerger
	}
	if baseName == "CLAUDE.md" || baseName == "AGENTS.md" {
		return c.sectionMerger
	}
	return nil
}

// collectSourceContents gathers content from each source for a given relPath,
// preserving config order for deterministic merging.
func (c *PullCommand) collectSourceContents(
	config *entities.Config,
	relPath string,
	sourceFileMap map[string]map[string][]byte,
) [][]byte {
	sourcesMap := sourceFileMap[relPath]
	if sourcesMap == nil {
		return nil
	}

	var contents [][]byte
	for _, source := range config.Sources {
		if content, ok := sourcesMap[source.Name]; ok {
			contents = append(contents, content)
		}
	}
	return contents
}

// readPersonalFile reads a personal file from the sync repo for the given tool
// and relative path. If the file matches encrypt patterns, it reads the .age
// version and decrypts it.
func (c *PullCommand) readPersonalFile(
	repoPath, toolName, localRel string,
	encryptPatterns *entities.EncryptPatterns,
	config *entities.Config,
) []byte {
	personalPath := filepath.Join(repoPath, "personal", toolName, localRel)

	// Check for encrypted version first.
	if encryptPatterns.Matches(localRel) && config.Encryption.Identity != "" {
		encryptedPath := personalPath + ".age"
		encrypted, err := os.ReadFile(encryptedPath)
		if err == nil {
			identityPath := ExpandHome(config.Encryption.Identity)
			decrypted, decErr := c.encryptionService.Decrypt(encrypted, identityPath)
			if decErr != nil {
				logger.Warnf("failed to decrypt personal file %s: %v", encryptedPath, decErr)
				return nil
			}
			return decrypted
		}
	}

	content, err := os.ReadFile(personalPath)
	if err != nil {
		return nil
	}
	return content
}

// applyPersonalOnlyFiles finds files in personal/<tool>/ that have no shared
// counterpart and adds them to the pending files and manifest.
func (c *PullCommand) applyPersonalOnlyFiles(
	repoPath, toolName, toolDir, sharedPrefix string,
	allFiles map[string]fileEntry,
	pendingFiles map[string][]byte,
	manifest *entities.Manifest,
	encryptPatterns *entities.EncryptPatterns,
	config *entities.Config,
) {
	personalDir := filepath.Join(repoPath, "personal", toolName)
	if _, err := os.Stat(personalDir); os.IsNotExist(err) {
		return
	}

	_ = filepath.Walk(personalDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(personalDir, path)
		if err != nil {
			return nil
		}

		// Skip .age files; they are handled via the decrypt path.
		if strings.HasSuffix(relPath, ".age") {
			return nil
		}

		// Check if a shared version already exists (already handled above).
		sharedKey := sharedPrefix + relPath
		if _, exists := allFiles[sharedKey]; exists {
			return nil
		}

		destPath := filepath.Join(toolDir, relPath)
		if entities.IsDenied(destPath) {
			return nil
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			logger.Warnf("failed to read personal file %s: %v", path, readErr)
			return nil
		}

		// Decrypt if this file matches encrypt patterns and an encrypted version exists.
		if encryptPatterns.Matches(relPath) && config.Encryption.Identity != "" {
			encryptedPath := path + ".age"
			encrypted, encErr := os.ReadFile(encryptedPath)
			if encErr == nil {
				identityPath := ExpandHome(config.Encryption.Identity)
				decrypted, decErr := c.encryptionService.Decrypt(encrypted, identityPath)
				if decErr == nil {
					content = decrypted
				} else {
					logger.Warnf("failed to decrypt %s: %v", encryptedPath, decErr)
				}
			}
		}

		pendingFiles[destPath] = content
		manifest.SetFile(relPath, "personal", "personal", checksumBytes(content))
		return nil
	})
}

// detectDeletions compares the old manifest against the new manifest and returns
// the absolute paths of files that should be deleted (present in old, absent in new).
func (c *PullCommand) detectDeletions(
	oldManifest, newManifest *entities.Manifest,
	toolDir string,
) []string {
	if oldManifest == nil {
		return nil
	}

	var deletions []string
	for relPath := range oldManifest.Files {
		if _, exists := newManifest.Files[relPath]; !exists {
			absPath := filepath.Join(toolDir, relPath)
			if _, statErr := os.Stat(absPath); statErr == nil {
				deletions = append(deletions, absPath)
			}
		}
	}
	return deletions
}

// printDryRun displays what would change without applying anything.
func (c *PullCommand) printDryRun(
	toolName string,
	pendingFiles map[string][]byte,
	deletions []string,
	toolDir string,
) {
	fmt.Printf("[dry-run] Tool %s:\n", toolName)
	for destPath, content := range pendingFiles {
		relPath, _ := filepath.Rel(toolDir, destPath)
		existing, err := os.ReadFile(destPath)
		if err != nil {
			fmt.Printf("  + %s (%s, new)\n", relPath, formatSize(int64(len(content))))
		} else if checksumBytes(existing) != checksumBytes(content) {
			detail := fmt.Sprintf("%s → %s", formatSize(int64(len(existing))), formatSize(int64(len(content))))
			oldLines := bytes.Count(existing, []byte("\n"))
			newLines := bytes.Count(content, []byte("\n"))
			if delta := newLines - oldLines; delta != 0 {
				detail += fmt.Sprintf(", %+d lines", delta)
			}
			fmt.Printf("  ~ %s (%s)\n", relPath, detail)
		} else {
			fmt.Printf("  = %s (unchanged)\n", relPath)
		}
	}
	for _, delPath := range deletions {
		relPath, _ := filepath.Rel(toolDir, delPath)
		fmt.Printf("  - %s (deleted)\n", relPath)
	}
}

// printDiffSummary displays a compact summary of pending changes for confirmation.
func (c *PullCommand) printDiffSummary(
	toolName string,
	pendingFiles map[string][]byte,
	deletions []string,
	toolDir string,
) {
	added := 0
	modified := 0
	unchanged := 0

	for destPath, content := range pendingFiles {
		existing, err := os.ReadFile(destPath)
		if err != nil {
			added++
		} else if checksumBytes(existing) != checksumBytes(content) {
			modified++
		} else {
			unchanged++
		}
	}

	fmt.Printf("Tool %s: %d new, %d modified, %d unchanged, %d deletions\n",
		toolName, added, modified, unchanged, len(deletions))
}

// loadEncryptPatterns reads the .aisyncencrypt file from the sync repo root.
func (c *PullCommand) loadEncryptPatterns(repoPath string) *entities.EncryptPatterns {
	encryptPath := filepath.Join(repoPath, ".aisyncencrypt")
	content, err := os.ReadFile(encryptPath)
	if err != nil {
		return entities.ParseEncryptPatterns([]byte{})
	}
	return entities.ParseEncryptPatterns(content)
}

// checksumBytes computes a sha256 checksum string for the given data.
func checksumBytes(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", h)
}

// collectIncomingPersonalFiles reads all personal files for a tool from the sync
// repo (which may include files pushed by other devices). Returns a map of
// relative path -> file content.
func (c *PullCommand) collectIncomingPersonalFiles(
	repoPath, toolName string,
	encryptPatterns *entities.EncryptPatterns,
	config *entities.Config,
) map[string][]byte {
	personalDir := filepath.Join(repoPath, "personal", toolName)
	if _, err := os.Stat(personalDir); os.IsNotExist(err) {
		return nil
	}

	incoming := make(map[string][]byte)
	_ = filepath.Walk(personalDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(personalDir, path)
		if err != nil {
			return nil
		}

		// Skip .age files; they are handled via the decrypt path.
		if strings.HasSuffix(relPath, ".age") {
			return nil
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		// Decrypt if needed.
		if encryptPatterns.Matches(relPath) && config.Encryption.Identity != "" {
			encryptedPath := path + ".age"
			encrypted, encErr := os.ReadFile(encryptedPath)
			if encErr == nil {
				identityPath := ExpandHome(config.Encryption.Identity)
				decrypted, decErr := c.encryptionService.Decrypt(encrypted, identityPath)
				if decErr == nil {
					content = decrypted
				}
			}
		}

		incoming[relPath] = content
		return nil
	})

	return incoming
}

// resolveConflicts prompts the user to resolve each conflict interactively.
// When force is true, the remote version wins automatically. Returns the list of
// conflicts that were resolved in favor of the remote version (so the caller can
// update pending files).
func (c *PullCommand) resolveConflicts(
	toolDir string,
	conflicts []entities.Conflict,
	force bool,
) []entities.Conflict {
	var remoteWins []entities.Conflict

	for _, conflict := range conflicts {
		if force {
			if err := c.conflictDetector.ResolveConflict(toolDir, conflict, "remote"); err != nil {
				logger.Warnf("failed to resolve conflict for %s: %v", conflict.Path, err)
			} else {
				remoteWins = append(remoteWins, conflict)
			}
			continue
		}

		choice := c.promptService.PromptConflictResolution(conflict.Path, conflict.RemoteDevice)

		switch choice {
		case "local":
			if err := c.conflictDetector.ResolveConflict(toolDir, conflict, "local"); err != nil {
				logger.Warnf("failed to resolve conflict for %s: %v", conflict.Path, err)
			}
		case "remote":
			if err := c.conflictDetector.ResolveConflict(toolDir, conflict, "remote"); err != nil {
				logger.Warnf("failed to resolve conflict for %s: %v", conflict.Path, err)
			} else {
				remoteWins = append(remoteWins, conflict)
			}
		case "skip":
			fmt.Printf("  Skipped conflict for %s (conflict file preserved)\n", conflict.Path)
		}
	}

	return remoteWins
}

// ExpandHome resolves a path that starts with ~/ to the user's home directory.
func ExpandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	// Expand Windows environment variables like %APPDATA% and %USERPROFILE%.
	if strings.Contains(path, "%") {
		return os.ExpandEnv(path)
	}
	return path
}

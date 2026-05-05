package commands

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

const actionSkip = "skip"

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
	bundleService     repositories.BundleService
	bundleStateRepo   repositories.BundleStateRepository
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
	bundleService repositories.BundleService,
	bundleStateRepo repositories.BundleStateRepository,
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
		bundleService:     bundleService,
		bundleStateRepo:   bundleStateRepo,
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

	c.applyHooksExclude(config)

	// Step 1: Pull personal changes from the sync repo (other devices).
	if pullErr := c.pullGitRepo(repoPath); pullErr != nil {
		logger.Warnf("git pull skipped: %v", pullErr)
	}

	// Step 2: Recover any incomplete atomic apply from a previous run.
	if recoverErr := c.applyService.Recover(); recoverErr != nil {
		logger.Warnf("failed to recover incomplete apply: %v", recoverErr)
	}

	// Step 3: Load state for ETag caching.
	state := c.loadOrCreateState(repoPath)

	// Step 4-6: Fetch sources, verify checksums, and write to the sync repo.
	allFiles, sourceFileMap, fetchErr := c.fetchAndVerifySources(config, state, repoPath, opts)
	if fetchErr != nil {
		return fetchErr
	}

	// Step 7: Load encrypt patterns for personal file decryption.
	encryptPatterns := c.loadEncryptPatterns(repoPath)

	// Step 8: Apply files to each enabled AI tool directory.
	c.applyToolDirectories(config, repoPath, allFiles, sourceFileMap, encryptPatterns, opts)

	// Step 8b: Apply project bundles (post-file-apply so individual files
	// always land first; bundle merging only runs after the rest of the
	// pull has finished writing).
	c.applyBundles(config, repoPath)

	// Step 9: Update state timestamps after successful pull.
	c.finalizeState(state, repoPath)

	fmt.Fprintln(os.Stdout, "Pull complete.")
	return nil
}

// applyHooksExclude wires hooks_exclude rules from the config into the hooks
// merger when the merger implements the ExcludeAware extension.
func (c *PullCommand) applyHooksExclude(config *entities.Config) {
	if len(config.HooksExclude) == 0 {
		return
	}
	if ea, ok := c.hooksMerger.(repositories.ExcludeAware); ok {
		ea.SetExcludes(config.HooksExclude)
	}
}

// fetchAndVerifySources fetches every configured source in order, persists
// updated ETag/Last-Modified headers, runs force-push detection against the
// existing sync repo, and writes the new content into shared/.
func (c *PullCommand) fetchAndVerifySources(
	config *entities.Config,
	state *entities.State,
	repoPath string,
	opts PullOptions,
) (map[string]fileEntry, map[string]map[string][]byte, error) {
	allFiles := make(map[string]fileEntry)
	sourceFileMap := make(map[string]map[string][]byte)

	if len(config.Sources) == 0 {
		fmt.Fprintln(
			os.Stdout,
			"No external sources configured — applying personal files only. Add sources with: aisync source add",
		)
		return allFiles, sourceFileMap, nil
	}

	// Snapshot old ETags before fetching so we can detect force-push scenarios.
	oldETags := make(map[string]string)
	for _, source := range config.Sources {
		if etag := state.GetETag(source.Name); etag != "" {
			oldETags[source.Name] = etag
		}
	}

	allFiles, sourceFileMap = c.fetchSources(config, state, opts.SourceFilter)

	if saveErr := c.stateRepo.Save(repoPath, state); saveErr != nil {
		logger.Warnf("failed to save state after fetching sources: %v", saveErr)
	}

	if len(allFiles) == 0 {
		fmt.Fprintln(os.Stdout, "All sources are up to date.")
		return allFiles, sourceFileMap, nil
	}

	// Per-file checksum verification against previous sync repo contents.
	// Warn when a file's content changed but the source ETag did not, which may
	// indicate a force-push or silent upstream modification.
	if err := c.verifyFileChecksums(repoPath, allFiles, oldETags, state, opts.Force); err != nil {
		return nil, nil, err
	}

	c.writeToSyncRepo(repoPath, allFiles)
	return allFiles, sourceFileMap, nil
}

// applyToolDirectories iterates over enabled tools and applies the merged
// content to each tool's home directory.
func (c *PullCommand) applyToolDirectories(
	config *entities.Config,
	repoPath string,
	allFiles map[string]fileEntry,
	sourceFileMap map[string]map[string][]byte,
	encryptPatterns *entities.EncryptPatterns,
	opts PullOptions,
) {
	for toolName, tool := range config.Tools {
		if !tool.Enabled {
			continue
		}

		if applyErr := c.applyToToolDir(
			config, repoPath, toolName, tool, allFiles, sourceFileMap,
			encryptPatterns, opts,
		); applyErr != nil {
			logger.Warnf("failed to apply to tool %s: %v", toolName, applyErr)
		}
	}
}

// finalizeState refreshes the LastPull timestamp and the current device's
// LastSync timestamp, then persists the state to the sync repo.
func (c *PullCommand) finalizeState(state *entities.State, repoPath string) {
	state.LastPull = time.Now()
	hostname, _ := os.Hostname()
	if device := state.FindDevice(hostname); device != nil {
		device.LastSync = time.Now()
	}
	if saveErr := c.stateRepo.Save(repoPath, state); saveErr != nil {
		logger.Warnf("failed to update state after pull: %v", saveErr)
	}
}

// pullGitRepo opens the sync repo and pulls the latest changes from remote.
func (c *PullCommand) pullGitRepo(repoPath string) error {
	if err := c.gitRepo.Open(repoPath); err != nil {
		return fmt.Errorf("failed to open git repo: %w", err)
	}

	if !c.gitRepo.HasRemote() {
		return errors.New("no remote configured")
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

		if err := os.MkdirAll(destDir, 0700); err != nil {
			logger.Warnf("failed to create directory %s: %v", destDir, err)
			continue
		}

		existing, readErr := os.ReadFile(destPath)
		if readErr == nil && checksumBytes(existing) == entry.checksum {
			skipped++
			continue
		}

		if err := os.WriteFile(destPath, entry.content, 0600); err != nil {
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

	oldManifest := c.loadOldManifest(toolDir)
	manifest := entities.NewManifest("0.1.0", hostname)

	pendingFiles := c.collectSharedFiles(
		config, repoPath, toolName, toolDir, prefix, allFiles,
		sourceFileMap, encryptPatterns, manifest, opts, tool.ExtraAllowlist,
	)

	c.applyPersonalOnlyFiles(
		repoPath, toolName, toolDir, prefix, allFiles,
		pendingFiles, manifest, encryptPatterns, config, tool.ExtraAllowlist,
	)

	c.detectAndResolveConflicts(
		repoPath, toolName, toolDir, hostname, oldManifest,
		encryptPatterns, config, pendingFiles, manifest, opts,
	)

	deletions := c.detectDeletions(oldManifest, manifest, toolDir)

	if len(pendingFiles) == 0 && len(deletions) == 0 {
		logger.Infof("tool %s: no changes", toolName)
		return nil
	}

	if opts.DryRun {
		c.printDryRun(toolName, pendingFiles, deletions, toolDir)
		return nil
	}

	if !opts.Force {
		proceed, confirmErr := c.confirmToolApply(toolName, pendingFiles, deletions, toolDir)
		if confirmErr != nil {
			return confirmErr
		}
		if !proceed {
			return nil
		}
	}

	c.filterPerFileSkips(toolDir, pendingFiles, opts)

	if err := c.atomicApply(toolName, pendingFiles); err != nil {
		return err
	}

	c.processDeletions(deletions)

	if err := c.manifestRepo.Save(toolDir, manifest); err != nil {
		logger.Warnf("failed to save manifest for %s: %v", toolName, err)
	}

	logger.Infof("tool %s: applied %d files, deleted %d files", toolName, len(pendingFiles), len(deletions))
	return nil
}

// loadOldManifest loads the existing manifest from a tool directory for deletion
// detection. Returns nil if no manifest exists.
func (c *PullCommand) loadOldManifest(toolDir string) *entities.Manifest {
	if !c.manifestRepo.Exists(toolDir) {
		return nil
	}
	loaded, err := c.manifestRepo.Load(toolDir)
	if err != nil {
		return nil
	}
	return loaded
}

// collectSharedFiles iterates over all shared files destined for a tool,
// applies merge strategies and personal overrides, and populates the manifest.
func (c *PullCommand) collectSharedFiles(
	config *entities.Config,
	repoPath, toolName, toolDir, prefix string,
	allFiles map[string]fileEntry,
	sourceFileMap map[string]map[string][]byte,
	encryptPatterns *entities.EncryptPatterns,
	manifest *entities.Manifest,
	opts PullOptions,
	extraAllowlist []string,
) map[string][]byte {
	pendingFiles := make(map[string][]byte)

	for relPath, entry := range allFiles {
		if !strings.HasPrefix(relPath, prefix) {
			continue
		}

		localRel := strings.TrimPrefix(relPath, prefix)
		destPath := filepath.Join(toolDir, localRel)

		if !entities.IsSyncable(toolName, localRel, extraAllowlist) {
			logger.Warnf("pull: source %q tried to deliver %q which is not in %s's allowlist — skipping",
				entry.source, relPath, toolName)
			continue
		}

		content := c.resolveFileContent(
			config, repoPath, toolName, localRel, relPath,
			entry, sourceFileMap, encryptPatterns,
		)

		if !opts.Force { //nolint:nestif // confirmation flow requires nested conditions
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

	return pendingFiles
}

// resolveFileContent applies merge strategies and personal file overrides to
// determine the final content for a shared file.
func (c *PullCommand) resolveFileContent(
	config *entities.Config,
	repoPath, toolName, localRel, relPath string,
	entry fileEntry,
	sourceFileMap map[string]map[string][]byte,
	encryptPatterns *entities.EncryptPatterns,
) []byte {
	content, mergeErr := c.applyMergeStrategy(
		config, repoPath, toolName, localRel, relPath,
		entry, sourceFileMap, encryptPatterns,
	)
	if mergeErr != nil {
		logger.Warnf("merge failed for %s: %v", relPath, mergeErr)
		content = entry.content
	}

	personalContent := c.readPersonalFile(
		repoPath, toolName, localRel, encryptPatterns, config,
	)
	if personalContent != nil {
		logger.Warnf("personal file shadows shared file: %s/%s", toolName, localRel)
		content = personalContent
	}

	return content
}

// detectAndResolveConflicts runs conflict detection for incoming personal files
// and applies resolved remote choices to the pending files and manifest.
func (c *PullCommand) detectAndResolveConflicts(
	repoPath, toolName, toolDir, hostname string,
	oldManifest *entities.Manifest,
	encryptPatterns *entities.EncryptPatterns,
	config *entities.Config,
	pendingFiles map[string][]byte,
	manifest *entities.Manifest,
	opts PullOptions,
) {
	if c.conflictDetector == nil || oldManifest == nil {
		return
	}

	incomingPersonal := c.collectIncomingPersonalFiles(repoPath, toolName, encryptPatterns, config)
	if len(incomingPersonal) == 0 {
		return
	}

	conflicts, detectErr := c.conflictDetector.DetectConflicts(toolDir, incomingPersonal, oldManifest, hostname)
	if detectErr != nil {
		logger.Warnf("conflict detection failed for %s: %v", toolName, detectErr)
		return
	}

	if len(conflicts) == 0 {
		return
	}

	resolved := c.resolveConflicts(toolDir, conflicts, opts.Force)
	for _, rc := range resolved {
		destPath := filepath.Join(toolDir, rc.Path)
		pendingFiles[destPath] = rc.RemoteContent
		manifest.SetFile(rc.Path, "personal", "personal", checksumBytes(rc.RemoteContent))
	}
}

// confirmToolApply prompts the user for confirmation before applying changes to
// a tool directory. Returns (true, nil) to proceed, (false, nil) if the user
// skipped, or (false, error) if the user aborted.
func (c *PullCommand) confirmToolApply(
	toolName string,
	pendingFiles map[string][]byte,
	deletions []string,
	toolDir string,
) (bool, error) {
	c.printDiffSummary(toolName, pendingFiles, deletions, toolDir)
	for {
		action := c.promptService.PromptToolAction(toolName)
		switch action {
		case actionSkip:
			fmt.Fprintf(os.Stdout, "Skipped tool %s.\n", toolName)
			return false, nil
		case "abort":
			return false, errors.New("aborted by user")
		case "diff":
			c.printDryRun(toolName, pendingFiles, deletions, toolDir)
			continue
		case "apply":
			// proceed
		}
		break
	}
	return true, nil
}

// filterPerFileSkips allows the user to exclude individual files when not forced
// and there are multiple pending files.
func (c *PullCommand) filterPerFileSkips(
	toolDir string,
	pendingFiles map[string][]byte,
	opts PullOptions,
) {
	if opts.Force || len(pendingFiles) <= 1 {
		return
	}

	for destPath := range pendingFiles {
		relPath, _ := filepath.Rel(toolDir, destPath)
		action := c.promptService.PromptFileAction(relPath, "incoming")
		if action == actionSkip {
			delete(pendingFiles, destPath)
			logger.Infof("skipped file '%s' (user choice)", relPath)
		}
	}
}

// atomicApply stages pending files and then moves them to their final destinations.
func (c *PullCommand) atomicApply(toolName string, pendingFiles map[string][]byte) error {
	if len(pendingFiles) == 0 {
		return nil
	}

	journal, stageErr := c.applyService.Stage(pendingFiles)
	if stageErr != nil {
		return fmt.Errorf("failed to stage files for %s: %w", toolName, stageErr)
	}

	if applyErr := c.applyService.Apply(journal); applyErr != nil {
		return fmt.Errorf("failed to apply staged files for %s: %w", toolName, applyErr)
	}
	return nil
}

// processDeletions removes files that were present in the old manifest but not
// in the new one.
func (c *PullCommand) processDeletions(deletions []string) {
	for _, delPath := range deletions {
		if err := os.Remove(delPath); err != nil && !os.IsNotExist(err) {
			logger.Warnf("failed to delete %s: %v", delPath, err)
		} else {
			logger.Infof("deleted: %s", delPath)
		}
	}
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
	extraAllowlist []string,
) {
	personalDir := filepath.Join(repoPath, "personal", toolName)
	if _, err := os.Stat(personalDir); os.IsNotExist(err) {
		return
	}

	_ = filepath.Walk(personalDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return nil //nolint:nilerr // return nil to continue Walk traversal
		}

		relPath, err := filepath.Rel(personalDir, path)
		if err != nil {
			return nil //nolint:nilerr // return nil to continue Walk traversal
		}

		if strings.HasSuffix(relPath, ".age") {
			return nil
		}

		sharedKey := sharedPrefix + relPath
		if _, exists := allFiles[sharedKey]; exists {
			return nil
		}

		destPath := filepath.Join(toolDir, relPath)
		if !entities.IsSyncable(toolName, relPath, extraAllowlist) {
			return nil
		}

		content := c.readAndDecryptPersonalFile(path, relPath, encryptPatterns, config)
		if content == nil {
			return nil
		}

		pendingFiles[destPath] = content
		manifest.SetFile(relPath, "personal", "personal", checksumBytes(content))
		return nil
	})
}

// readAndDecryptPersonalFile reads a personal file and decrypts it if it matches
// encrypt patterns and an encrypted version exists.
func (c *PullCommand) readAndDecryptPersonalFile(
	path, relPath string,
	encryptPatterns *entities.EncryptPatterns,
	config *entities.Config,
) []byte {
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		logger.Warnf("failed to read personal file %s: %v", path, readErr)
		return nil
	}

	if encryptPatterns.Matches(relPath) && config.Encryption.Identity != "" {
		encryptedPath := path + ".age"
		encrypted, encErr := os.ReadFile(encryptedPath)
		if encErr == nil {
			identityPath := ExpandHome(config.Encryption.Identity)
			decrypted, decErr := c.encryptionService.Decrypt(encrypted, identityPath)
			if decErr == nil {
				return decrypted
			}
			logger.Warnf("failed to decrypt %s: %v", encryptedPath, decErr)
		}
	}

	return content
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
	fmt.Fprintf(os.Stdout, "[dry-run] Tool %s:\n", toolName)
	for destPath, content := range pendingFiles {
		relPath, _ := filepath.Rel(toolDir, destPath)
		existing, err := os.ReadFile(destPath)
		switch {
		case err != nil:
			fmt.Fprintf(os.Stdout, "  + %s (%s, new)\n", relPath, formatSize(int64(len(content))))
		case checksumBytes(existing) != checksumBytes(content):
			detail := fmt.Sprintf("%s → %s", formatSize(int64(len(existing))), formatSize(int64(len(content))))
			oldLines := bytes.Count(existing, []byte("\n"))
			newLines := bytes.Count(content, []byte("\n"))
			if delta := newLines - oldLines; delta != 0 {
				detail += fmt.Sprintf(", %+d lines", delta)
			}
			fmt.Fprintf(os.Stdout, "  ~ %s (%s)\n", relPath, detail)
		default:
			fmt.Fprintf(os.Stdout, "  = %s (unchanged)\n", relPath)
		}
	}
	for _, delPath := range deletions {
		relPath, _ := filepath.Rel(toolDir, delPath)
		fmt.Fprintf(os.Stdout, "  - %s (deleted)\n", relPath)
	}
}

// printDiffSummary displays a compact summary of pending changes for confirmation.
func (c *PullCommand) printDiffSummary(
	toolName string,
	pendingFiles map[string][]byte,
	deletions []string,
	_ string,
) {
	added := 0
	modified := 0
	unchanged := 0

	for destPath, content := range pendingFiles {
		existing, err := os.ReadFile(destPath)
		switch {
		case err != nil:
			added++
		case checksumBytes(existing) != checksumBytes(content):
			modified++
		default:
			unchanged++
		}
	}

	fmt.Fprintf(os.Stdout, "Tool %s: %d new, %d modified, %d unchanged, %d deletions\n",
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
			return nil //nolint:nilerr // return nil to continue Walk traversal
		}

		relPath, err := filepath.Rel(personalDir, path)
		if err != nil {
			return nil //nolint:nilerr // return nil to continue Walk traversal
		}

		// Skip .age files; they are handled via the decrypt path.
		if strings.HasSuffix(relPath, ".age") {
			return nil
		}

		content, readErr := os.ReadFile(path) //nolint:gosec // paths are from trusted tool directories
		if readErr != nil {
			return nil //nolint:nilerr // return nil to continue Walk traversal
		}

		// Decrypt if needed.
		if encryptPatterns.Matches(relPath) && config.Encryption.Identity != "" {
			encryptedPath := path + ".age"
			encrypted, encErr := os.ReadFile(encryptedPath) //nolint:gosec // paths are from trusted tool directories
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
		case actionSkip:
			fmt.Fprintf(os.Stdout, "  Skipped conflict for %s (conflict file preserved)\n", conflict.Path)
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

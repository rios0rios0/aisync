package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// PushCommand collects personal files from AI tool directories, copies them
// into the sync repository, and pushes the changes to the remote.
type PushCommand struct {
	configRepo        repositories.ConfigRepository
	stateRepo         repositories.StateRepository
	gitRepo           repositories.GitRepository
	encryptionService repositories.EncryptionService
	manifestRepo      repositories.ManifestRepository
	secretScanner     repositories.SecretScanner
	ndaChecker        repositories.NDAContentChecker
}

// NewPushCommand creates a new PushCommand.
func NewPushCommand(
	configRepo repositories.ConfigRepository,
	stateRepo repositories.StateRepository,
	gitRepo repositories.GitRepository,
	encryptionService repositories.EncryptionService,
	manifestRepo repositories.ManifestRepository,
	secretScanner repositories.SecretScanner,
	ndaChecker repositories.NDAContentChecker,
) *PushCommand {
	return &PushCommand{
		configRepo:        configRepo,
		stateRepo:         stateRepo,
		gitRepo:           gitRepo,
		encryptionService: encryptionService,
		manifestRepo:      manifestRepo,
		secretScanner:     secretScanner,
		ndaChecker:        ndaChecker,
	}
}

// encryptMatchPath builds the repo-relative path under personal/<tool>/ that
// is used for matching .aisyncencrypt patterns. Every encrypt match site must
// use this helper so .aisyncencrypt patterns, .gitattributes filters, and the
// secret scanner all agree on path semantics.
func encryptMatchPath(toolName, relPath string) string {
	return filepath.ToSlash(filepath.Join("personal", toolName, relPath))
}

// PushOptions bundles the boolean flags Execute takes so new flags can be
// added (such as the NDA-scan bypass) without breaking every caller.
type PushOptions struct {
	SkipSecretScan bool
	SkipNDAScan    bool
	DryRun         bool
}

// Execute scans enabled AI tool directories for personal files, copies them into
// the sync repo under personal/<tool>/, commits, and pushes. The options
// control whether the secret scanner, NDA scanner, or dry-run mode are
// active. Any scanner firing with findings blocks the push; the NDA scan
// runs after the secret scan so users see both classes of finding in a
// single command when both trip.
func (c *PushCommand) Execute(configPath, repoPath, commitMsg string, opts PushOptions) error {
	config, err := c.configRepo.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if !opts.DryRun {
		if err = c.gitRepo.Open(repoPath); err != nil {
			return fmt.Errorf("failed to open git repo: %w", err)
		}
	}

	ignorePatterns := c.loadIgnorePatterns(repoPath)
	encryptPatterns := c.loadEncryptPatterns(repoPath)

	if opts.DryRun {
		if dryErr := c.executeDryRun(config, repoPath, ignorePatterns, encryptPatterns, opts); dryErr != nil {
			return dryErr
		}
		c.warnLegacyRepoFiles(config, repoPath)
		return nil
	}

	copied := c.collectAllPersonalFiles(config, repoPath, ignorePatterns, encryptPatterns)
	logger.Infof("collected %d personal files into sync repo", copied)

	c.warnLegacyRepoFiles(config, repoPath)

	if err = c.commitAndPush(repoPath, commitMsg, opts, encryptPatterns, config); err != nil {
		return err
	}

	if updateErr := c.updateState(repoPath); updateErr != nil {
		logger.Warnf("failed to update state: %v", updateErr)
	}

	fmt.Fprintf(os.Stdout, "Push complete: %d files collected.\n", copied)
	return nil
}

// ageSuffix is the filename suffix [PushCommand.copyPersonalFile] appends
// to files that matched an encrypt pattern. [warnLegacyRepoFiles] strips
// it before checking the allowlist so a legitimately-encrypted file whose
// plaintext equivalent is allowlisted is not flagged as legacy.
const ageSuffix = ".age"

// legacyHit records a single file under personal/<tool>/ whose tool-relative
// path no longer passes the current allowlist.
type legacyHit struct {
	toolName string
	relPath  string
	fullPath string
}

// warnLegacyRepoFiles walks personal/<tool>/ directories in the sync repo
// and emits a loud warning for any file whose tool-relative path is no
// longer syncable under the current allowlist. These are typically legacy
// entries committed under an older, more permissive deny-list-based
// version of aisync (e.g. projects/*.jsonl, paste-cache/*, plugins/**).
// The function never deletes anything — cleanup is the user's call. The
// warning includes the exact git command to remove obsolete paths.
func (c *PushCommand) warnLegacyRepoFiles(config *entities.Config, repoPath string) {
	var legacy []legacyHit
	for toolName, tool := range config.Tools {
		if !tool.Enabled {
			continue
		}
		legacy = append(legacy, c.collectLegacyHitsForTool(repoPath, toolName, tool.ExtraAllowlist)...)
	}

	if len(legacy) == 0 {
		return
	}

	logLegacyWarning(repoPath, legacy)
}

// collectLegacyHitsForTool walks personal/<tool>/ and returns any file whose
// tool-relative path no longer passes the allowlist. Never deletes.
func (c *PushCommand) collectLegacyHitsForTool(repoPath, toolName string, extra []string) []legacyHit {
	personalDir := filepath.Join(repoPath, "personal", toolName)
	if _, err := os.Stat(personalDir); os.IsNotExist(err) {
		return nil
	}

	var hits []legacyHit
	_ = filepath.Walk(personalDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return nil //nolint:nilerr // return nil to continue Walk traversal
		}
		relPath, relErr := filepath.Rel(personalDir, path)
		if relErr != nil {
			return nil //nolint:nilerr // return nil to continue Walk traversal
		}
		// .age-encrypted copies are allowed even when the plaintext
		// equivalent is syncable — strip the suffix for the allowlist check.
		checkPath := relPath
		if filepath.Ext(checkPath) == ageSuffix {
			checkPath = checkPath[:len(checkPath)-len(ageSuffix)]
		}
		if entities.IsSyncable(toolName, checkPath, extra) {
			return nil
		}
		hits = append(hits, legacyHit{
			toolName: toolName,
			relPath:  relPath,
			fullPath: filepath.Join("personal", toolName, relPath),
		})
		return nil
	})
	return hits
}

// logLegacyWarning prints the WARN block for legacy files with a ready-to-run
// git rm command so users can clean up obsolete entries on demand.
func logLegacyWarning(repoPath string, legacy []legacyHit) {
	logger.Warnf(
		"%d file(s) under personal/ are no longer in the allowlist and will be LEFT UNTOUCHED in the repo:",
		len(legacy),
	)
	for _, hit := range legacy {
		logger.Warnf("  %s", hit.fullPath)
	}
	logger.Warn("To clean them up, run:")
	logger.Warnf("  git -C %s rm -r \\", repoPath)
	for i, hit := range legacy {
		if i == len(legacy)-1 {
			logger.Warnf("    %s", hit.fullPath)
			continue
		}
		logger.Warnf("    %s \\", hit.fullPath)
	}
}

// collectAllPersonalFiles iterates over all enabled tools and collects personal
// files from each tool directory into the sync repo.
func (c *PushCommand) collectAllPersonalFiles(
	config *entities.Config,
	repoPath string,
	ignorePatterns *entities.IgnorePatterns,
	encryptPatterns *entities.EncryptPatterns,
) int {
	copied := 0
	for toolName, tool := range config.Tools {
		if !tool.Enabled {
			continue
		}

		toolDir := ExpandHome(tool.Path)
		if _, statErr := os.Stat(toolDir); os.IsNotExist(statErr) {
			logger.Debugf("tool directory %s does not exist, skipping", toolDir)
			continue
		}

		manifest := c.loadManifest(toolDir)
		n, err := c.collectPersonalFiles(
			toolDir,
			toolName,
			repoPath,
			manifest,
			ignorePatterns,
			encryptPatterns,
			config,
			tool.ExtraAllowlist,
		)
		if err != nil {
			logger.Warnf("failed to collect personal files for %s: %v", toolName, err)
			continue
		}
		copied += n
	}
	return copied
}

// commitAndPush checks for changes, runs the content-scan pipeline
// (secret scanner + NDA scanner, each independently skippable), commits,
// and pushes.
func (c *PushCommand) commitAndPush(
	repoPath, commitMsg string,
	opts PushOptions,
	encryptPatterns *entities.EncryptPatterns,
	config *entities.Config,
) error {
	clean, err := c.gitRepo.IsClean()
	if err != nil {
		return fmt.Errorf("failed to check git status: %w", err)
	}
	if clean {
		logger.Info("no changes to commit")
		return nil
	}

	unencrypted, err := c.collectUnencryptedFiles(repoPath, encryptPatterns, config)
	if err != nil {
		return err
	}

	if !opts.SkipSecretScan && len(unencrypted) > 0 {
		if scanErr := c.runSecretScan(unencrypted); scanErr != nil {
			return scanErr
		}
	}

	if !opts.SkipNDAScan && len(unencrypted) > 0 && c.ndaChecker != nil {
		if scanErr := c.runNDAScan(repoPath, config, unencrypted); scanErr != nil {
			return scanErr
		}
	}

	if commitMsg == "" {
		hostname, _ := os.Hostname()
		commitMsg = fmt.Sprintf("sync(%s): updated personal configurations", hostname)
	}

	if err = c.gitRepo.CommitAll(commitMsg); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}
	logger.Infof("committed: %s", commitMsg)

	if c.gitRepo.HasRemote() {
		if pushErr := c.gitRepo.Push(); pushErr != nil {
			logger.Warnf("push failed (will retry on next sync): %v", pushErr)
		} else {
			logger.Info("pushed to remote")
		}
	} else {
		logger.Info("no remote configured, skipping push")
	}

	return nil
}

// dryRunToolResult holds the per-tool dry-run scan outcome.
type dryRunToolResult struct {
	files     int
	encrypted int
}

// executeDryRun detects personal files that would be pushed and prints a
// summary without modifying the sync repo, committing, or pushing. It
// also runs the secret + NDA content scanners against the would-be
// pushed files so users discover blocks before they commit instead of
// after — the whole point of `--dry-run` is to preview the real result.
func (c *PushCommand) executeDryRun(
	config *entities.Config,
	repoPath string,
	ignorePatterns *entities.IgnorePatterns,
	encryptPatterns *entities.EncryptPatterns,
	opts PushOptions,
) error {
	totalFiles := 0
	encryptedFiles := 0
	skippedTools := 0
	unencrypted := make(map[string][]byte)

	fmt.Fprintln(os.Stdout, "[dry-run] Push summary:")

	for toolName, tool := range config.Tools {
		if !tool.Enabled {
			skippedTools++
			continue
		}

		toolDir := ExpandHome(tool.Path)
		if _, statErr := os.Stat(toolDir); os.IsNotExist(statErr) {
			skippedTools++
			continue
		}

		result := c.dryRunScanTool(
			toolName, toolDir, ignorePatterns, encryptPatterns,
			config, tool.ExtraAllowlist, unencrypted,
		)
		totalFiles += result.files
		encryptedFiles += result.encrypted
	}

	fmt.Fprintf(os.Stdout, "\n[dry-run] %d file(s) to push, %d encrypted, %d tool(s) skipped\n",
		totalFiles, encryptedFiles, skippedTools)

	if !opts.SkipSecretScan && len(unencrypted) > 0 {
		if scanErr := c.runSecretScan(unencrypted); scanErr != nil {
			return scanErr
		}
	}
	if !opts.SkipNDAScan && len(unencrypted) > 0 && c.ndaChecker != nil {
		if scanErr := c.runNDAScan(repoPath, config, unencrypted); scanErr != nil {
			return scanErr
		}
	}
	return nil
}

// dryRunScanTool walks a single tool directory and prints the files that
// would be pushed, returning the count of files and encrypted files.
// Plaintext (non-encrypted) file contents are also accumulated into the
// shared `unencrypted` map so the dry-run caller can run the secret + NDA
// scanners against them — same map shape `commitAndPush` builds in the
// real push path.
func (c *PushCommand) dryRunScanTool(
	toolName, toolDir string,
	ignorePatterns *entities.IgnorePatterns,
	encryptPatterns *entities.EncryptPatterns,
	config *entities.Config,
	extraAllowlist []string,
	unencrypted map[string][]byte,
) dryRunToolResult {
	manifest := c.loadManifest(toolDir)
	var result dryRunToolResult

	walkErr := filepath.Walk(toolDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil //nolint:nilerr // return nil to continue Walk traversal
		}

		relPath, relErr := filepath.Rel(toolDir, path)
		if relErr != nil {
			return nil //nolint:nilerr // return nil to continue Walk traversal
		}

		if !entities.IsSyncable(toolName, relPath, extraAllowlist) ||
			ignorePatterns.Matches(relPath) ||
			c.isSharedFile(relPath, manifest) {
			return nil
		}

		encrypted := encryptPatterns.Matches(encryptMatchPath(toolName, relPath)) &&
			len(config.Encryption.Recipients) > 0
		if encrypted {
			result.encrypted++
			fmt.Fprintf(os.Stdout, "  %s/%s (encrypted)\n", toolName, relPath)
		} else {
			fmt.Fprintf(os.Stdout, "  %s/%s\n", toolName, relPath)
			//nolint:gosec // paths are from trusted tool directories
			content, readErr := os.ReadFile(path)
			if readErr == nil {
				// Use the same `personal/<tool>/<rel>` key shape the
				// real-push scanners see, so finding messages line up
				// with the on-disk repo paths users will edit.
				key := filepath.Join("personal", toolName, relPath)
				unencrypted[key] = content
			}
		}

		result.files++
		return nil
	})
	if walkErr != nil {
		logger.Warnf("failed to walk %s: %v", toolDir, walkErr)
	}

	return result
}

// collectPersonalFiles walks a tool directory and copies files that are not tracked
// as "shared" in the manifest into the sync repo under personal/<tool>/.
func (c *PushCommand) collectPersonalFiles(
	toolDir, toolName, repoPath string,
	manifest *entities.Manifest,
	ignorePatterns *entities.IgnorePatterns,
	encryptPatterns *entities.EncryptPatterns,
	config *entities.Config,
	extraAllowlist []string,
) (int, error) {
	personalDir := filepath.Join(repoPath, "personal", toolName)
	if err := os.MkdirAll(personalDir, 0700); err != nil {
		return 0, fmt.Errorf("failed to create personal directory: %w", err)
	}

	copied := 0
	err := filepath.Walk(toolDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return nil //nolint:nilerr // return nil to continue Walk traversal
		}

		relPath, err := filepath.Rel(toolDir, path)
		if err != nil {
			return nil //nolint:nilerr // return nil to continue Walk traversal
		}

		if !entities.IsSyncable(toolName, relPath, extraAllowlist) ||
			ignorePatterns.Matches(relPath) ||
			c.isSharedFile(relPath, manifest) {
			return nil
		}

		if c.copyPersonalFile(path, relPath, toolName, personalDir, encryptPatterns, config) {
			copied++
		}
		return nil
	})

	return copied, err
}

// copyPersonalFile reads a single file, optionally encrypts it, and writes it to
// the personal directory if it has changed. Returns true if the file was copied.
// The toolName parameter is used to build the repo-relative path under
// personal/<tool>/ for matching encrypt patterns.
func (c *PushCommand) copyPersonalFile(
	path, relPath, toolName, personalDir string,
	encryptPatterns *entities.EncryptPatterns,
	config *entities.Config,
) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		logger.Warnf("failed to read %s: %v", path, err)
		return false
	}

	matchedForEncryption := encryptPatterns.Matches(encryptMatchPath(toolName, relPath))
	switch {
	case matchedForEncryption && len(config.Encryption.Recipients) > 0:
		encrypted, encErr := c.encryptionService.Encrypt(content, config.Encryption.Recipients)
		if encErr != nil {
			logger.Warnf("failed to encrypt %s: %v", relPath, encErr)
			return false
		}
		content = encrypted
		relPath += ageSuffix
	case matchedForEncryption:
		// Pattern matches but no recipients are configured: the file is
		// about to be written as plaintext. Warn loudly so operators
		// notice the misconfiguration (typically a cloned repo with no
		// imported age identity, or a stale `recipients: []` list). The
		// secret scanner still runs on this file — see runSecretScan.
		// Route the path through encryptMatchPath so the log line shows
		// the repo-relative form (`personal/<tool>/<rest>`) that matches
		// what .gitattributes and .aisyncencrypt operate on, rather than
		// a bare tool-relative path that is ambiguous when multiple
		// tools are enabled.
		logger.Warnf(
			"file %s matches an encrypt pattern but no recipients are configured; "+
				"writing as plaintext. Run `aisync key generate` or add a recipient to config.yaml.",
			encryptMatchPath(toolName, relPath),
		)
	}

	destPath := filepath.Clean(filepath.Join(personalDir, relPath))
	destDir := filepath.Dir(destPath)
	if err = os.MkdirAll(destDir, 0700); err != nil {
		logger.Warnf("failed to create directory %s: %v", destDir, err)
		return false
	}

	existing, readErr := os.ReadFile(destPath)
	if readErr == nil && checksumBytes(existing) == checksumBytes(content) {
		return false
	}

	if err = os.WriteFile(destPath, content, 0600); err != nil { //nolint:gosec // destPath is filepath.Clean'd above
		logger.Warnf("failed to write %s: %v", destPath, err)
		return false
	}

	logger.Debugf("collected %s -> %s", relPath, destPath)
	return true
}

// isSharedFile checks whether a relative path is tracked as "shared" in the manifest.
func (c *PushCommand) isSharedFile(relPath string, manifest *entities.Manifest) bool {
	if manifest == nil {
		return false
	}

	entry, exists := manifest.Files[relPath]
	if !exists {
		return false
	}

	return entry.Namespace == "shared"
}

// loadManifest loads the manifest for a tool directory, returning nil if it does not exist.
func (c *PushCommand) loadManifest(toolDir string) *entities.Manifest {
	if !c.manifestRepo.Exists(toolDir) {
		return nil
	}

	manifest, err := c.manifestRepo.Load(toolDir)
	if err != nil {
		logger.Warnf("failed to load manifest from %s: %v", toolDir, err)
		return nil
	}

	return manifest
}

// loadIgnorePatterns reads the .aisyncignore file from the sync repo root.
func (c *PushCommand) loadIgnorePatterns(repoPath string) *entities.IgnorePatterns {
	ignorePath := filepath.Join(repoPath, ".aisyncignore")
	content, err := os.ReadFile(ignorePath)
	if err != nil {
		return entities.ParseIgnorePatterns([]byte{})
	}
	return entities.ParseIgnorePatterns(content)
}

// loadEncryptPatterns reads the .aisyncencrypt file from the sync repo root.
func (c *PushCommand) loadEncryptPatterns(repoPath string) *entities.EncryptPatterns {
	encryptPath := filepath.Join(repoPath, ".aisyncencrypt")
	content, err := os.ReadFile(encryptPath)
	if err != nil {
		return entities.ParseEncryptPatterns([]byte{})
	}
	return entities.ParseEncryptPatterns(content)
}

// collectUnencryptedFiles walks the personal/ directory in the sync repo
// and collects every file that is NOT already `.age`-encrypted and NOT
// pattern-matched for at-commit encryption (with recipients present).
// The result is the shared input for both the secret scanner and the NDA
// scanner: either running or neither, but both always see the exact same
// set of files.
//
// The recipients gate mirrors [PushCommand.copyPersonalFile]: a file is
// only considered "already encrypted-at-commit" when its encrypt pattern
// matches AND the config has at least one recipient. Without the gate, a
// repo with `recipients: []` (reachable via clone without key import, or
// a stale config) would write pattern-matched files as plaintext via
// copyPersonalFile and then have the scanner skip them here — exactly
// the silent plaintext commit failure mode the scanners exist to prevent.
func (c *PushCommand) collectUnencryptedFiles(
	repoPath string,
	encryptPatterns *entities.EncryptPatterns,
	config *entities.Config,
) (map[string][]byte, error) {
	personalDir := filepath.Join(repoPath, "personal")
	if _, err := os.Stat(personalDir); os.IsNotExist(err) {
		// First-run repo without a personal/ tree yet — return an empty
		// (non-nil) map so callers can iterate without a special case.
		return map[string][]byte{}, nil
	}

	hasRecipients := len(config.Encryption.Recipients) > 0

	unencrypted := make(map[string][]byte)
	err := filepath.Walk(personalDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return nil //nolint:nilerr // return nil to continue Walk traversal
		}

		relPath, relErr := filepath.Rel(repoPath, path)
		if relErr != nil {
			return nil //nolint:nilerr // return nil to continue Walk traversal
		}

		// Skip files that are already encrypted (.age suffix).
		if filepath.Ext(path) == ageSuffix {
			return nil
		}

		// Skip files that would be encrypted AND have at least one
		// recipient configured. When recipients is empty we MUST scan
		// the file because copyPersonalFile writes it as plaintext.
		if hasRecipients && encryptPatterns.Matches(filepath.ToSlash(relPath)) {
			return nil
		}

		content, readErr := os.ReadFile(path) //nolint:gosec // paths are from trusted tool directories
		if readErr != nil {
			return nil //nolint:nilerr // return nil to continue Walk traversal
		}

		unencrypted[relPath] = content
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk personal directory: %w", err)
	}
	return unencrypted, nil
}

// runSecretScan runs the credential regex scanner against the already-
// collected unencrypted files. Returns an error if any credential patterns
// match. The block message hints at both fixes (encrypt the file or pass
// `--skip-secret-scan`).
func (c *PushCommand) runSecretScan(unencrypted map[string][]byte) error {
	findings := c.secretScanner.Scan(unencrypted)
	if len(findings) == 0 {
		return nil
	}

	fmt.Fprintln(os.Stdout, "Secret scan findings:")
	for _, f := range findings {
		fmt.Fprintf(os.Stdout, "  %s:%d - %s\n", f.Path, f.Line, f.Description)
	}

	return fmt.Errorf(
		"push blocked: %d secret(s) detected in unencrypted files. "+
			"Encrypt them with .aisyncencrypt or use --skip-secret-scan",
		len(findings),
	)
}

// runNDAScan runs the NDA content checker (explicit list + auto-derive +
// heuristics) against the already-collected unencrypted files. Any
// finding blocks the push with a source-tagged error message so the user
// knows which knob to turn to fix each hit.
func (c *PushCommand) runNDAScan(
	repoPath string,
	config *entities.Config,
	unencrypted map[string][]byte,
) error {
	findings, err := c.ndaChecker.Check(repoPath, config, unencrypted)
	if err != nil {
		return fmt.Errorf("nda scan failed: %w", err)
	}
	if len(findings) == 0 {
		return nil
	}

	fmt.Fprintln(os.Stdout, "NDA scan findings:")
	for _, f := range findings {
		fmt.Fprintf(os.Stdout, "  %s:%d  [%s]  %s\n", f.Path, f.Line, f.Kind, f.Term)
	}
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "To fix:")
	fmt.Fprintln(os.Stdout, "  1. Sanitize the files (replace the literal with a placeholder)")
	fmt.Fprintln(os.Stdout, "  2. Or ignore a specific auto-derived term: `aisync nda ignore <term>`")
	fmt.Fprintln(os.Stdout, "  3. Or bypass for this push only: `aisync push --skip-nda-scan` (discouraged)")

	return fmt.Errorf(
		"push blocked: %d NDA term hit(s) detected in unencrypted files",
		len(findings),
	)
}

// updateState loads the current state, updates the LastPush timestamp, and saves it back.
func (c *PushCommand) updateState(repoPath string) error {
	var state *entities.State

	if c.stateRepo.Exists(repoPath) {
		loaded, err := c.stateRepo.Load(repoPath)
		if err != nil {
			return fmt.Errorf("failed to load state: %w", err)
		}
		state = loaded
	} else {
		hostname, _ := os.Hostname()
		state = entities.NewState(hostname)
	}

	state.LastPush = time.Now()

	hostname, _ := os.Hostname()
	device := state.FindDevice(hostname)
	if device != nil {
		device.LastSync = time.Now()
	}

	return c.stateRepo.Save(repoPath, state)
}

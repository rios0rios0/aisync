package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// PushCommand reconciles personal files from AI tool directories into the
// sync repository, commits the result, and pushes it to the remote.
type PushCommand struct {
	configRepo     repositories.ConfigRepository
	stateRepo      repositories.StateRepository
	gitRepo        repositories.GitRepository
	secretScanner  repositories.SecretScanner
	ndaChecker     repositories.NDAContentChecker
	bundleService  repositories.BundleService
	personalSyncer *PersonalSyncer
}

// NewPushCommand creates a new PushCommand.
func NewPushCommand(
	configRepo repositories.ConfigRepository,
	stateRepo repositories.StateRepository,
	gitRepo repositories.GitRepository,
	secretScanner repositories.SecretScanner,
	ndaChecker repositories.NDAContentChecker,
	bundleService repositories.BundleService,
	personalSyncer *PersonalSyncer,
) *PushCommand {
	return &PushCommand{
		configRepo:     configRepo,
		stateRepo:      stateRepo,
		gitRepo:        gitRepo,
		secretScanner:  secretScanner,
		ndaChecker:     ndaChecker,
		bundleService:  bundleService,
		personalSyncer: personalSyncer,
	}
}

// encryptMatchPath builds the repo-relative path under personal/<tool>/ that
// is used for matching .aisyncencrypt patterns. Every encrypt match site must
// use this helper so .aisyncencrypt patterns, .gitattributes filters, and the
// secret scanner all agree on path semantics.
func encryptMatchPath(toolName, relPath string) string {
	return filepath.ToSlash(filepath.Join("personal", toolName, relPath))
}

// PushOptions bundles the flags Execute takes so new flags can be added
// (such as the NDA-scan bypass) without breaking every caller.
type PushOptions struct {
	SkipSecretScan bool
	SkipNDAScan    bool
	DryRun         bool
	// Preference resolves personal-file conflicts without prompting.
	Preference entities.SyncPreference
	// NoPrompt skips conflicts Preference does not resolve instead of
	// asking (watch mode must never block on input).
	NoPrompt bool
	// Direction limits the personal-file reconciliation; empty means
	// outgoing only, which is what a push is. Sync passes SyncBoth.
	Direction entities.SyncDirection
	// SkipFetch skips updating the sync repo from its remote first, for
	// callers that just did (sync runs its pull phase immediately before).
	SkipFetch bool
}

// Execute updates the sync repo from its remote, reconciles personal files
// into it with the three-way merge, commits, and pushes. Updating first is
// what lets the reconciliation see changes other devices pushed, so they are
// detected as conflicts instead of being overwritten. Only the plaintext
// files the commit is about to add are scanned for secrets and NDA terms;
// any scanner finding blocks the push.
func (c *PushCommand) Execute(configPath, repoPath, commitMsg string, opts PushOptions) error {
	config, err := c.configRepo.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err = c.gitRepo.Open(repoPath); err != nil {
		return fmt.Errorf("failed to open git repo: %w", err)
	}
	if !opts.SkipFetch && c.gitRepo.HasRemote() {
		if err = c.gitRepo.Pull(); err != nil {
			return fmt.Errorf("failed to update the sync repo from its remote before pushing: %w", err)
		}
	}

	direction := opts.Direction
	if direction == "" {
		direction = entities.SyncOutgoing
	}
	encrypted, err := c.encryptStoredPlaintext(config, repoPath, direction, opts.DryRun)
	if err != nil {
		return err
	}

	bundleCount, bundleErr := c.produceBundles(config, repoPath, opts.DryRun)
	if bundleErr != nil {
		return bundleErr
	}
	report, err := c.personalSyncer.Sync(config, repoPath, PersonalSyncOptions{
		Direction:  direction,
		Preference: opts.Preference,
		NoPrompt:   opts.NoPrompt,
		DryRun:     opts.DryRun,
	})
	if err != nil {
		return err
	}

	c.warnLegacyRepoFiles(config, repoPath)

	if opts.DryRun {
		fmt.Fprintf(
			os.Stdout,
			"\n[dry-run] %d file(s) to send to the repo, %d to delete there, %d to encrypt there, %d project bundle(s)\n",
			report.Pushed,
			report.DeletedRepo,
			encrypted,
			bundleCount,
		)
		return c.scanContent(repoPath, config, report.Plaintext, opts)
	}

	if err = c.commitAndPush(repoPath, commitMsg, opts, config); err != nil {
		return err
	}

	if updateErr := c.updateState(repoPath); updateErr != nil {
		logger.Warnf("failed to update state: %v", updateErr)
	}

	fmt.Fprintln(os.Stdout, "Push complete.")
	return nil
}

// ageSuffix is the filename suffix the PersonalSyncer appends to files that
// matched an encrypt pattern. [warnLegacyRepoFiles] strips it before
// checking the allowlist so a legitimately-encrypted file whose plaintext
// equivalent is allowlisted is not flagged as legacy.
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
		extras := append([]string{}, tool.ExtraAllowlist...)
		extras = append(extras, bundleTargetExtraAllowlist(tool)...)
		legacy = append(legacy, c.collectLegacyHitsForTool(repoPath, toolName, extras)...)
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
		// .gitkeep files are the repo's own scaffolding, not obsolete content.
		if filepath.Base(relPath) == gitkeepName || entities.IsSyncable(toolName, checkPath, extra) {
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

// encryptStoredPlaintext encrypts in place the plaintext personal files that
// match .aisyncencrypt. When the repo had no other uncommitted change, it
// commits them on their own with the encryption-only trailer, so history
// lookups keep each file's last real change instead of this commit. With
// other changes pending, the files ride in the main commit, which is
// scanned, rather than sweeping unscanned plaintext into this one.
func (c *PushCommand) encryptStoredPlaintext(
	config *entities.Config,
	repoPath string,
	direction entities.SyncDirection,
	dryRun bool,
) (int, error) {
	if !direction.Allows(entities.SyncActionPush) || len(config.Encryption.Recipients) == 0 {
		return 0, nil
	}
	if dryRun {
		return c.personalSyncer.EncryptStoredPlaintext(config, repoPath, true), nil
	}

	clean, err := c.gitRepo.IsClean()
	if err != nil {
		return 0, fmt.Errorf("failed to check git status: %w", err)
	}
	encrypted := c.personalSyncer.EncryptStoredPlaintext(config, repoPath, false)
	if encrypted == 0 {
		return 0, nil
	}
	fmt.Fprintf(os.Stdout, "Encrypted %d plaintext personal file(s) in the repo.\n", encrypted)
	if !clean {
		return encrypted, nil
	}

	hostname, _ := os.Hostname()
	message := fmt.Sprintf("sync(%s): encrypted %d personal file(s) in place\n\n%s: %s",
		hostname, encrypted, entities.ChangeTrailerKey, entities.ChangeEncryptionOnly)
	if err = c.gitRepo.CommitAll(message); err != nil {
		return encrypted, fmt.Errorf("failed to commit the encrypted files: %w", err)
	}
	return encrypted, nil
}

// commitAndPush scans the plaintext files about to be committed (secret
// scanner + NDA scanner, each independently skippable), commits, and pushes.
// The push runs even when there is nothing new to commit, so a commit whose
// earlier push failed is retried; a failed push is an error, not a warning.
func (c *PushCommand) commitAndPush(
	repoPath, commitMsg string,
	opts PushOptions,
	config *entities.Config,
) error {
	clean, err := c.gitRepo.IsClean()
	if err != nil {
		return fmt.Errorf("failed to check git status: %w", err)
	}

	if clean {
		logger.Info("no changes to commit")
	} else if err = c.commitPending(repoPath, commitMsg, opts, config); err != nil {
		return err
	}

	if !c.gitRepo.HasRemote() {
		logger.Info("no remote configured, skipping push")
		return nil
	}
	if err = c.gitRepo.Push(); err != nil {
		return fmt.Errorf("committed locally but could not push to the remote "+
			"(the next push retries it): %w", err)
	}
	logger.Info("pushed to remote")
	return nil
}

// commitPending scans the uncommitted plaintext files and commits everything.
func (c *PushCommand) commitPending(
	repoPath, commitMsg string,
	opts PushOptions,
	config *entities.Config,
) error {
	pending, err := c.collectPendingPlaintext(repoPath)
	if err != nil {
		return err
	}
	if err = c.scanContent(repoPath, config, pending, opts); err != nil {
		return err
	}

	if commitMsg == "" {
		hostname, _ := os.Hostname()
		commitMsg = fmt.Sprintf("sync(%s): updated personal configurations", hostname)
	}
	if err = c.gitRepo.CommitAll(commitMsg); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}
	logger.Infof("committed: %s", commitMsg)
	return nil
}

// collectPendingPlaintext reads every uncommitted plaintext file under
// personal/: exactly the unencrypted content the next commit would publish,
// whichever command wrote it (a pull resolving a conflict in favor of the
// local copy also writes into the repo). Encrypted .age files are skipped.
func (c *PushCommand) collectPendingPlaintext(repoPath string) (map[string][]byte, error) {
	changed, err := c.gitRepo.ChangedFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to list uncommitted files: %w", err)
	}

	pending := make(map[string][]byte)
	for _, relPath := range changed {
		if !strings.HasPrefix(relPath, "personal/") || strings.HasSuffix(relPath, ageSuffix) {
			continue
		}
		content, readErr := os.ReadFile(filepath.Join(repoPath, filepath.FromSlash(relPath)))
		if readErr != nil {
			continue // deleted files have nothing to scan
		}
		pending[relPath] = content
	}
	return pending, nil
}

// scanContent runs the secret scanner and then the NDA scanner over files
// about to be published, so users see both classes of finding in one run.
func (c *PushCommand) scanContent(
	repoPath string,
	config *entities.Config,
	plaintext map[string][]byte,
	opts PushOptions,
) error {
	if len(plaintext) == 0 {
		return nil
	}
	if !opts.SkipSecretScan {
		if err := c.runSecretScan(plaintext); err != nil {
			return err
		}
	}
	if !opts.SkipNDAScan && c.ndaChecker != nil {
		if err := c.runNDAScan(repoPath, config, plaintext); err != nil {
			return err
		}
	}
	return nil
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

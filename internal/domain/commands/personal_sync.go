package commands

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

const (
	// manifestVersion is the schema version written into tool manifests.
	manifestVersion = "0.1.0"
	// privateFileMode is the mode of every file the syncer writes.
	privateFileMode os.FileMode = 0o600
	// privateExecutableMode keeps an executable owner-only but runnable.
	privateExecutableMode os.FileMode = 0o700
	// ownerExecuteBit marks a file as executable by its owner.
	ownerExecuteBit os.FileMode = 0o100
)

// ErrSyncAborted is returned when the user quits at a conflict prompt. All
// conflicts are resolved before anything is written, so quitting leaves
// every personal file and the sync repo untouched.
var ErrSyncAborted = errors.New("aborted at a conflict prompt; no personal files were changed")

// PersonalSyncOptions controls one reconciliation of personal files.
type PersonalSyncOptions struct {
	// Direction limits which automatic actions run. Conflicts the user
	// resolves explicitly always run in full, whatever the direction.
	Direction entities.SyncDirection
	// Preference resolves conflicts without prompting.
	Preference entities.SyncPreference
	// NoPrompt skips conflicts that Preference does not resolve instead of
	// asking, for callers such as watch mode that must never block on input.
	NoPrompt bool
	// DryRun prints the plan without touching either side.
	DryRun bool
}

// PersonalSyncReport summarises one reconciliation.
type PersonalSyncReport struct {
	Pushed           int
	Pulled           int
	DeletedLocal     int
	DeletedRepo      int
	ConflictsSkipped int
	PendingIncoming  int
	PendingOutgoing  int
	// Plaintext maps every repo-relative path this run wrote, or in a dry
	// run would write, without encryption to its content.
	Plaintext map[string][]byte
}

// PersonalChange is one difference between a tool directory and the sync
// repo, with both contents and timestamps, as shown by `aisync diff`.
type PersonalChange struct {
	Tool           string
	Entry          entities.SyncEntry
	LocalContent   []byte
	RemoteContent  []byte
	LocalModified  time.Time
	RemoteModified time.Time
	RemoteSummary  string
}

// PersonalSyncer reconciles personal files (rules, agents, commands, skills,
// settings, ...) between each enabled AI tool directory and personal/<tool>/
// in the sync repo. It is a three-way merge: the base is the content this
// device last synced for each path, recorded in the tool's manifest, so a
// change on one side is applied to the other without asking, a deletion on
// one side is applied to the other when the surviving copy is unchanged, and
// only changes on both sides become conflicts.
type PersonalSyncer struct {
	manifestRepo repositories.ManifestRepository
	encryption   repositories.EncryptionService
	applyService repositories.ApplyService
	prompt       repositories.PromptService
	differ       repositories.TextDiffer
	history      repositories.RepoHistory
}

// NewPersonalSyncer creates a new PersonalSyncer.
func NewPersonalSyncer(
	manifestRepo repositories.ManifestRepository,
	encryption repositories.EncryptionService,
	applyService repositories.ApplyService,
	prompt repositories.PromptService,
	differ repositories.TextDiffer,
	history repositories.RepoHistory,
) *PersonalSyncer {
	return &PersonalSyncer{
		manifestRepo: manifestRepo,
		encryption:   encryption,
		applyService: applyService,
		prompt:       prompt,
		differ:       differ,
		history:      history,
	}
}

// localFile is a personal file found in a tool directory.
type localFile struct {
	content    []byte
	checksum   string
	modTime    time.Time
	executable bool
}

// repoFile is a personal file found under personal/<tool>/, already
// decrypted when it is stored as <path>.age.
type repoFile struct {
	content    []byte
	checksum   string
	storedPath string // path under personal/<tool>/ as stored, ".age" included
	executable bool
}

// toolPlan is the reconciliation state of one tool.
type toolPlan struct {
	name     string
	dir      string
	repoDir  string
	manifest *entities.Manifest
	local    map[string]localFile
	remote   map[string]repoFile
	entries  []entities.SyncEntry
	resolved map[string]bool // paths whose conflict was resolved, which run in any direction
	dirty    bool            // the manifest base changed and must be saved
}

// setBase records checksum as the content this device last synced for rel.
func (p *toolPlan) setBase(rel, checksum string) {
	if existing, ok := p.manifest.Files[rel]; ok && existing.Source == "personal" && existing.Checksum == checksum {
		return
	}
	p.manifest.SetFile(rel, "personal", "personal", checksum)
	p.dirty = true
}

// removeBase forgets rel, which is now absent on both sides.
func (p *toolPlan) removeBase(rel string) {
	if _, ok := p.manifest.Files[rel]; ok {
		p.manifest.RemoveFile(rel)
		p.dirty = true
	}
}

// Sync plans every enabled tool, resolves the conflicts, and then executes
// the plan (or prints it, in a dry run).
func (s *PersonalSyncer) Sync(
	config *entities.Config,
	repoPath string,
	opts PersonalSyncOptions,
) (*PersonalSyncReport, error) {
	plans := s.planTools(config, repoPath)
	if err := s.resolveConflicts(repoPath, plans, opts); err != nil {
		return nil, err
	}

	encryptPatterns := readEncryptPatterns(repoPath)
	report := &PersonalSyncReport{Plaintext: make(map[string][]byte)}
	for _, plan := range plans {
		state := &executionState{
			plan:    plan,
			config:  config,
			encrypt: encryptPatterns,
			report:  report,
			pending: make(map[string][]byte),
		}
		if opts.DryRun {
			s.previewTool(repoPath, state, opts)
			continue
		}
		if err := s.executeTool(state, opts); err != nil {
			return report, fmt.Errorf("failed to sync %s: %w", plan.name, err)
		}
	}

	if !opts.DryRun {
		printSyncSummary(report)
	}
	return report, nil
}

// Changes returns every path whose local and repo copies differ, without
// resolving or writing anything.
func (s *PersonalSyncer) Changes(config *entities.Config, repoPath string) []PersonalChange {
	var changes []PersonalChange
	for _, plan := range s.planTools(config, repoPath) {
		for _, entry := range plan.entries {
			if entry.Action == entities.SyncActionNone || entry.Action == entities.SyncActionForget {
				continue
			}
			local := plan.local[entry.Path]
			remote := plan.remote[entry.Path]
			when, subject := s.remoteChange(repoPath, plan, entry.Path)
			changes = append(changes, PersonalChange{
				Tool:           plan.name,
				Entry:          entry,
				LocalContent:   local.content,
				RemoteContent:  remote.content,
				LocalModified:  local.modTime,
				RemoteModified: when,
				RemoteSummary:  subject,
			})
		}
	}
	return changes
}

// planTools plans every enabled tool that exists on this device, in name
// order so prompts and output are deterministic.
func (s *PersonalSyncer) planTools(config *entities.Config, repoPath string) []*toolPlan {
	ignorePatterns := readIgnorePatterns(repoPath)
	identityPath := ExpandHome(config.Encryption.Identity)
	hostname, _ := os.Hostname()

	names := make([]string, 0, len(config.Tools))
	for name, tool := range config.Tools {
		if tool.Enabled {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	plans := make([]*toolPlan, 0, len(names))
	for _, name := range names {
		if plan := s.planTool(name, config.Tools[name], repoPath, ignorePatterns, identityPath, hostname); plan != nil {
			plans = append(plans, plan)
		}
	}
	return plans
}

// planTool scans one tool's local files, repo files, and manifest base and
// reconciles them. It returns nil when the tool is not installed here.
func (s *PersonalSyncer) planTool(
	name string,
	tool entities.Tool,
	repoPath string,
	ignorePatterns *entities.IgnorePatterns,
	identityPath, hostname string,
) *toolPlan {
	toolDir := ExpandHome(tool.Path)
	if info, err := os.Stat(toolDir); err != nil || !info.IsDir() {
		logger.Debugf("tool %s is not installed on this device (%s), skipping personal sync", name, toolDir)
		return nil
	}

	manifest := s.loadManifest(toolDir, hostname)
	sharedPaths := make(map[string]bool)
	for rel, file := range manifest.Files {
		if file.Namespace == "shared" {
			sharedPaths[rel] = true
		}
	}
	inScope := func(rel string) bool {
		return entities.IsSyncable(name, rel, tool.ExtraAllowlist) &&
			!ignorePatterns.Matches(rel) && !sharedPaths[rel]
	}

	plan := &toolPlan{
		name:     name,
		dir:      toolDir,
		repoDir:  filepath.Join(repoPath, "personal", name),
		manifest: manifest,
		local:    scanLocalFiles(toolDir, inScope),
		resolved: make(map[string]bool),
	}
	remote, unreadable := s.scanRepoFiles(plan.repoDir, inScope, identityPath)
	plan.remote = remote

	base := make(map[string]string)
	for rel, file := range manifest.Files {
		if file.Source == "personal" && inScope(rel) {
			base[rel] = file.Checksum
		}
	}
	for rel := range unreadable {
		// A repo copy this device cannot decrypt is neither overwritten nor
		// treated as deleted: the path is left alone on both sides.
		delete(plan.local, rel)
		delete(base, rel)
	}

	plan.entries = entities.Reconcile(localChecksums(plan.local), remoteChecksums(plan.remote), base)
	return plan
}

// loadManifest returns the tool's manifest, or a fresh one when the device
// never synced this tool (or the manifest is unreadable).
func (s *PersonalSyncer) loadManifest(toolDir, hostname string) *entities.Manifest {
	if s.manifestRepo.Exists(toolDir) {
		if manifest, err := s.manifestRepo.Load(toolDir); err == nil {
			return manifest
		}
		logger.Warnf("could not read the aisync manifest in %s; treating this as the first sync", toolDir)
	}
	return entities.NewManifest(manifestVersion, hostname)
}

// walkRoot visits every regular file under dir through an [os.Root], so a
// symlink can never lead the walk (or a read) outside dir, and a path swapped
// for a symlink mid-walk cannot redirect it either. visit receives the
// slash-separated path relative to dir.
func walkRoot(dir string, visit func(fsys fs.FS, rel string, d fs.DirEntry)) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return // a missing directory simply has no files yet
	}
	defer root.Close()

	fsys := root.FS()
	_ = fs.WalkDir(fsys, ".", func(rel string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			logger.Debugf("skipping %s/%s: %v", dir, rel, walkErr)
			return nil
		}
		if !d.IsDir() {
			visit(fsys, rel, d)
		}
		return nil
	})
}

// scanLocalFiles returns every in-scope personal file under toolDir. A
// relative symlink to a file inside toolDir is read through; absolute
// symlinks and links leading outside toolDir are skipped, so a link can
// never publish a file from elsewhere on the disk.
func scanLocalFiles(toolDir string, inScope func(string) bool) map[string]localFile {
	files := make(map[string]localFile)
	walkRoot(toolDir, func(fsys fs.FS, rel string, _ fs.DirEntry) {
		if !inScope(rel) {
			return
		}
		info, statErr := fs.Stat(fsys, rel)
		if statErr != nil || !info.Mode().IsRegular() {
			logger.Debugf("not syncing %s/%s: not a regular file inside the tool directory", toolDir, rel)
			return
		}
		content, readErr := fs.ReadFile(fsys, rel)
		if readErr != nil {
			logger.Warnf("failed to read %s/%s: %v", toolDir, rel, readErr)
			return
		}
		executable := info.Mode().Perm()&ownerExecuteBit != 0
		files[rel] = localFile{
			content:    content,
			checksum:   fileChecksum(content, executable),
			modTime:    info.ModTime(),
			executable: executable,
		}
	})
	return files
}

// scanRepoFiles returns every in-scope file under personal/<tool>/,
// decrypting .age copies. Paths whose ciphertext cannot be decrypted are
// returned separately so the caller leaves them untouched.
func (s *PersonalSyncer) scanRepoFiles(
	repoDir string,
	inScope func(string) bool,
	identityPath string,
) (map[string]repoFile, map[string]bool) {
	files := make(map[string]repoFile)
	unreadable := make(map[string]bool)

	walkRoot(repoDir, func(fsys fs.FS, stored string, d fs.DirEntry) {
		logical, encrypted := strings.CutSuffix(stored, ageSuffix)
		if !inScope(logical) {
			return
		}
		if _, seen := files[logical]; seen && !encrypted {
			return // an encrypted copy always wins over a stray plaintext one
		}

		content, readErr := fs.ReadFile(fsys, stored)
		if readErr != nil {
			logger.Warnf("failed to read %s/%s: %v", repoDir, stored, readErr)
			return
		}
		if encrypted {
			plaintext, decErr := s.encryption.Decrypt(content, identityPath)
			if decErr != nil {
				logger.Warnf(
					"cannot decrypt %s/%s with %s, leaving it untouched: %v",
					repoDir,
					stored,
					identityPath,
					decErr,
				)
				unreadable[logical] = true
				return
			}
			content = plaintext
		}

		executable := false
		if info, infoErr := d.Info(); infoErr == nil {
			executable = info.Mode().Perm()&ownerExecuteBit != 0
		}
		files[logical] = repoFile{
			content:    content,
			checksum:   fileChecksum(content, executable),
			storedPath: stored,
			executable: executable,
		}
	})
	return files, unreadable
}

// resolveConflicts decides every conflict across all tools before anything
// is written, so quitting at a prompt changes nothing.
func (s *PersonalSyncer) resolveConflicts(repoPath string, plans []*toolPlan, opts PersonalSyncOptions) error {
	var sticky entities.ConflictChoice
	for _, plan := range plans {
		for i, entry := range plan.entries {
			if entry.Action != entities.SyncActionConflict {
				continue
			}
			choice := s.chooseResolution(repoPath, plan, entry, opts, &sticky)
			if choice == entities.ChoiceQuit {
				return ErrSyncAborted
			}
			if resolved := entry.Resolve(choice); resolved.Action != entities.SyncActionConflict {
				plan.entries[i] = resolved
				plan.resolved[entry.Path] = true
			}
		}
	}
	return nil
}

// preferenceChoices maps the --prefer values that always keep the same side
// to that side.
//
//nolint:gochecknoglobals // compiled-in lookup table
//exhaustive:ignore // partial by design: ask and newer are decided per conflict
var preferenceChoices = map[entities.SyncPreference]entities.ConflictChoice{
	entities.PreferLocal:  entities.ChoiceKeepLocal,
	entities.PreferRemote: entities.ChoiceKeepRemote,
}

// chooseResolution applies, in order: a "for all" answer given earlier in
// the run, the --prefer flag, and finally an interactive prompt.
func (s *PersonalSyncer) chooseResolution(
	repoPath string,
	plan *toolPlan,
	entry entities.SyncEntry,
	opts PersonalSyncOptions,
	sticky *entities.ConflictChoice,
) entities.ConflictChoice {
	return decideConflict(s.prompt, opts, sticky, func(withDiff bool) entities.ConflictDetail {
		return s.conflictDetail(repoPath, plan, entry, withDiff)
	})
}

// decideConflict is the resolution order shared by every syncer: a "for
// all" answer given earlier in the run, then --prefer, then the prompt.
// detail builds the conflict description, with the diff only when it will be
// shown, since computing it reads both contents.
func decideConflict(
	prompt repositories.PromptService,
	opts PersonalSyncOptions,
	sticky *entities.ConflictChoice,
	detail func(withDiff bool) entities.ConflictDetail,
) entities.ConflictChoice {
	if *sticky != "" {
		return *sticky
	}
	if choice, fixed := preferenceChoices[opts.Preference]; fixed {
		return choice
	}
	if opts.Preference == entities.PreferNewer {
		return detail(false).NewerSide()
	}
	if opts.DryRun || opts.NoPrompt {
		return entities.ChoiceSkip
	}

	choice := prompt.PromptSyncConflict(detail(true))
	if choice == entities.ChoiceKeepLocalAll || choice == entities.ChoiceKeepRemoteAll {
		*sticky = choice
	}
	return choice
}

// conflictDetail gathers both timestamps, the repo commit that last touched
// the path, and (when withDiff is set) the unified diff from repo to local.
func (s *PersonalSyncer) conflictDetail(
	repoPath string,
	plan *toolPlan,
	entry entities.SyncEntry,
	withDiff bool,
) entities.ConflictDetail {
	local := plan.local[entry.Path]
	remote := plan.remote[entry.Path]

	detail := entities.ConflictDetail{
		Tool:          plan.name,
		Path:          entry.Path,
		Reason:        entry.Reason,
		LocalModified: local.modTime,
	}
	detail.RemoteModified, detail.RemoteSummary = s.remoteChange(repoPath, plan, entry.Path)

	if withDiff {
		if isBinary(local.content) || isBinary(remote.content) {
			detail.Binary = true
		} else {
			detail.Diff = s.differ.Unified("repo: "+entry.Path, "local: "+entry.Path, remote.content, local.content)
		}
	}
	return detail
}

// remoteChange returns when the content of the repo copy of rel last changed
// and the subject of that commit. Both stored names are searched together,
// so a file encrypted in place, or deleted under either name, keeps the date
// of its last real change.
func (s *PersonalSyncer) remoteChange(repoPath string, plan *toolPlan, rel string) (time.Time, string) {
	plain := path.Join("personal", plan.name, rel)
	when, subject, err := s.history.LastChange(repoPath, plain, plain+ageSuffix)
	if err != nil {
		logger.Debugf("no history for %s/%s: %v", plan.name, rel, err)
		return time.Time{}, ""
	}
	return when, subject
}

// executionState carries one tool's run: its plan and configuration, the
// shared report, and the incoming files that are applied together, in one
// atomic batch, after every other action ran.
type executionState struct {
	plan        *toolPlan
	config      *entities.Config
	encrypt     *entities.EncryptPatterns
	report      *PersonalSyncReport
	pending     map[string][]byte
	executables []string
	pulled      []entities.SyncEntry
}

// actionHandler executes one plan entry whose action writes to a side.
type actionHandler func(s *PersonalSyncer, state *executionState, entry entities.SyncEntry)

// actionHandlers maps every action that writes to a side to the function
// that performs it; actions missing from the map write nothing.
//
//nolint:gochecknoglobals // compiled-in dispatch table
//exhaustive:ignore // partial by design: actions without a handler write nothing
var actionHandlers = map[entities.SyncAction]actionHandler{
	entities.SyncActionPush:        (*PersonalSyncer).executePush,
	entities.SyncActionDeleteRepo:  (*PersonalSyncer).executeDeleteRepo,
	entities.SyncActionPull:        (*PersonalSyncer).executePull,
	entities.SyncActionDeleteLocal: (*PersonalSyncer).executeDeleteLocal,
}

// executeTool applies one tool's plan to both sides and records the new base
// for every path that ended up in sync.
func (s *PersonalSyncer) executeTool(state *executionState, opts PersonalSyncOptions) error {
	for _, entry := range state.plan.entries {
		handler, writes := actionHandlers[entry.Action]
		switch {
		case !writes:
			state.recordNonWriting(entry)
		case opts.Direction.Allows(entry.Action) || state.plan.resolved[entry.Path]:
			handler(s, state, entry)
		case entry.Action.IsOutgoing():
			state.report.PendingOutgoing++
		default:
			state.report.PendingIncoming++
		}
	}

	err := s.applyPulls(state)
	s.saveManifest(state.plan)
	return err
}

// recordNonWriting handles the actions that leave both sides untouched.
func (st *executionState) recordNonWriting(entry entities.SyncEntry) {
	switch entry.Action {
	case entities.SyncActionNone:
		st.plan.setBase(entry.Path, entry.LocalChecksum)
	case entities.SyncActionForget:
		st.plan.removeBase(entry.Path)
	case entities.SyncActionConflict:
		st.report.ConflictsSkipped++
	case entities.SyncActionPush,
		entities.SyncActionPull,
		entities.SyncActionDeleteRepo,
		entities.SyncActionDeleteLocal:
	}
}

// executePush copies the local version into the repo.
func (s *PersonalSyncer) executePush(state *executionState, entry entities.SyncEntry) {
	if err := s.writeRepoFile(state, entry.Path); err != nil {
		logger.Warnf("failed to write %s/%s to the sync repo: %v", state.plan.name, entry.Path, err)
		return
	}
	state.plan.setBase(entry.Path, entry.LocalChecksum)
	state.report.Pushed++
}

// executeDeleteRepo removes the repo copy of a file deleted locally.
func (*PersonalSyncer) executeDeleteRepo(state *executionState, entry entities.SyncEntry) {
	if err := deleteRepoFile(state.plan, entry.Path); err != nil {
		logger.Warnf("failed to delete %s/%s from the sync repo: %v", state.plan.name, entry.Path, err)
		return
	}
	state.plan.removeBase(entry.Path)
	state.report.DeletedRepo++
}

// executePull queues the repo version for the atomic local apply.
func (*PersonalSyncer) executePull(state *executionState, entry entities.SyncEntry) {
	remote := state.plan.remote[entry.Path]
	localPath := filepath.Join(state.plan.dir, filepath.FromSlash(entry.Path))
	state.pending[localPath] = remote.content
	if remote.executable {
		state.executables = append(state.executables, localPath)
	}
	state.pulled = append(state.pulled, entry)
}

// executeDeleteLocal removes the local copy of a file deleted in the repo.
func (*PersonalSyncer) executeDeleteLocal(state *executionState, entry entities.SyncEntry) {
	localPath := filepath.Join(state.plan.dir, filepath.FromSlash(entry.Path))
	if err := removeFileAndEmptyParents(localPath, state.plan.dir); err != nil {
		logger.Warnf("failed to delete %s: %v", localPath, err)
		return
	}
	state.plan.removeBase(entry.Path)
	state.report.DeletedLocal++
}

// applyPulls writes every queued incoming file in one atomic batch, restores
// executable bits, and records the new base for each.
func (s *PersonalSyncer) applyPulls(state *executionState) error {
	if len(state.pending) == 0 {
		return nil
	}
	if err := s.applyLocal(state.pending); err != nil {
		return err
	}
	for _, file := range state.executables {
		if err := os.Chmod(file, privateExecutableMode); err != nil {
			logger.Warnf("failed to mark %s executable: %v", file, err)
		}
	}
	for _, entry := range state.pulled {
		state.plan.setBase(entry.Path, entry.RemoteChecksum)
		state.report.Pulled++
	}
	return nil
}

// applyLocal writes incoming files byte for byte through the atomic apply
// journal, so an interrupted pull is completed or rolled back on the next
// run, and a file reads back exactly as the base recorded for it.
func (s *PersonalSyncer) applyLocal(pending map[string][]byte) error {
	journal, err := s.applyService.StageVerbatim(pending)
	if err != nil {
		return fmt.Errorf("failed to stage incoming files: %w", err)
	}
	if err = s.applyService.Apply(journal); err != nil {
		return fmt.Errorf("failed to apply incoming files: %w", err)
	}
	return nil
}

// saveManifest persists the base when it changed. A failure only costs the
// base, so the next sync re-detects the same changes; it is logged, not
// returned.
func (s *PersonalSyncer) saveManifest(plan *toolPlan) {
	if !plan.dirty {
		return
	}
	hostname, _ := os.Hostname()
	plan.manifest.Device = hostname
	plan.manifest.LastSync = time.Now()
	if err := s.manifestRepo.Save(plan.dir, plan.manifest); err != nil {
		logger.Warnf("failed to save the aisync manifest for %s: %v", plan.name, err)
	}
}

// writeRepoFile copies the local version of rel into personal/<tool>/,
// encrypting it when .aisyncencrypt matches and recipients are configured,
// and removes the other variant (plaintext or .age) so exactly one copy of
// the path exists in the repo.
func (s *PersonalSyncer) writeRepoFile(state *executionState, rel string) error {
	plan := state.plan
	content, stored, err := s.repoContent(plan, rel, state.config, state.encrypt)
	if err != nil {
		return err
	}

	plainPath := filepath.Join(plan.repoDir, filepath.FromSlash(rel))
	target := filepath.Join(plan.repoDir, filepath.FromSlash(stored))
	stale := plainPath + ageSuffix
	if target != plainPath {
		stale = plainPath
	}

	mode := privateFileMode
	if plan.local[rel].executable {
		mode = privateExecutableMode
	}
	if err = os.MkdirAll(filepath.Dir(target), privateExecutableMode); err != nil {
		return err
	}
	if err = os.WriteFile(target, content, mode); err != nil {
		return err
	}
	if err = os.Chmod(target, mode); err != nil {
		return err
	}
	if err = os.Remove(stale); err != nil && !os.IsNotExist(err) {
		return err
	}

	if target == plainPath {
		state.report.Plaintext[path.Join("personal", plan.name, rel)] = content
	}
	return nil
}

// repoContent returns the bytes to store for rel and the path to store them
// under: the ciphertext and "<rel>.age" when the file must be encrypted,
// otherwise the plaintext and rel itself.
func (s *PersonalSyncer) repoContent(
	plan *toolPlan,
	rel string,
	config *entities.Config,
	encryptPatterns *entities.EncryptPatterns,
) ([]byte, string, error) {
	plaintext := plan.local[rel].content
	if !encryptPatterns.Matches(encryptMatchPath(plan.name, rel)) {
		return plaintext, rel, nil
	}
	if len(config.Encryption.Recipients) == 0 {
		logger.Warnf(
			"file %s matches an encrypt pattern but no recipients are configured; "+
				"writing as plaintext. Run `aisync key generate` or add a recipient to config.yaml.",
			encryptMatchPath(plan.name, rel),
		)
		return plaintext, rel, nil
	}
	ciphertext, err := s.encryption.Encrypt(plaintext, config.Encryption.Recipients)
	if err != nil {
		return nil, "", fmt.Errorf("failed to encrypt: %w", err)
	}
	return ciphertext, rel + ageSuffix, nil
}

// deleteRepoFile removes both stored variants of rel from personal/<tool>/
// and any directories that become empty.
func deleteRepoFile(plan *toolPlan, rel string) error {
	plainPath := filepath.Join(plan.repoDir, filepath.FromSlash(rel))
	if err := removeFileAndEmptyParents(plainPath+ageSuffix, plan.repoDir); err != nil {
		return err
	}
	return removeFileAndEmptyParents(plainPath, plan.repoDir)
}

// removeFileAndEmptyParents deletes file (a missing file is not an error)
// and then every directory between it and stop that is left empty.
func removeFileAndEmptyParents(file, stop string) error {
	if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
		return err
	}
	pruneEmptyParents(filepath.Dir(file), filepath.Clean(stop))
	return nil
}

// pruneEmptyParents removes dir and its ancestors below stop while they are
// empty. [os.Remove] refuses a non-empty directory, which ends the climb.
func pruneEmptyParents(dir, stop string) {
	for ; strings.HasPrefix(dir, stop+string(os.PathSeparator)); dir = filepath.Dir(dir) {
		if os.Remove(dir) != nil {
			return
		}
	}
}

// previewTool prints what executing the plan would do, and fills the
// report's counters and plaintext map as if it had run.
func (s *PersonalSyncer) previewTool(repoPath string, state *executionState, opts PersonalSyncOptions) {
	printedHeader := false
	for _, entry := range state.plan.entries {
		if entry.Action == entities.SyncActionNone || entry.Action == entities.SyncActionForget {
			continue
		}
		if !printedHeader {
			fmt.Fprintf(os.Stdout, "%s:\n", state.plan.name)
			printedHeader = true
		}
		runs := opts.Direction.Allows(entry.Action) || state.plan.resolved[entry.Path]
		s.previewEntry(repoPath, state, entry, runs)
	}
}

// previewEntry prints one plan entry and counts it the way executing it would.
func (s *PersonalSyncer) previewEntry(repoPath string, state *executionState, entry entities.SyncEntry, runs bool) {
	label, description := describeEntry(entry)
	if entry.Action != entities.SyncActionConflict && !runs {
		description += " (not applied in this direction)"
	}
	fmt.Fprintf(os.Stdout, "  %-8s %s  %s\n", label, entry.Path, description)

	report := state.report
	switch {
	case entry.Action == entities.SyncActionConflict:
		report.ConflictsSkipped++
		fmt.Fprintf(
			os.Stdout,
			"           %s\n",
			describeTimestamps(s.conflictDetail(repoPath, state.plan, entry, false)),
		)
	case !runs && entry.Action.IsOutgoing():
		report.PendingOutgoing++
	case !runs:
		report.PendingIncoming++
	default:
		if counter, counted := previewCounters[entry.Action]; counted {
			*counter(report)++
		}
		s.previewPlaintext(state, entry)
	}
}

// previewPlaintext records, for a push a dry run would make, the content the
// commit would publish unencrypted.
func (s *PersonalSyncer) previewPlaintext(state *executionState, entry entities.SyncEntry) {
	if entry.Action != entities.SyncActionPush {
		return
	}
	content, stored, err := s.repoContent(state.plan, entry.Path, state.config, state.encrypt)
	if err != nil || stored != entry.Path {
		return // an encrypted (or unencryptable) file publishes no plaintext
	}
	state.report.Plaintext[path.Join("personal", state.plan.name, entry.Path)] = content
}

// previewCounters maps every action a dry run counts to its report counter.
//
//nolint:gochecknoglobals // compiled-in lookup table
//exhaustive:ignore // partial by design: only actions that write are counted
var previewCounters = map[entities.SyncAction]func(*PersonalSyncReport) *int{
	entities.SyncActionPush:        func(r *PersonalSyncReport) *int { return &r.Pushed },
	entities.SyncActionPull:        func(r *PersonalSyncReport) *int { return &r.Pulled },
	entities.SyncActionDeleteRepo:  func(r *PersonalSyncReport) *int { return &r.DeletedRepo },
	entities.SyncActionDeleteLocal: func(r *PersonalSyncReport) *int { return &r.DeletedLocal },
}

// entryDescriptions maps each action shown to the user to its column label
// and explanation.
//
//nolint:gochecknoglobals // compiled-in lookup table
//exhaustive:ignore // partial by design: unchanged paths are never shown
var entryDescriptions = map[entities.SyncAction]func(entities.SyncEntry) (string, string){
	entities.SyncActionPush: func(e entities.SyncEntry) (string, string) {
		return "→ repo", newOrChanged(e, "new here", "changed here")
	},
	entities.SyncActionPull: func(e entities.SyncEntry) (string, string) {
		return "← repo", newOrChanged(e, "new in the repo", "changed in the repo")
	},
	entities.SyncActionDeleteRepo: func(entities.SyncEntry) (string, string) {
		return "✗ repo", "deleted here"
	},
	entities.SyncActionDeleteLocal: func(entities.SyncEntry) (string, string) {
		return "✗ here", "deleted in the repo"
	},
	entities.SyncActionConflict: func(e entities.SyncEntry) (string, string) {
		return "! both", "conflict: " + e.Reason.Describe()
	},
}

// describeEntry returns the column label and explanation printed for an entry.
func describeEntry(entry entities.SyncEntry) (string, string) {
	if describe, ok := entryDescriptions[entry.Action]; ok {
		return describe(entry)
	}
	return "", ""
}

// newOrChanged picks the wording for a file that was never synced versus one
// that changed since the last sync.
func newOrChanged(entry entities.SyncEntry, isNew, changed string) string {
	if entry.BaseChecksum == "" {
		return isNew
	}
	return changed
}

// describeTimestamps renders when each side of a conflict last changed.
func describeTimestamps(detail entities.ConflictDetail) string {
	local := "absent"
	if !detail.LocalModified.IsZero() {
		local = detail.LocalModified.Local().Format("2006-01-02 15:04")
	}
	remote := "unknown"
	if !detail.RemoteModified.IsZero() {
		remote = detail.RemoteModified.Local().Format("2006-01-02 15:04")
		if detail.RemoteSummary != "" {
			remote += " (" + detail.RemoteSummary + ")"
		}
	}
	return fmt.Sprintf("here: %s · repo: %s", local, remote)
}

// printSyncSummary prints one line describing what a real run did.
func printSyncSummary(report *PersonalSyncReport) {
	fmt.Fprintf(os.Stdout,
		"Personal files: %d sent to the repo, %d received, %d deleted here, %d deleted in the repo",
		report.Pushed, report.Pulled, report.DeletedLocal, report.DeletedRepo)
	if report.ConflictsSkipped > 0 {
		fmt.Fprintf(os.Stdout, ", %d conflict(s) skipped", report.ConflictsSkipped)
	}
	if pending := report.PendingIncoming + report.PendingOutgoing; pending > 0 {
		fmt.Fprintf(os.Stdout, ", %d change(s) left for the other direction", pending)
	}
	fmt.Fprintln(os.Stdout, ".")
	if report.ConflictsSkipped > 0 {
		fmt.Fprintln(os.Stdout,
			"Skipped conflicts are reported again on the next sync; resolve them in a terminal or with --prefer.")
	}
}

// executableSuffix marks the checksum of an executable file, so a change of
// the executable bit alone syncs like a content change (a hook committed
// without it would otherwise reach other devices unable to run).
const executableSuffix = "+x"

// fileChecksum identifies a personal file by its content and executable bit.
// Non-executable files keep the plain content checksum.
func fileChecksum(content []byte, executable bool) string {
	if executable {
		return checksumBytes(content) + executableSuffix
	}
	return checksumBytes(content)
}

// localChecksums projects scanned local files onto their checksums.
func localChecksums(files map[string]localFile) map[string]string {
	checksums := make(map[string]string, len(files))
	for rel, file := range files {
		checksums[rel] = file.checksum
	}
	return checksums
}

// remoteChecksums projects scanned repo files onto their checksums.
func remoteChecksums(files map[string]repoFile) map[string]string {
	checksums := make(map[string]string, len(files))
	for rel, file := range files {
		checksums[rel] = file.checksum
	}
	return checksums
}

// isBinary reports whether content should not be shown as a text diff.
func isBinary(content []byte) bool {
	return bytes.IndexByte(content, 0) >= 0 || !utf8.Valid(content)
}

// readIgnorePatterns reads .aisyncignore from the sync repo root.
func readIgnorePatterns(repoPath string) *entities.IgnorePatterns {
	content, err := os.ReadFile(filepath.Join(repoPath, ".aisyncignore"))
	if err != nil {
		return entities.ParseIgnorePatterns([]byte{})
	}
	return entities.ParseIgnorePatterns(content)
}

// readEncryptPatterns reads .aisyncencrypt from the sync repo root.
func readEncryptPatterns(repoPath string) *entities.EncryptPatterns {
	content, err := os.ReadFile(filepath.Join(repoPath, ".aisyncencrypt"))
	if err != nil {
		return entities.ParseEncryptPatterns([]byte{})
	}
	return entities.ParseEncryptPatterns(content)
}

package commands

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// transcriptPublishAttempts bounds how often one sync fetches and merges
// again after another device published conversations in the meantime.
const transcriptPublishAttempts = 3

// TranscriptSyncer syncs conversation transcripts, and the files kept next to
// them such as project memories, through the transcript snapshot branch.
// It applies the same three-way merge as personal files, plus two rules for
// append-only logs: a log untouched for longer than the spec's max_age leaves
// the snapshot on every device, and when two devices extended the same log
// the copy that contains the other wins without asking.
type TranscriptSyncer struct {
	service      repositories.TranscriptService
	baseRepo     repositories.TranscriptBaseRepository
	applyService repositories.ApplyService
	prompt       repositories.PromptService
	differ       repositories.TextDiffer
}

// NewTranscriptSyncer creates a new TranscriptSyncer.
func NewTranscriptSyncer(
	service repositories.TranscriptService,
	baseRepo repositories.TranscriptBaseRepository,
	applyService repositories.ApplyService,
	prompt repositories.PromptService,
	differ repositories.TextDiffer,
) *TranscriptSyncer {
	return &TranscriptSyncer{
		service:      service,
		baseRepo:     baseRepo,
		applyService: applyService,
		prompt:       prompt,
		differ:       differ,
	}
}

// transcriptTool is one installed tool whose conversations are synced.
type transcriptTool struct {
	name   string
	dir    string // absolute path of the tool's transcript directory
	spec   entities.TranscriptSpec
	maxAge time.Duration
}

// transcriptLocal is a conversation file found on this device. Its content
// is only read once the file is known to be in the merge.
type transcriptLocal struct {
	path     string
	modTime  time.Time
	content  []byte
	checksum string
}

// transcriptRun is the state of one fetch, merge, and publish attempt.
type transcriptRun struct {
	tools    map[string]transcriptTool
	snapshot *entities.TranscriptSnapshot
	base     map[string]string
	local    map[string]transcriptLocal
	remote   map[string]entities.TranscriptFile
	kinds    map[string]entities.TranscriptKind
	expired  []string // keys that leave the snapshot and the base
	entries  []entities.SyncEntry
	resolved map[string]bool
	contents map[string][]byte // remote contents already read, by key
}

// Sync merges this device's conversations with the snapshot branch and,
// unless only pulling, publishes the result. When another device publishes
// first, it fetches and merges again instead of overwriting that device.
func (s *TranscriptSyncer) Sync(
	config *entities.Config,
	repoPath string,
	opts PersonalSyncOptions,
) (*PersonalSyncReport, error) {
	report := &PersonalSyncReport{Plaintext: make(map[string][]byte)}
	tools, err := transcriptTools(config)
	if err != nil || len(tools) == 0 {
		return report, err
	}

	for attempt := 1; ; attempt++ {
		report, err = s.syncOnce(config, repoPath, tools, opts)
		if !errors.Is(err, repositories.ErrSnapshotMoved) || attempt == transcriptPublishAttempts {
			return report, err
		}
		logger.Info("another device published conversations meanwhile; merging again")
	}
}

// transcriptTools returns the enabled, installed tools with a transcripts spec.
func transcriptTools(config *entities.Config) (map[string]transcriptTool, error) {
	tools := make(map[string]transcriptTool)
	for name, tool := range config.Tools {
		if !tool.Enabled || tool.Transcripts == nil {
			continue
		}
		maxAge, err := tool.Transcripts.MaxAgeDuration()
		if err != nil {
			return nil, fmt.Errorf("tool %s: %w", name, err)
		}
		dir := filepath.Join(ExpandHome(tool.Path), tool.Transcripts.Dir)
		if info, statErr := os.Stat(dir); statErr != nil || !info.IsDir() {
			logger.Debugf("no conversations for %s on this device (%s), skipping", name, dir)
			continue
		}
		tools[name] = transcriptTool{name: name, dir: dir, spec: *tool.Transcripts, maxAge: maxAge}
	}
	return tools, nil
}

// syncOnce is a single fetch, merge, and publish attempt.
func (s *TranscriptSyncer) syncOnce(
	config *entities.Config,
	repoPath string,
	tools map[string]transcriptTool,
	opts PersonalSyncOptions,
) (*PersonalSyncReport, error) {
	identityPath := ExpandHome(config.Encryption.Identity)
	snapshot, err := s.service.Fetch(repoPath, config.Sync.EffectiveTranscriptsBranch(), identityPath)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch conversations: %w", err)
	}
	base, err := s.baseRepo.Load(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load the conversation sync state: %w", err)
	}

	run := planTranscripts(tools, snapshot, base, time.Now())
	if err = s.resolveTranscripts(repoPath, run, identityPath, opts); err != nil {
		return nil, err
	}

	report := &PersonalSyncReport{Plaintext: make(map[string][]byte)}
	if opts.DryRun {
		previewTranscripts(run, opts, report)
		return report, nil
	}
	if err = s.executeTranscripts(repoPath, run, config, identityPath, opts, report); err != nil {
		return report, err
	}
	printTranscriptSummary(report, len(run.expired))
	return report, nil
}

// planTranscripts scans this device, drops expired logs on both sides, and
// reconciles the rest against the base. Files of tools not synced on this
// device stay in the snapshot untouched.
func planTranscripts(
	tools map[string]transcriptTool,
	snapshot *entities.TranscriptSnapshot,
	base map[string]string,
	now time.Time,
) *transcriptRun {
	run := &transcriptRun{
		tools:    tools,
		snapshot: snapshot,
		base:     maps.Clone(base),
		local:    make(map[string]transcriptLocal),
		remote:   make(map[string]entities.TranscriptFile),
		kinds:    make(map[string]entities.TranscriptKind),
		resolved: make(map[string]bool),
		contents: make(map[string][]byte),
	}
	if run.base == nil {
		run.base = make(map[string]string)
	}

	for _, tool := range tools {
		scanTranscriptDir(tool, run)
	}
	for key, file := range snapshot.Files {
		tool, rel, synced := run.toolOf(key)
		if !synced {
			continue
		}
		kind := tool.spec.Classify(rel)
		if kind == entities.TranscriptKindNone {
			run.expired = append(run.expired, key) // no longer covered by the spec
			continue
		}
		run.kinds[key] = kind
		run.remote[key] = file
	}

	run.dropExpiredLogs(now)
	run.loadLocalContents()

	base = make(map[string]string)
	for key, checksum := range run.base {
		if _, _, synced := run.toolOf(key); synced {
			base[key] = checksum
		}
	}
	run.entries = entities.Reconcile(transcriptLocalChecksums(run.local), transcriptRemoteChecksums(run.remote), base)
	return run
}

// toolOf splits a snapshot key into its tool and the path relative to the
// tool's transcript directory, reporting whether that tool is synced here.
func (r *transcriptRun) toolOf(key string) (transcriptTool, string, bool) {
	name, rel, found := strings.Cut(key, "/")
	if !found {
		return transcriptTool{}, "", false
	}
	tool, synced := r.tools[name]
	return tool, rel, synced
}

// scanTranscriptDir records every file of tool's spec on this device,
// without reading contents yet.
func scanTranscriptDir(tool transcriptTool, run *transcriptRun) {
	walkRoot(tool.dir, func(fsys fs.FS, rel string, _ fs.DirEntry) {
		kind := tool.spec.Classify(rel)
		if kind == entities.TranscriptKindNone {
			return
		}
		info, err := fs.Stat(fsys, rel)
		if err != nil || !info.Mode().IsRegular() {
			return
		}
		key := tool.name + "/" + rel
		run.kinds[key] = kind
		run.local[key] = transcriptLocal{
			path:    filepath.Join(tool.dir, filepath.FromSlash(rel)),
			modTime: info.ModTime(),
		}
	})
}

// dropExpiredLogs removes logs whose newest copy, on either side, is older
// than the tool's max age. Judging both sides together keeps expiry
// symmetric: a log continued on another device is never dropped here, and
// an expired log is never mistaken for a deletion.
func (r *transcriptRun) dropExpiredLogs(now time.Time) {
	keys := make(map[string]struct{}, len(r.local)+len(r.remote))
	for key := range r.local {
		keys[key] = struct{}{}
	}
	for key := range r.remote {
		keys[key] = struct{}{}
	}

	for key := range keys {
		tool, _, _ := r.toolOf(key)
		if r.kinds[key] != entities.TranscriptKindLog || tool.maxAge == 0 {
			continue
		}
		newest := r.local[key].modTime
		if remote, ok := r.remote[key]; ok && remote.ModTime.After(newest) {
			newest = remote.ModTime
		}
		if now.Sub(newest) <= tool.maxAge {
			continue
		}
		if _, inSnapshot := r.remote[key]; inSnapshot {
			r.expired = append(r.expired, key)
		}
		delete(r.local, key)
		delete(r.remote, key)
		delete(r.base, key)
	}
}

// loadLocalContents reads and checksums the local files that remain in the
// merge; a file that cannot be read is left out of this run.
func (r *transcriptRun) loadLocalContents() {
	for key, local := range r.local {
		content, err := os.ReadFile(local.path)
		if err != nil {
			logger.Warnf("failed to read %s, skipping it this time: %v", local.path, err)
			delete(r.local, key)
			continue
		}
		local.content = content
		local.checksum = checksumBytes(content)
		r.local[key] = local
	}
}

// resolveTranscripts decides every conflict before anything is written.
// Logs resolve themselves; other files go through the shared decision order.
func (s *TranscriptSyncer) resolveTranscripts(
	repoPath string,
	run *transcriptRun,
	identityPath string,
	opts PersonalSyncOptions,
) error {
	var sticky entities.ConflictChoice
	for i, entry := range run.entries {
		if entry.Action != entities.SyncActionConflict {
			continue
		}

		var choice entities.ConflictChoice
		if run.kinds[entry.Path] == entities.TranscriptKindLog {
			choice = s.resolveLog(repoPath, run, entry, identityPath)
		} else {
			choice = decideConflict(s.prompt, opts, &sticky, func(withDiff bool) entities.ConflictDetail {
				return s.transcriptConflictDetail(repoPath, run, entry, identityPath, withDiff)
			})
		}
		if choice == entities.ChoiceQuit {
			return ErrSyncAborted
		}
		if resolved := entry.Resolve(choice); resolved.Action != entities.SyncActionConflict {
			run.entries[i] = resolved
			run.resolved[entry.Path] = true
		}
	}
	return nil
}

// resolveLog settles a conversation log changed on both sides. A change
// beats a deletion; when one copy extends the other (a conversation
// continued on one device), the longer copy wins; otherwise the copy that
// changed last wins and the divergence is reported.
func (s *TranscriptSyncer) resolveLog(
	repoPath string,
	run *transcriptRun,
	entry entities.SyncEntry,
	identityPath string,
) entities.ConflictChoice {
	local, hasLocal := run.local[entry.Path]
	remote, hasRemote := run.remote[entry.Path]
	switch {
	case !hasLocal:
		return entities.ChoiceKeepRemote
	case !hasRemote:
		return entities.ChoiceKeepLocal
	}

	remoteContent, err := s.remoteContent(repoPath, run, entry.Path, identityPath)
	if err != nil {
		logger.Warnf("cannot read the synced copy of %s, keeping this device's: %v", entry.Path, err)
		return entities.ChoiceKeepLocal
	}
	switch {
	case bytes.HasPrefix(remoteContent, local.content):
		return entities.ChoiceKeepRemote
	case bytes.HasPrefix(local.content, remoteContent):
		return entities.ChoiceKeepLocal
	case remote.ModTime.After(local.modTime):
		logger.Warnf(
			"conversation %s diverged between devices; kept the newer copy, from %s",
			entry.Path,
			remote.Device,
		)
		return entities.ChoiceKeepRemote
	default:
		logger.Warnf("conversation %s diverged between devices; kept this device's newer copy", entry.Path)
		return entities.ChoiceKeepLocal
	}
}

// transcriptConflictDetail describes a conflict on a non-log file, such as a
// project memory, for the shared conflict review.
func (s *TranscriptSyncer) transcriptConflictDetail(
	repoPath string,
	run *transcriptRun,
	entry entities.SyncEntry,
	identityPath string,
	withDiff bool,
) entities.ConflictDetail {
	tool, rel, _ := run.toolOf(entry.Path)
	local := run.local[entry.Path]
	remote := run.remote[entry.Path]

	detail := entities.ConflictDetail{
		Tool:           tool.name,
		Path:           path.Join(tool.spec.Dir, rel),
		Reason:         entry.Reason,
		LocalModified:  local.modTime,
		RemoteModified: remote.ModTime,
	}
	if remote.Device != "" {
		detail.RemoteSummary = "from " + remote.Device
	}
	if !withDiff {
		return detail
	}

	var remoteContent []byte
	if entry.RemoteChecksum != "" {
		content, err := s.remoteContent(repoPath, run, entry.Path, identityPath)
		if err != nil {
			logger.Warnf("cannot read the synced copy of %s for the diff: %v", entry.Path, err)
		}
		remoteContent = content
	}
	if isBinary(local.content) || isBinary(remoteContent) {
		detail.Binary = true
	} else {
		detail.Diff = s.differ.Unified("repo: "+detail.Path, "local: "+detail.Path, remoteContent, local.content)
	}
	return detail
}

// remoteContent reads (once) the decrypted snapshot copy of key.
func (s *TranscriptSyncer) remoteContent(
	repoPath string,
	run *transcriptRun,
	key, identityPath string,
) ([]byte, error) {
	if content, cached := run.contents[key]; cached {
		return content, nil
	}
	content, err := s.service.Read(repoPath, run.snapshot.Lease, run.remote[key], identityPath)
	if err != nil {
		return nil, err
	}
	run.contents[key] = content
	return content, nil
}

// transcriptExecution accumulates one attempt's changes: the next snapshot,
// the incoming files to apply, and base updates that wait for the publish.
type transcriptExecution struct {
	run          *transcriptRun
	next         *entities.TranscriptSnapshot
	published    bool              // the next snapshot differs from the fetched one
	pending      map[string][]byte // incoming files to apply, by absolute path
	pulled       []entities.SyncEntry
	afterPublish map[string]string // base updates for pushed (checksum) or removed ("") keys
	report       *PersonalSyncReport
}

// transcriptHandler executes one plan entry whose action writes to a side.
type transcriptHandler func(s *TranscriptSyncer, exec *transcriptExecution, repoPath, identityPath string, entry entities.SyncEntry)

// transcriptHandlers maps every action that writes to a side to the function
// that performs it; actions missing from the map write nothing.
//
//nolint:gochecknoglobals // compiled-in dispatch table
//exhaustive:ignore // partial by design: actions without a handler write nothing
var transcriptHandlers = map[entities.SyncAction]transcriptHandler{
	entities.SyncActionPush:        (*TranscriptSyncer).executeTranscriptPush,
	entities.SyncActionDeleteRepo:  (*TranscriptSyncer).executeTranscriptDeleteRepo,
	entities.SyncActionPull:        (*TranscriptSyncer).executeTranscriptPull,
	entities.SyncActionDeleteLocal: (*TranscriptSyncer).executeTranscriptDeleteLocal,
}

// executeTranscripts applies the plan: incoming files first, then the new
// snapshot is published, then the base is saved for everything that ended up
// in sync. A pull never publishes; dropping expired logs waits for a push.
func (s *TranscriptSyncer) executeTranscripts(
	repoPath string,
	run *transcriptRun,
	config *entities.Config,
	identityPath string,
	opts PersonalSyncOptions,
	report *PersonalSyncReport,
) error {
	exec := &transcriptExecution{
		run:          run,
		next:         &entities.TranscriptSnapshot{Lease: run.snapshot.Lease, Files: maps.Clone(run.snapshot.Files)},
		pending:      make(map[string][]byte),
		afterPublish: make(map[string]string),
		report:       report,
	}
	if exec.next.Files == nil {
		exec.next.Files = make(map[string]entities.TranscriptFile)
	}
	for _, key := range run.expired {
		delete(exec.next.Files, key)
		exec.published = true
	}

	for _, entry := range run.entries {
		handler, writes := transcriptHandlers[entry.Action]
		switch {
		case !writes:
			exec.recordNonWriting(entry)
		case opts.Direction.Allows(entry.Action) || run.resolved[entry.Path]:
			handler(s, exec, repoPath, identityPath, entry)
		case entry.Action.IsOutgoing():
			report.PendingOutgoing++
		default:
			report.PendingIncoming++
		}
	}

	if err := s.applyTranscriptPulls(exec); err != nil {
		s.saveTranscriptBase(repoPath, run.base)
		return err
	}
	if exec.published && opts.Direction != entities.SyncIncoming {
		if err := s.publish(repoPath, config, identityPath, exec); err != nil {
			s.saveTranscriptBase(repoPath, run.base)
			return err
		}
	}
	s.saveTranscriptBase(repoPath, run.base)
	return nil
}

// recordNonWriting handles the actions that leave both sides untouched.
func (e *transcriptExecution) recordNonWriting(entry entities.SyncEntry) {
	switch entry.Action {
	case entities.SyncActionNone:
		e.run.base[entry.Path] = entry.LocalChecksum
	case entities.SyncActionForget:
		delete(e.run.base, entry.Path)
	case entities.SyncActionConflict:
		e.report.ConflictsSkipped++
	case entities.SyncActionPush,
		entities.SyncActionPull,
		entities.SyncActionDeleteRepo,
		entities.SyncActionDeleteLocal:
	}
}

// executeTranscriptPush puts this device's copy into the next snapshot.
func (*TranscriptSyncer) executeTranscriptPush(exec *transcriptExecution, _, _ string, entry entities.SyncEntry) {
	local := exec.run.local[entry.Path]
	hostname, _ := os.Hostname()
	exec.next.Files[entry.Path] = entities.TranscriptFile{
		Key:      entry.Path,
		Checksum: local.checksum,
		ModTime:  local.modTime,
		Device:   hostname,
		Content:  local.content,
	}
	exec.published = true
	exec.afterPublish[entry.Path] = local.checksum
	exec.report.Pushed++
}

// executeTranscriptDeleteRepo removes a file deleted here from the next snapshot.
func (*TranscriptSyncer) executeTranscriptDeleteRepo(exec *transcriptExecution, _, _ string, entry entities.SyncEntry) {
	delete(exec.next.Files, entry.Path)
	exec.published = true
	exec.afterPublish[entry.Path] = ""
	exec.report.DeletedRepo++
}

// executeTranscriptPull queues the snapshot copy for the atomic local apply.
func (s *TranscriptSyncer) executeTranscriptPull(
	exec *transcriptExecution,
	repoPath, identityPath string,
	entry entities.SyncEntry,
) {
	content, err := s.remoteContent(repoPath, exec.run, entry.Path, identityPath)
	if err != nil {
		logger.Warnf("failed to read the synced copy of %s: %v", entry.Path, err)
		return
	}
	tool, rel, _ := exec.run.toolOf(entry.Path)
	exec.pending[filepath.Join(tool.dir, filepath.FromSlash(rel))] = content
	exec.pulled = append(exec.pulled, entry)
}

// executeTranscriptDeleteLocal removes this device's copy of a file deleted
// on another device.
func (*TranscriptSyncer) executeTranscriptDeleteLocal(
	exec *transcriptExecution,
	_, _ string,
	entry entities.SyncEntry,
) {
	tool, rel, _ := exec.run.toolOf(entry.Path)
	localPath := filepath.Join(tool.dir, filepath.FromSlash(rel))
	if err := removeFileAndEmptyParents(localPath, tool.dir); err != nil {
		logger.Warnf("failed to delete %s: %v", localPath, err)
		return
	}
	delete(exec.run.base, entry.Path)
	exec.report.DeletedLocal++
}

// applyTranscriptPulls writes every incoming file byte for byte in one atomic
// batch and gives each the modification time it had on the device that
// published it, so expiry and the tool's own cleanup see the conversation's
// real age, and the file reads back exactly as its recorded base.
func (s *TranscriptSyncer) applyTranscriptPulls(exec *transcriptExecution) error {
	if len(exec.pending) == 0 {
		return nil
	}
	journal, err := s.applyService.StageVerbatim(exec.pending)
	if err != nil {
		return fmt.Errorf("failed to stage incoming conversations: %w", err)
	}
	if err = s.applyService.Apply(journal); err != nil {
		return fmt.Errorf("failed to apply incoming conversations: %w", err)
	}

	for _, entry := range exec.pulled {
		remote := exec.run.remote[entry.Path]
		tool, rel, _ := exec.run.toolOf(entry.Path)
		localPath := filepath.Join(tool.dir, filepath.FromSlash(rel))
		if !remote.ModTime.IsZero() {
			if chErr := os.Chtimes(localPath, remote.ModTime, remote.ModTime); chErr != nil {
				logger.Debugf("could not keep the modification time of %s: %v", localPath, chErr)
			}
		}
		exec.run.base[entry.Path] = entry.RemoteChecksum
		exec.report.Pulled++
	}
	return nil
}

// publish replaces the snapshot branch with the next snapshot and then
// records the base for every pushed or removed file.
func (s *TranscriptSyncer) publish(
	repoPath string,
	config *entities.Config,
	identityPath string,
	exec *transcriptExecution,
) error {
	if len(config.Encryption.Recipients) == 0 {
		return errors.New("conversations are only published encrypted; add an age recipient to config.yaml")
	}
	hostname, _ := os.Hostname()
	message := fmt.Sprintf("sync(%s): updated conversations", hostname)
	err := s.service.Publish(repoPath, config.Sync.EffectiveTranscriptsBranch(), exec.next,
		config.Encryption.Recipients, identityPath, message)
	if err != nil {
		if errors.Is(err, repositories.ErrSnapshotMoved) {
			return err
		}
		return fmt.Errorf("failed to publish conversations: %w", err)
	}

	for key, checksum := range exec.afterPublish {
		if checksum == "" {
			delete(exec.run.base, key)
		} else {
			exec.run.base[key] = checksum
		}
	}
	return nil
}

// saveTranscriptBase persists the base; a failure only costs the base, so
// the next sync re-detects the same changes.
func (s *TranscriptSyncer) saveTranscriptBase(repoPath string, base map[string]string) {
	if err := s.baseRepo.Save(repoPath, base); err != nil {
		logger.Warnf("failed to save the conversation sync state: %v", err)
	}
}

// previewTranscripts prints what executing the plan would do.
func previewTranscripts(run *transcriptRun, opts PersonalSyncOptions, report *PersonalSyncReport) {
	keys := make([]string, 0, len(run.entries))
	entries := make(map[string]entities.SyncEntry, len(run.entries))
	for _, entry := range run.entries {
		if entry.Action == entities.SyncActionNone || entry.Action == entities.SyncActionForget {
			continue
		}
		keys = append(keys, entry.Path)
		entries[entry.Path] = entry
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		fmt.Fprintln(os.Stdout, "conversations:")
	}

	for _, key := range keys {
		entry := entries[key]
		runs := opts.Direction.Allows(entry.Action) || run.resolved[key]
		label, description := describeEntry(entry)
		if entry.Action != entities.SyncActionConflict && !runs {
			description += " (not applied in this direction)"
		}
		fmt.Fprintf(os.Stdout, "  %-8s %s  %s\n", label, key, description)

		switch {
		case entry.Action == entities.SyncActionConflict:
			report.ConflictsSkipped++
		case !runs && entry.Action.IsOutgoing():
			report.PendingOutgoing++
		case !runs:
			report.PendingIncoming++
		default:
			if counter, counted := previewCounters[entry.Action]; counted {
				*counter(report)++
			}
		}
	}
	if len(run.expired) > 0 {
		fmt.Fprintf(os.Stdout, "  %d conversation file(s) past max_age would leave the snapshot\n", len(run.expired))
	}
}

// printTranscriptSummary prints one line describing what a real run did.
func printTranscriptSummary(report *PersonalSyncReport, expired int) {
	fmt.Fprintf(os.Stdout,
		"Conversations: %d sent, %d received, %d deleted here, %d deleted in the snapshot, %d expired",
		report.Pushed, report.Pulled, report.DeletedLocal, report.DeletedRepo, expired)
	if report.ConflictsSkipped > 0 {
		fmt.Fprintf(os.Stdout, ", %d conflict(s) skipped", report.ConflictsSkipped)
	}
	fmt.Fprintln(os.Stdout, ".")
}

// transcriptLocalChecksums projects local conversation files onto checksums.
func transcriptLocalChecksums(files map[string]transcriptLocal) map[string]string {
	checksums := make(map[string]string, len(files))
	for key, file := range files {
		checksums[key] = file.checksum
	}
	return checksums
}

// transcriptRemoteChecksums projects snapshot files onto checksums.
func transcriptRemoteChecksums(files map[string]entities.TranscriptFile) map[string]string {
	checksums := make(map[string]string, len(files))
	for key, file := range files {
		checksums[key] = file.Checksum
	}
	return checksums
}

package entities

import (
	"fmt"
	"sort"
	"time"
)

// SyncAction is the outcome of reconciling one path across the local tool
// directory, the sync repo, and the base (the content this device last
// synced for that path).
type SyncAction string

const (
	// SyncActionNone means both sides already hold the same content.
	SyncActionNone SyncAction = "none"
	// SyncActionPush means only the local side changed since the last sync
	// (or the file is new locally), so the local content goes to the repo.
	SyncActionPush SyncAction = "push"
	// SyncActionPull means only the repo side changed since the last sync
	// (or the file is new in the repo), so the repo content comes local.
	SyncActionPull SyncAction = "pull"
	// SyncActionDeleteRepo means the file was deleted locally and the repo
	// copy is unchanged since the last sync, so the repo copy is deleted.
	SyncActionDeleteRepo SyncAction = "delete-repo"
	// SyncActionDeleteLocal means the file was deleted in the repo and the
	// local copy is unchanged since the last sync, so it is deleted locally.
	SyncActionDeleteLocal SyncAction = "delete-local"
	// SyncActionForget means the file is gone on both sides; only the base
	// entry is dropped.
	SyncActionForget SyncAction = "forget"
	// SyncActionConflict means both sides changed since the last sync, or the
	// file was never synced and the two sides disagree.
	SyncActionConflict SyncAction = "conflict"
)

// IsOutgoing reports whether the action writes to the sync repo.
func (a SyncAction) IsOutgoing() bool {
	return a == SyncActionPush || a == SyncActionDeleteRepo
}

// IsIncoming reports whether the action writes to the local tool directory.
func (a SyncAction) IsIncoming() bool {
	return a == SyncActionPull || a == SyncActionDeleteLocal
}

// ConflictReason explains why a path could not be reconciled automatically.
type ConflictReason string

const (
	// ConflictBothModified means both sides changed the file since the last sync.
	ConflictBothModified ConflictReason = "both-modified"
	// ConflictBothAdded means the file was never synced from this device and
	// the local and repo copies differ (typical on a device's first sync).
	ConflictBothAdded ConflictReason = "both-added"
	// ConflictLocalModifiedRemoteDeleted means the file changed locally while
	// another device deleted it from the repo.
	ConflictLocalModifiedRemoteDeleted ConflictReason = "local-modified-remote-deleted"
	// ConflictLocalDeletedRemoteModified means the file was deleted locally
	// while another device changed it in the repo.
	ConflictLocalDeletedRemoteModified ConflictReason = "local-deleted-remote-modified"
)

// conflictReasonDescriptions explains each reason in a short phrase.
//
//nolint:gochecknoglobals // compiled-in lookup table
var conflictReasonDescriptions = map[ConflictReason]string{
	ConflictBothModified:               "changed on both sides since the last sync",
	ConflictBothAdded:                  "never synced from this device and the two copies differ",
	ConflictLocalModifiedRemoteDeleted: "changed here but deleted in the repo",
	ConflictLocalDeletedRemoteModified: "deleted here but changed in the repo",
}

// Describe returns a short human-readable explanation of the reason.
func (r ConflictReason) Describe() string {
	if description, ok := conflictReasonDescriptions[r]; ok {
		return description
	}
	return string(r)
}

// SyncEntry is one row of a reconciliation plan. A checksum is empty when the
// file is absent on that side, or, for the base, when this device never
// synced the path.
type SyncEntry struct {
	Path           string
	Action         SyncAction
	Reason         ConflictReason
	LocalChecksum  string
	RemoteChecksum string
	BaseChecksum   string
}

// Reconcile classifies every path present in any of the three snapshots,
// each a map from tool-relative path to content checksum. The base lets the
// classification tell "changed here" from "changed there": a side that still
// matches the base did not change, so the other side's version wins without
// asking. Without a base entry (first sync) nothing can be inferred about
// deletions, and differing copies are reported as conflicts.
//
// The result is sorted by path so plans print and execute deterministically.
func Reconcile(local, remote, base map[string]string) []SyncEntry {
	paths := make(map[string]struct{}, len(local)+len(remote)+len(base))
	for _, snapshot := range []map[string]string{local, remote, base} {
		for path := range snapshot {
			paths[path] = struct{}{}
		}
	}

	sorted := make([]string, 0, len(paths))
	for path := range paths {
		sorted = append(sorted, path)
	}
	sort.Strings(sorted)

	entries := make([]SyncEntry, 0, len(sorted))
	for _, path := range sorted {
		entries = append(entries, classify(path, local[path], remote[path], base[path]))
	}
	return entries
}

// classify applies the three-way decision table to a single path.
func classify(path, local, remote, base string) SyncEntry {
	entry := SyncEntry{
		Path:           path,
		LocalChecksum:  local,
		RemoteChecksum: remote,
		BaseChecksum:   base,
	}

	switch {
	case local == "" && remote == "":
		entry.Action = SyncActionForget
	case local == remote:
		entry.Action = SyncActionNone
	case base == "":
		entry.Action, entry.Reason = classifyUnsynced(local, remote)
	case remote == "":
		entry.Action, entry.Reason = pickUnlessChanged(local, base, SyncActionDeleteLocal,
			ConflictLocalModifiedRemoteDeleted)
	case local == "":
		entry.Action, entry.Reason = pickUnlessChanged(remote, base, SyncActionDeleteRepo,
			ConflictLocalDeletedRemoteModified)
	case remote == base:
		entry.Action = SyncActionPush
	case local == base:
		entry.Action = SyncActionPull
	default:
		entry.Action = SyncActionConflict
		entry.Reason = ConflictBothModified
	}
	return entry
}

// classifyUnsynced handles a path this device never synced: a file present on
// one side only is new there, and two differing copies cannot be ordered.
func classifyUnsynced(local, remote string) (SyncAction, ConflictReason) {
	switch {
	case remote == "":
		return SyncActionPush, ""
	case local == "":
		return SyncActionPull, ""
	default:
		return SyncActionConflict, ConflictBothAdded
	}
}

// pickUnlessChanged returns action when the surviving side still matches the
// base (so the deletion on the other side wins), or a conflict with reason
// when the surviving side changed since the last sync.
func pickUnlessChanged(surviving, base string, action SyncAction, reason ConflictReason) (SyncAction, ConflictReason) {
	if surviving == base {
		return action, ""
	}
	return SyncActionConflict, reason
}

// ConflictChoice is how a conflict gets resolved, either by the user at a
// prompt or by a preference flag.
type ConflictChoice string

const (
	// ChoiceKeepLocal applies the local version on both sides.
	ChoiceKeepLocal ConflictChoice = "local"
	// ChoiceKeepRemote applies the repo version on both sides.
	ChoiceKeepRemote ConflictChoice = "remote"
	// ChoiceSkip leaves both sides untouched; the conflict is reported again
	// on the next sync.
	ChoiceSkip ConflictChoice = "skip"
	// ChoiceKeepLocalAll keeps the local version for this and every
	// remaining conflict in the run.
	ChoiceKeepLocalAll ConflictChoice = "local-all"
	// ChoiceKeepRemoteAll keeps the repo version for this and every
	// remaining conflict in the run.
	ChoiceKeepRemoteAll ConflictChoice = "remote-all"
	// ChoiceQuit aborts the run before anything is written.
	ChoiceQuit ConflictChoice = "quit"
)

// Resolve returns the entry with its action replaced by the one that applies
// choice: keeping a side that no longer has the file deletes it on the other
// side. Choices that do not pick a side leave the entry unchanged.
func (e SyncEntry) Resolve(choice ConflictChoice) SyncEntry {
	switch choice {
	case ChoiceKeepLocal, ChoiceKeepLocalAll:
		if e.LocalChecksum == "" {
			e.Action = SyncActionDeleteRepo
		} else {
			e.Action = SyncActionPush
		}
	case ChoiceKeepRemote, ChoiceKeepRemoteAll:
		if e.RemoteChecksum == "" {
			e.Action = SyncActionDeleteLocal
		} else {
			e.Action = SyncActionPull
		}
	case ChoiceSkip, ChoiceQuit:
	}
	return e
}

// SyncDirection limits which automatic actions a run executes: push only
// writes to the repo, pull only writes locally, sync does both. Conflicts the
// user resolves explicitly are always executed in full.
type SyncDirection string

const (
	// SyncBoth executes incoming and outgoing actions.
	SyncBoth SyncDirection = "both"
	// SyncOutgoing executes only actions that write to the repo.
	SyncOutgoing SyncDirection = "outgoing"
	// SyncIncoming executes only actions that write locally.
	SyncIncoming SyncDirection = "incoming"
)

// Allows reports whether an automatic action runs in this direction.
func (d SyncDirection) Allows(action SyncAction) bool {
	switch {
	case action.IsOutgoing():
		return d != SyncIncoming
	case action.IsIncoming():
		return d != SyncOutgoing
	default:
		return true
	}
}

// SyncPreference resolves conflicts without prompting.
type SyncPreference string

const (
	// PreferAsk prompts for every conflict (and skips them when no terminal
	// is attached).
	PreferAsk SyncPreference = ""
	// PreferLocal keeps the local version of every conflict.
	PreferLocal SyncPreference = "local"
	// PreferRemote keeps the repo version of every conflict.
	PreferRemote SyncPreference = "remote"
	// PreferNewer keeps whichever side changed last; conflicts whose order
	// cannot be determined are skipped.
	PreferNewer SyncPreference = "newer"
)

// ParseSyncPreference validates a --prefer flag value.
func ParseSyncPreference(value string) (SyncPreference, error) {
	switch preference := SyncPreference(value); preference {
	case PreferAsk, PreferLocal, PreferRemote, PreferNewer:
		return preference, nil
	default:
		return PreferAsk, fmt.Errorf("invalid preference %q: must be local, remote, or newer", value)
	}
}

// ConflictDetail is everything a person needs to decide one conflict: when
// each side last changed, where the repo change came from, and the diff.
type ConflictDetail struct {
	Tool           string
	Path           string
	Reason         ConflictReason
	LocalModified  time.Time // zero when the local file is absent
	RemoteModified time.Time // time of the last repo commit touching the file; zero when unknown
	RemoteSummary  string    // subject of that commit, e.g. "sync(laptop): updated personal configurations"
	Diff           string    // unified diff from the repo version to the local version
	Binary         bool      // true when either side is not text, so no diff is shown
}

// NewerSide reports which side changed last: ChoiceKeepLocal,
// ChoiceKeepRemote, or ChoiceSkip when either timestamp is unknown or they
// are equal.
func (d ConflictDetail) NewerSide() ConflictChoice {
	switch {
	case d.LocalModified.IsZero() || d.RemoteModified.IsZero():
		return ChoiceSkip
	case d.LocalModified.After(d.RemoteModified):
		return ChoiceKeepLocal
	case d.RemoteModified.After(d.LocalModified):
		return ChoiceKeepRemote
	default:
		return ChoiceSkip
	}
}

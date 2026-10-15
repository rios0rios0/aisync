package entities

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// DefaultTranscriptsBranch is the branch of the sync repo that holds the
// transcript snapshot when sync.transcripts_branch is not set.
const DefaultTranscriptsBranch = "transcripts"

// hoursPerDay converts the "d" unit of a max age into a [time.Duration].
const hoursPerDay = 24

// TranscriptSpec declares which conversation files of a tool are synced
// through the transcript snapshot branch. Only the latest snapshot is kept
// there, so conversations never accumulate in git history.
type TranscriptSpec struct {
	// Dir is the tool-relative directory holding the conversations, such as
	// "projects" for Claude Code.
	Dir string `yaml:"dir"`
	// Logs are patterns, relative to Dir, of append-only conversation logs,
	// such as "*/*.jsonl". Logs expire after MaxAge, and when two devices
	// extended the same log, the copy that contains the other wins.
	Logs []string `yaml:"logs"`
	// Files are patterns, relative to Dir, of other files to sync, such as
	// "*/memory/**". They never expire and their conflicts are reviewed like
	// any personal file.
	Files []string `yaml:"files,omitempty"`
	// MaxAge is how long a log can go untouched before it leaves the
	// snapshot, such as "14d" or "36h". Empty keeps logs forever.
	MaxAge string `yaml:"max_age,omitempty"`
}

// TranscriptKind classifies a path under a TranscriptSpec's Dir.
type TranscriptKind string

const (
	// TranscriptKindNone is a path the spec does not sync.
	TranscriptKindNone TranscriptKind = ""
	// TranscriptKindLog is an append-only conversation log.
	TranscriptKindLog TranscriptKind = "log"
	// TranscriptKindFile is any other synced file.
	TranscriptKindFile TranscriptKind = "file"
)

// Classify reports whether rel (relative to Dir) is a log, another synced
// file, or out of scope. A path matching both lists is a log.
func (s TranscriptSpec) Classify(rel string) TranscriptKind {
	normalized := strings.ReplaceAll(rel, "\\", "/")
	switch {
	case matchesAnchoredPattern(normalized, s.Logs):
		return TranscriptKindLog
	case matchesAnchoredPattern(normalized, s.Files):
		return TranscriptKindFile
	default:
		return TranscriptKindNone
	}
}

// MaxAgeDuration parses MaxAge. It accepts whole days ("14d") and any
// [time.ParseDuration] value ("36h"); empty means logs never expire (zero).
func (s TranscriptSpec) MaxAgeDuration() (time.Duration, error) {
	value := strings.TrimSpace(s.MaxAge)
	if value == "" {
		return 0, nil
	}
	if days, isDays := strings.CutSuffix(value, "d"); isDays {
		count, err := strconv.Atoi(days)
		if err != nil || count < 0 {
			return 0, fmt.Errorf("invalid transcripts max_age %q: days must be a whole number", s.MaxAge)
		}
		return time.Duration(count) * hoursPerDay * time.Hour, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration < 0 {
		return 0, fmt.Errorf(
			"invalid transcripts max_age %q: use a number of days such as 14d or a duration such as 36h",
			s.MaxAge,
		)
	}
	return duration, nil
}

// TranscriptFile is one file in the transcript snapshot, keyed by
// "<tool>/<path relative to the tool's transcript dir>".
type TranscriptFile struct {
	Key      string
	Checksum string
	ModTime  time.Time
	// Device is the hostname of the device whose copy is stored.
	Device string
	// Stored is the blob path inside the snapshot; empty until stored.
	Stored string
	// Content is plaintext waiting to be stored, set only for files that
	// are new or changed since the snapshot was fetched.
	Content []byte
}

// TranscriptSnapshot is the content of the transcript branch at one commit.
type TranscriptSnapshot struct {
	// Lease is the commit the snapshot was fetched from, or empty when the
	// branch does not exist yet. Publishing fails if the branch moved.
	Lease string
	Files map[string]TranscriptFile
}

// NewTranscriptSnapshot returns an empty snapshot with no lease.
func NewTranscriptSnapshot() *TranscriptSnapshot {
	return &TranscriptSnapshot{Files: make(map[string]TranscriptFile)}
}

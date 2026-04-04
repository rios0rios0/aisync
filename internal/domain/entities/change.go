package entities

import "time"

// ChangeDirection represents the type of file change.
type ChangeDirection string

const (
	ChangeAdded    ChangeDirection = "+"
	ChangeModified ChangeDirection = "~"
	ChangeRemoved  ChangeDirection = "-"
)

// FileChange represents a detected change in a managed file.
type FileChange struct {
	Path            string
	Direction       ChangeDirection
	Source          string // external source name or "personal"
	Namespace       string // "shared" or "personal"
	LocalTimestamp  time.Time
	RemoteTimestamp time.Time
	LocalSize       int64
	RemoteSize      int64
	LocalChecksum   string
	RemoteChecksum  string
	Encrypted       bool
	LocalContent    []byte
	RemoteContent   []byte
}

// SizeDelta returns the size difference (remote - local) in bytes.
func (c *FileChange) SizeDelta() int64 {
	return c.RemoteSize - c.LocalSize
}

// IsRemoteNewer returns true if the remote version is newer than the local one.
func (c *FileChange) IsRemoteNewer() bool {
	return c.RemoteTimestamp.After(c.LocalTimestamp)
}

// HasClockSkew returns true if the timestamps match within 1 second but the
// checksums differ, indicating a potential clock skew issue.
func (c *FileChange) HasClockSkew() bool {
	if c.LocalChecksum == "" || c.RemoteChecksum == "" {
		return false
	}
	diff := c.LocalTimestamp.Sub(c.RemoteTimestamp)
	if diff < 0 {
		diff = -diff
	}
	return diff <= time.Second && c.LocalChecksum != c.RemoteChecksum
}

// DiffResult holds the complete set of detected changes grouped by category.
type DiffResult struct {
	SharedChanges      []FileChange
	PersonalChanges    []FileChange
	LocalUncommitted   []FileChange
}

// HasChanges returns true if any changes were detected.
func (d *DiffResult) HasChanges() bool {
	return len(d.SharedChanges) > 0 || len(d.PersonalChanges) > 0 || len(d.LocalUncommitted) > 0
}

// TotalCount returns the total number of detected changes.
func (d *DiffResult) TotalCount() int {
	return len(d.SharedChanges) + len(d.PersonalChanges) + len(d.LocalUncommitted)
}

package entities

import "time"

// Journal tracks the atomic apply operation so it can be recovered if interrupted.
type Journal struct {
	Timestamp  time.Time          `json:"timestamp"`
	StagingDir string             `json:"staging_dir"`
	Operations []JournalOperation `json:"operations"`
}

// JournalOperation represents a single file move from staging to its final target.
type JournalOperation struct {
	SourcePath  string `json:"source_path"`
	TargetPath  string `json:"target_path"`
	OldChecksum string `json:"old_checksum,omitempty"`
	NewChecksum string `json:"new_checksum"`
	Status      string `json:"status"` // "pending" or "applied"
}

// NewJournal creates a new Journal for the given staging directory.
func NewJournal(stagingDir string) *Journal {
	return &Journal{
		Timestamp:  time.Now(),
		StagingDir: stagingDir,
		Operations: make([]JournalOperation, 0),
	}
}

// AddOperation appends a new pending operation to the journal.
func (j *Journal) AddOperation(sourcePath, targetPath, oldChecksum, newChecksum string) {
	j.Operations = append(j.Operations, JournalOperation{
		SourcePath:  sourcePath,
		TargetPath:  targetPath,
		OldChecksum: oldChecksum,
		NewChecksum: newChecksum,
		Status:      "pending",
	})
}

// MarkApplied sets the status of the operation matching the given target path to "applied".
func (j *Journal) MarkApplied(targetPath string) {
	for i := range j.Operations {
		if j.Operations[i].TargetPath == targetPath {
			j.Operations[i].Status = "applied"
			return
		}
	}
}

// IsComplete returns true if all operations have been applied.
func (j *Journal) IsComplete() bool {
	for _, op := range j.Operations {
		if op.Status != "applied" {
			return false
		}
	}
	return len(j.Operations) > 0
}

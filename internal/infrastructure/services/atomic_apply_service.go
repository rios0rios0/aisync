package services

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

const binaryCheckLimit = 8192

// AtomicApplyService stages files to a temporary directory and then atomically
// moves them to their final targets. A journal tracks operations so that
// interrupted applies can be recovered.
type AtomicApplyService struct {
	journalRepo repositories.JournalRepository
	basePath    string
}

// NewAtomicApplyService creates a new AtomicApplyService with the given
// journal repository and base path for staging directories.
func NewAtomicApplyService(journalRepo repositories.JournalRepository, basePath string) *AtomicApplyService {
	return &AtomicApplyService{
		journalRepo: journalRepo,
		basePath:    basePath,
	}
}

// Stage writes all files to a staging directory and creates a journal with
// pending operations. The files map keys are final target paths and values
// are the file contents.
func (s *AtomicApplyService) Stage(files map[string][]byte) (*entities.Journal, error) {
	stagingDir := filepath.Join(s.basePath, "staging", strconv.FormatInt(time.Now().UnixNano(), 10))
	if err := os.MkdirAll(stagingDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create staging directory: %w", err)
	}

	journal := entities.NewJournal(stagingDir)

	for targetPath, content := range files {
		// Normalize the target path for staging: strip volume/root prefix
		// in a cross-platform way rather than using filepath.Rel("/", ...).
		cleanPath := filepath.Clean(targetPath)
		vol := filepath.VolumeName(cleanPath)
		relPath := strings.TrimPrefix(cleanPath, vol)
		relPath = strings.TrimPrefix(relPath, string(filepath.Separator))

		stagingPath := filepath.Join(stagingDir, relPath)
		if err := os.MkdirAll(filepath.Dir(stagingPath), 0700); err != nil {
			return nil, fmt.Errorf("failed to create staging subdirectory for %s: %w", relPath, err)
		}

		normalized := normalizeLineEndings(content)
		if err := os.WriteFile(stagingPath, normalized, 0600); err != nil {
			return nil, fmt.Errorf("failed to write staged file %s: %w", stagingPath, err)
		}

		oldChecksum := readExistingChecksum(targetPath)
		newChecksum := computeChecksum(normalized)

		journal.AddOperation(stagingPath, targetPath, oldChecksum, newChecksum)
		logger.Debugf("staged file: %s -> %s", stagingPath, targetPath)
	}

	if err := s.journalRepo.Save(journal); err != nil {
		return nil, fmt.Errorf("failed to save journal after staging: %w", err)
	}

	logger.Infof("staged %d files to %s", len(files), stagingDir)
	return journal, nil
}

// Apply moves all pending staged files to their final targets. After each
// successful move the journal is updated. Once all operations complete, the
// journal and staging directory are cleaned up.
func (s *AtomicApplyService) Apply(journal *entities.Journal) error {
	for i := range journal.Operations {
		op := &journal.Operations[i]
		if op.Status == "applied" {
			continue
		}

		if err := os.MkdirAll(filepath.Dir(op.TargetPath), 0700); err != nil {
			return fmt.Errorf("failed to create target directory for %s: %w", op.TargetPath, err)
		}

		if err := moveFile(op.SourcePath, op.TargetPath); err != nil {
			return fmt.Errorf("failed to move %s to %s: %w", op.SourcePath, op.TargetPath, err)
		}

		journal.MarkApplied(op.TargetPath)
		if err := s.journalRepo.Save(journal); err != nil {
			return fmt.Errorf("failed to update journal after applying %s: %w", op.TargetPath, err)
		}

		logger.Debugf("applied: %s -> %s", op.SourcePath, op.TargetPath)
	}

	if err := s.journalRepo.Clear(); err != nil {
		return fmt.Errorf("failed to clear journal after apply: %w", err)
	}

	logger.Info("all staged files applied successfully")
	return nil
}

// Recover checks for an incomplete journal and resumes the apply. If the
// staging directory is missing but the journal exists, it clears the corrupt
// journal state.
func (s *AtomicApplyService) Recover() error {
	if !s.journalRepo.Exists() {
		return nil
	}

	journal, err := s.journalRepo.Load()
	if err != nil {
		logger.Warnf("failed to load journal for recovery, clearing: %s", err)
		return s.journalRepo.Clear()
	}

	if journal.IsComplete() {
		logger.Info("journal found but all operations already applied, clearing")
		return s.journalRepo.Clear()
	}

	if _, statErr := os.Stat(journal.StagingDir); os.IsNotExist(statErr) {
		logger.Warn("journal found but staging directory is missing, clearing corrupt state")
		return s.journalRepo.Clear()
	}

	logger.Info("recovering incomplete apply from journal")
	return s.Apply(journal)
}

// moveFile moves a file from src to dst. It first attempts [os.Rename] for an
// atomic move. If that fails (e.g., cross-device), it falls back to copy + remove.
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read source file for copy: %w", err)
	}

	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("failed to stat source file for copy: %w", err)
	}

	if err = os.WriteFile(filepath.Clean(dst), data, info.Mode()); err != nil { //nolint:gosec // dst is cleaned above
		return fmt.Errorf("failed to write destination file: %w", err)
	}

	if err = os.Remove(src); err != nil {
		return fmt.Errorf("failed to remove source file after copy: %w", err)
	}

	return nil
}

// readExistingChecksum returns the SHA-256 checksum of an existing file, or an
// empty string if the file does not exist or cannot be read.
func readExistingChecksum(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return computeChecksum(data)
}

// computeChecksum returns the hex-encoded SHA-256 checksum of the given data.
func computeChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// normalizeLineEndings converts CRLF line endings to LF. Binary files (detected
// by the presence of null bytes in the first 8 KB) are returned unchanged.
func normalizeLineEndings(data []byte) []byte {
	if isBinaryContent(data) {
		return data
	}
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
}

// isBinaryContent returns true if the data likely represents a binary file by
// checking for null bytes in the first 8 KB — the same heuristic Git uses.
func isBinaryContent(data []byte) bool {
	limit := min(len(data), binaryCheckLimit)
	return bytes.ContainsRune(data[:limit], 0)
}

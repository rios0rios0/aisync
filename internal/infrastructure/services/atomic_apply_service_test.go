//go:build unit

package services_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	services "github.com/rios0rios0/aisync/internal/infrastructure/services"
)

// inMemoryJournalRepo is a stub JournalRepository for testing.
type inMemoryJournalRepo struct {
	journal *entities.Journal
	exists  bool
}

func newInMemoryJournalRepo() *inMemoryJournalRepo {
	return &inMemoryJournalRepo{}
}

func (r *inMemoryJournalRepo) Save(journal *entities.Journal) error {
	r.journal = journal
	r.exists = true
	return nil
}

func (r *inMemoryJournalRepo) Load() (*entities.Journal, error) {
	return r.journal, nil
}

func (r *inMemoryJournalRepo) Exists() bool {
	return r.exists
}

func (r *inMemoryJournalRepo) Clear() error {
	r.journal = nil
	r.exists = false
	return nil
}

func TestAtomicApplyService_Stage_ShouldCreateStagingDirAndJournal(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "target")
	assert.NoError(t, os.MkdirAll(targetDir, 0755))

	repo := newInMemoryJournalRepo()
	svc := services.NewAtomicApplyService(repo, tmpDir)

	targetPath := filepath.Join(targetDir, "config.json")
	files := map[string][]byte{
		targetPath: []byte(`{"key": "value"}`),
	}

	// when
	journal, err := svc.Stage(files)

	// then
	assert.NoError(t, err)
	assert.NotNil(t, journal)
	assert.Len(t, journal.Operations, 1)
	assert.Equal(t, "pending", journal.Operations[0].Status)
	assert.Equal(t, targetPath, journal.Operations[0].TargetPath)

	// Verify staging directory was created
	_, statErr := os.Stat(journal.StagingDir)
	assert.NoError(t, statErr)

	// Verify journal was saved
	assert.True(t, repo.Exists())
}

func TestAtomicApplyService_Apply_ShouldMoveFilesToTarget(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "target")
	assert.NoError(t, os.MkdirAll(targetDir, 0755))

	repo := newInMemoryJournalRepo()
	svc := services.NewAtomicApplyService(repo, tmpDir)

	targetPath := filepath.Join(targetDir, "config.json")
	content := []byte(`{"applied": true}`)
	files := map[string][]byte{
		targetPath: content,
	}

	journal, err := svc.Stage(files)
	assert.NoError(t, err)

	// when
	applyErr := svc.Apply(journal)

	// then
	assert.NoError(t, applyErr)

	data, readErr := os.ReadFile(targetPath)
	assert.NoError(t, readErr)
	assert.Equal(t, content, data)

	// Journal should be cleared
	assert.False(t, repo.Exists())
}

func TestAtomicApplyService_Apply_ShouldHandleMultipleFiles(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "target")
	assert.NoError(t, os.MkdirAll(targetDir, 0755))

	repo := newInMemoryJournalRepo()
	svc := services.NewAtomicApplyService(repo, tmpDir)

	target1 := filepath.Join(targetDir, "file1.txt")
	target2 := filepath.Join(targetDir, "file2.txt")
	files := map[string][]byte{
		target1: []byte("content one"),
		target2: []byte("content two"),
	}

	journal, err := svc.Stage(files)
	assert.NoError(t, err)

	// when
	applyErr := svc.Apply(journal)

	// then
	assert.NoError(t, applyErr)

	data1, _ := os.ReadFile(target1)
	assert.Equal(t, []byte("content one"), data1)

	data2, _ := os.ReadFile(target2)
	assert.Equal(t, []byte("content two"), data2)
}

func TestAtomicApplyService_Recover_ShouldCompleteApplyWhenPendingJournalExists(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "target")
	assert.NoError(t, os.MkdirAll(targetDir, 0755))

	repo := newInMemoryJournalRepo()
	svc := services.NewAtomicApplyService(repo, tmpDir)

	targetPath := filepath.Join(targetDir, "recovered.txt")
	content := []byte("recovered content")
	files := map[string][]byte{
		targetPath: content,
	}

	// Stage but do NOT apply — simulate interrupted apply
	journal, err := svc.Stage(files)
	assert.NoError(t, err)
	assert.NotNil(t, journal)

	// when
	recoverErr := svc.Recover()

	// then
	assert.NoError(t, recoverErr)

	data, readErr := os.ReadFile(targetPath)
	assert.NoError(t, readErr)
	assert.Equal(t, content, data)

	// Journal should be cleared after recovery
	assert.False(t, repo.Exists())
}

func TestAtomicApplyService_Recover_ShouldNoOpWhenNoJournalExists(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	repo := newInMemoryJournalRepo()
	svc := services.NewAtomicApplyService(repo, tmpDir)

	// when
	err := svc.Recover()

	// then
	assert.NoError(t, err)
	assert.False(t, repo.Exists())
}

func TestAtomicApplyService_Recover_ShouldClearJournalWhenStagingDirIsMissing(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	repo := newInMemoryJournalRepo()
	svc := services.NewAtomicApplyService(repo, tmpDir)

	// Create a journal pointing to a non-existent staging directory
	journal := entities.NewJournal(filepath.Join(tmpDir, "nonexistent-staging"))
	journal.AddOperation("/fake/source", "/fake/target", "", "abc123")
	assert.NoError(t, repo.Save(journal))

	// when
	err := svc.Recover()

	// then
	assert.NoError(t, err)
	assert.False(t, repo.Exists())
}

func TestAtomicApplyService_Stage_ShouldRecordOldChecksumWhenTargetExists(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "target")
	assert.NoError(t, os.MkdirAll(targetDir, 0755))

	targetPath := filepath.Join(targetDir, "existing.txt")
	assert.NoError(t, os.WriteFile(targetPath, []byte("old content"), 0644))

	repo := newInMemoryJournalRepo()
	svc := services.NewAtomicApplyService(repo, tmpDir)

	files := map[string][]byte{
		targetPath: []byte("new content"),
	}

	// when
	journal, err := svc.Stage(files)

	// then
	assert.NoError(t, err)
	assert.Len(t, journal.Operations, 1)
	assert.NotEmpty(t, journal.Operations[0].OldChecksum, "old checksum should be recorded for existing target")
	assert.NotEmpty(t, journal.Operations[0].NewChecksum)
	assert.NotEqual(t, journal.Operations[0].OldChecksum, journal.Operations[0].NewChecksum)
}

func TestAtomicApplyService_Apply_ShouldFallbackToCopyWhenCrossDevice(t *testing.T) {
	// given
	stagingDir := t.TempDir()
	targetDir := t.TempDir()

	stagingFile := filepath.Join(stagingDir, "cross.txt")
	content := []byte("cross-device content")
	assert.NoError(t, os.WriteFile(stagingFile, content, 0644))

	targetPath := filepath.Join(targetDir, "cross.txt")

	repo := newInMemoryJournalRepo()
	svc := services.NewAtomicApplyService(repo, stagingDir)

	journal := entities.NewJournal(stagingDir)
	journal.AddOperation(stagingFile, targetPath, "", services.ComputeChecksum(content))
	assert.NoError(t, repo.Save(journal))

	// when
	err := svc.Apply(journal)

	// then
	assert.NoError(t, err)

	data, readErr := os.ReadFile(targetPath)
	assert.NoError(t, readErr)
	assert.Equal(t, content, data)

	// Source file should be removed after copy
	_, statErr := os.Stat(stagingFile)
	assert.True(t, os.IsNotExist(statErr))
}

func TestAtomicApplyService_Apply_ShouldSkipAlreadyAppliedOperations(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "target")
	assert.NoError(t, os.MkdirAll(targetDir, 0755))

	stagingDir := filepath.Join(tmpDir, "staging", "test")
	assert.NoError(t, os.MkdirAll(stagingDir, 0755))

	// Only create a staging file for the pending operation
	pendingTarget := filepath.Join(targetDir, "pending.txt")
	pendingContent := []byte("pending content")
	pendingStagingFile := filepath.Join(stagingDir, "pending.txt")
	assert.NoError(t, os.WriteFile(pendingStagingFile, pendingContent, 0644))

	repo := newInMemoryJournalRepo()
	svc := services.NewAtomicApplyService(repo, tmpDir)

	journal := entities.NewJournal(stagingDir)

	// Add an operation that is already applied
	journal.Operations = append(journal.Operations, entities.JournalOperation{
		SourcePath:  filepath.Join(stagingDir, "done.txt"),
		TargetPath:  filepath.Join(targetDir, "done.txt"),
		NewChecksum: "abc",
		Status:      "applied",
	})

	// Add a pending operation
	journal.AddOperation(pendingStagingFile, pendingTarget, "", services.ComputeChecksum(pendingContent))
	assert.NoError(t, repo.Save(journal))

	// when
	err := svc.Apply(journal)

	// then
	assert.NoError(t, err)

	// Only the pending file should be moved
	data, readErr := os.ReadFile(pendingTarget)
	assert.NoError(t, readErr)
	assert.Equal(t, pendingContent, data)
}

func TestAtomicApplyService_Recover_ShouldClearWhenJournalIsComplete(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	repo := newInMemoryJournalRepo()
	svc := services.NewAtomicApplyService(repo, tmpDir)

	journal := entities.NewJournal(filepath.Join(tmpDir, "staging"))
	journal.Operations = append(journal.Operations, entities.JournalOperation{
		SourcePath:  "/fake/source",
		TargetPath:  "/fake/target",
		NewChecksum: "abc",
		Status:      "applied",
	})
	assert.NoError(t, repo.Save(journal))

	// when
	err := svc.Recover()

	// then
	assert.NoError(t, err)
	assert.False(t, repo.Exists())
}

func TestAtomicApplyService_Apply_ShouldCreateTargetDirectoryIfMissing(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	stagingDir := filepath.Join(tmpDir, "staging", "test")
	assert.NoError(t, os.MkdirAll(stagingDir, 0755))

	content := []byte("deep content")
	stagingFile := filepath.Join(stagingDir, "deep.txt")
	assert.NoError(t, os.WriteFile(stagingFile, content, 0644))

	targetPath := filepath.Join(tmpDir, "deep", "nested", "dir", "deep.txt")

	repo := newInMemoryJournalRepo()
	svc := services.NewAtomicApplyService(repo, tmpDir)

	journal := entities.NewJournal(stagingDir)
	journal.AddOperation(stagingFile, targetPath, "", services.ComputeChecksum(content))
	assert.NoError(t, repo.Save(journal))

	// when
	err := svc.Apply(journal)

	// then
	assert.NoError(t, err)

	data, readErr := os.ReadFile(targetPath)
	assert.NoError(t, readErr)
	assert.Equal(t, content, data)
}

func TestMoveFile_ShouldMoveFileSuccessfully(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "source.txt")
	dstFile := filepath.Join(tmpDir, "dest.txt")
	content := []byte("move me")
	assert.NoError(t, os.WriteFile(srcFile, content, 0644))

	// when
	err := services.MoveFile(srcFile, dstFile)

	// then
	assert.NoError(t, err)

	data, readErr := os.ReadFile(dstFile)
	assert.NoError(t, readErr)
	assert.Equal(t, content, data)

	// Source should no longer exist
	_, statErr := os.Stat(srcFile)
	assert.True(t, os.IsNotExist(statErr))
}

func TestComputeChecksum_ShouldReturnConsistentHash(t *testing.T) {
	// given
	data := []byte("test content for checksum")

	// when
	hash1 := services.ComputeChecksum(data)
	hash2 := services.ComputeChecksum(data)

	// then
	assert.NotEmpty(t, hash1)
	assert.Equal(t, hash1, hash2, "same data should produce same checksum")
}

func TestComputeChecksum_ShouldDifferForDifferentData(t *testing.T) {
	// given
	data1 := []byte("content A")
	data2 := []byte("content B")

	// when
	hash1 := services.ComputeChecksum(data1)
	hash2 := services.ComputeChecksum(data2)

	// then
	assert.NotEqual(t, hash1, hash2, "different data should produce different checksums")
}

func TestReadExistingChecksum_ShouldReturnEmptyForNonexistentFile(t *testing.T) {
	// given
	path := filepath.Join(t.TempDir(), "nonexistent.txt")

	// when
	result := services.ReadExistingChecksum(path)

	// then
	assert.Empty(t, result)
}

func TestReadExistingChecksum_ShouldReturnChecksumForExistingFile(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "existing.txt")
	content := []byte("existing content")
	assert.NoError(t, os.WriteFile(path, content, 0644))

	// when
	result := services.ReadExistingChecksum(path)

	// then
	assert.NotEmpty(t, result)
	assert.Equal(t, services.ComputeChecksum(content), result)
}

func TestNormalizeLineEndings_ShouldConvertCRLFToLF(t *testing.T) {
	// given
	input := []byte("line one\r\nline two\r\nline three\r\n")

	// when
	result := services.NormalizeLineEndings(input)

	// then
	assert.Equal(t, []byte("line one\nline two\nline three\n"), result)
}

func TestNormalizeLineEndings_ShouldPreserveLFOnly(t *testing.T) {
	// given
	input := []byte("line one\nline two\n")

	// when
	result := services.NormalizeLineEndings(input)

	// then
	assert.Equal(t, input, result)
}

func TestNormalizeLineEndings_ShouldNotModifyBinaryContent(t *testing.T) {
	// given
	input := []byte("binary\x00data\r\nwith CRLF")

	// when
	result := services.NormalizeLineEndings(input)

	// then
	assert.Equal(t, input, result)
}

func TestIsBinaryContent_ShouldReturnTrueWhenNullBytePresent(t *testing.T) {
	// given
	data := []byte("contains\x00null")

	// when
	result := services.IsBinaryContent(data)

	// then
	assert.True(t, result)
}

func TestIsBinaryContent_ShouldReturnFalseForTextContent(t *testing.T) {
	// given
	data := []byte("plain text content\nwith newlines\n")

	// when
	result := services.IsBinaryContent(data)

	// then
	assert.False(t, result)
}

func TestAtomicApplyService_Stage_ShouldNormalizeCRLFToLF(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "target")
	assert.NoError(t, os.MkdirAll(targetDir, 0755))

	repo := newInMemoryJournalRepo()
	svc := services.NewAtomicApplyService(repo, tmpDir)

	targetPath := filepath.Join(targetDir, "rules.md")
	files := map[string][]byte{
		targetPath: []byte("# Rule\r\n\r\nSome content\r\n"),
	}

	// when
	journal, err := svc.Stage(files)

	// then
	assert.NoError(t, err)
	assert.NotNil(t, journal)

	stagedContent, readErr := os.ReadFile(journal.Operations[0].SourcePath)
	assert.NoError(t, readErr)
	assert.Equal(t, []byte("# Rule\n\nSome content\n"), stagedContent)
}

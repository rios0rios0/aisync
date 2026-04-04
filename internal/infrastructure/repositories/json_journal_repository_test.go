//go:build unit

package repositories_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	repositories "github.com/rios0rios0/aisync/internal/infrastructure/repositories"
)

func TestJSONJournalRepository_SaveThenLoad(t *testing.T) {
	// given
	basePath := t.TempDir()
	repo := repositories.NewJSONJournalRepository(basePath)
	ts := time.Date(2026, 4, 1, 10, 30, 0, 0, time.UTC)
	original := &entities.Journal{
		Timestamp:  ts,
		StagingDir: "/tmp/aisync-staging-12345",
		Operations: []entities.JournalOperation{
			{
				SourcePath:  "/tmp/aisync-staging-12345/rules/arch.md",
				TargetPath:  "/home/user/.claude/rules/arch.md",
				OldChecksum: "old-abc",
				NewChecksum: "new-def",
				Status:      "pending",
			},
			{
				SourcePath:  "/tmp/aisync-staging-12345/agents/review.md",
				TargetPath:  "/home/user/.claude/agents/review.md",
				NewChecksum: "new-ghi",
				Status:      "applied",
			},
		},
	}

	// when
	err := repo.Save(original)
	assert.NoError(t, err)
	loaded, err := repo.Load()

	// then
	assert.NoError(t, err)
	assert.True(t, original.Timestamp.Equal(loaded.Timestamp))
	assert.Equal(t, original.StagingDir, loaded.StagingDir)
	assert.Equal(t, len(original.Operations), len(loaded.Operations))
	assert.Equal(t, original.Operations[0].SourcePath, loaded.Operations[0].SourcePath)
	assert.Equal(t, original.Operations[0].TargetPath, loaded.Operations[0].TargetPath)
	assert.Equal(t, original.Operations[0].OldChecksum, loaded.Operations[0].OldChecksum)
	assert.Equal(t, original.Operations[0].NewChecksum, loaded.Operations[0].NewChecksum)
	assert.Equal(t, original.Operations[0].Status, loaded.Operations[0].Status)
	assert.Equal(t, original.Operations[1].SourcePath, loaded.Operations[1].SourcePath)
	assert.Equal(t, original.Operations[1].Status, loaded.Operations[1].Status)
	assert.Equal(t, "", loaded.Operations[1].OldChecksum)
}

func TestJSONJournalRepository_Exists_WithFile(t *testing.T) {
	// given
	basePath := t.TempDir()
	repo := repositories.NewJSONJournalRepository(basePath)
	journal := &entities.Journal{
		Timestamp:  time.Now(),
		StagingDir: "/tmp/staging",
		Operations: []entities.JournalOperation{},
	}
	err := repo.Save(journal)
	assert.NoError(t, err)

	// when
	exists := repo.Exists()

	// then
	assert.True(t, exists)
}

func TestJSONJournalRepository_Exists_WithoutFile(t *testing.T) {
	// given
	basePath := t.TempDir()
	repo := repositories.NewJSONJournalRepository(basePath)

	// when
	exists := repo.Exists()

	// then
	assert.False(t, exists)
}

func TestJSONJournalRepository_Clear_RemovesJournalFile(t *testing.T) {
	// given
	basePath := t.TempDir()
	repo := repositories.NewJSONJournalRepository(basePath)
	stagingDir := filepath.Join(t.TempDir(), "staging")
	err := os.MkdirAll(stagingDir, 0755)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(stagingDir, "test.txt"), []byte("data"), 0644)
	assert.NoError(t, err)

	journal := &entities.Journal{
		Timestamp:  time.Now(),
		StagingDir: stagingDir,
		Operations: []entities.JournalOperation{
			{
				SourcePath:  filepath.Join(stagingDir, "test.txt"),
				TargetPath:  "/home/user/.claude/test.txt",
				NewChecksum: "abc",
				Status:      "pending",
			},
		},
	}
	err = repo.Save(journal)
	assert.NoError(t, err)
	assert.True(t, repo.Exists())

	// when
	err = repo.Clear()

	// then
	assert.NoError(t, err)
	assert.False(t, repo.Exists())

	journalPath := filepath.Join(basePath, "journal.json")
	_, statErr := os.Stat(journalPath)
	assert.True(t, os.IsNotExist(statErr))

	_, statErr = os.Stat(stagingDir)
	assert.True(t, os.IsNotExist(statErr))
}

func TestJSONJournalRepository_Clear_NoFileNoError(t *testing.T) {
	// given
	basePath := t.TempDir()
	repo := repositories.NewJSONJournalRepository(basePath)

	// when
	err := repo.Clear()

	// then
	assert.NoError(t, err)
}

func TestJSONJournalRepository_PreservesAllFields(t *testing.T) {
	// given
	basePath := t.TempDir()
	repo := repositories.NewJSONJournalRepository(basePath)
	ts := time.Date(2026, 2, 20, 16, 45, 30, 0, time.UTC)
	original := &entities.Journal{
		Timestamp:  ts,
		StagingDir: "/var/tmp/aisync-staging-99999",
		Operations: []entities.JournalOperation{
			{
				SourcePath:  "/var/tmp/aisync-staging-99999/file1.md",
				TargetPath:  "/target/file1.md",
				OldChecksum: "checksum-old-1",
				NewChecksum: "checksum-new-1",
				Status:      "pending",
			},
			{
				SourcePath:  "/var/tmp/aisync-staging-99999/file2.md",
				TargetPath:  "/target/file2.md",
				OldChecksum: "",
				NewChecksum: "checksum-new-2",
				Status:      "applied",
			},
		},
	}

	// when
	err := repo.Save(original)
	assert.NoError(t, err)
	loaded, err := repo.Load()

	// then
	assert.NoError(t, err)
	assert.True(t, ts.Equal(loaded.Timestamp))
	assert.Equal(t, "/var/tmp/aisync-staging-99999", loaded.StagingDir)
	assert.Equal(t, 2, len(loaded.Operations))
	assert.Equal(t, "checksum-old-1", loaded.Operations[0].OldChecksum)
	assert.Equal(t, "checksum-new-1", loaded.Operations[0].NewChecksum)
	assert.Equal(t, "pending", loaded.Operations[0].Status)
	assert.Equal(t, "", loaded.Operations[1].OldChecksum)
	assert.Equal(t, "checksum-new-2", loaded.Operations[1].NewChecksum)
	assert.Equal(t, "applied", loaded.Operations[1].Status)
}

func TestJSONJournalRepository_Clear_ShouldNotErrorWhenStagingDirDoesNotExist(t *testing.T) {
	// given
	basePath := t.TempDir()
	repo := repositories.NewJSONJournalRepository(basePath)

	journal := &entities.Journal{
		Timestamp:  time.Now(),
		StagingDir: filepath.Join(t.TempDir(), "nonexistent-staging-dir"),
		Operations: []entities.JournalOperation{
			{
				SourcePath:  "/fake/source",
				TargetPath:  "/fake/target",
				NewChecksum: "abc",
				Status:      "pending",
			},
		},
	}
	err := repo.Save(journal)
	assert.NoError(t, err)
	assert.True(t, repo.Exists())

	// when
	err = repo.Clear()

	// then
	assert.NoError(t, err)
	assert.False(t, repo.Exists())
}

func TestJSONJournalRepository_SaveThenLoad_WithEmptyOperations(t *testing.T) {
	// given
	basePath := t.TempDir()
	repo := repositories.NewJSONJournalRepository(basePath)
	ts := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	original := &entities.Journal{
		Timestamp:  ts,
		StagingDir: "/tmp/empty-staging",
		Operations: []entities.JournalOperation{},
	}

	// when
	err := repo.Save(original)
	assert.NoError(t, err)
	loaded, err := repo.Load()

	// then
	assert.NoError(t, err)
	assert.True(t, ts.Equal(loaded.Timestamp))
	assert.Equal(t, "/tmp/empty-staging", loaded.StagingDir)
	assert.Len(t, loaded.Operations, 0)
}

func TestJSONJournalRepository_Load_InvalidJSON(t *testing.T) {
	// given
	basePath := t.TempDir()
	repo := repositories.NewJSONJournalRepository(basePath)
	journalPath := filepath.Join(basePath, "journal.json")
	err := os.WriteFile(journalPath, []byte("{invalid json!!!"), 0644)
	assert.NoError(t, err)

	// when
	journal, err := repo.Load()

	// then
	assert.Error(t, err)
	assert.Nil(t, journal)
	assert.Contains(t, err.Error(), "failed to parse journal file")
}

func TestJSONJournalRepository_Load_MissingFile(t *testing.T) {
	// given
	basePath := t.TempDir()
	repo := repositories.NewJSONJournalRepository(basePath)

	// when
	journal, err := repo.Load()

	// then
	assert.Error(t, err)
	assert.Nil(t, journal)
	assert.Contains(t, err.Error(), "failed to read journal file")
}

func TestJSONJournalRepository_Clear_WithEmptyStagingDir(t *testing.T) {
	// given
	basePath := t.TempDir()
	repo := repositories.NewJSONJournalRepository(basePath)
	journal := &entities.Journal{
		Timestamp:  time.Now(),
		StagingDir: "",
		Operations: []entities.JournalOperation{},
	}
	err := repo.Save(journal)
	assert.NoError(t, err)
	assert.True(t, repo.Exists())

	// when
	err = repo.Clear()

	// then
	assert.NoError(t, err)
	assert.False(t, repo.Exists())
}

func TestJSONJournalRepository_Save_ShouldCreateDirectoryIfMissing(t *testing.T) {
	// given
	basePath := filepath.Join(t.TempDir(), "nested", "deep", "journal")
	repo := repositories.NewJSONJournalRepository(basePath)
	journal := &entities.Journal{
		Timestamp:  time.Now(),
		StagingDir: "/tmp/staging",
		Operations: []entities.JournalOperation{},
	}

	// when
	err := repo.Save(journal)

	// then
	assert.NoError(t, err)
	assert.True(t, repo.Exists())
}

func TestJSONJournalRepository_Clear_WithCorruptJournalFile(t *testing.T) {
	// given
	basePath := t.TempDir()
	repo := repositories.NewJSONJournalRepository(basePath)
	// Write invalid JSON as journal file
	journalPath := filepath.Join(basePath, "journal.json")
	assert.NoError(t, os.WriteFile(journalPath, []byte("{corrupt!"), 0644))
	assert.True(t, repo.Exists())

	// when -- Clear should still remove the journal file even if Load fails
	err := repo.Clear()

	// then
	assert.NoError(t, err)
	assert.False(t, repo.Exists())
}

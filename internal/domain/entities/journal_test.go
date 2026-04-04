//go:build unit

package entities_test

import (
	"testing"
	"github.com/rios0rios0/aisync/internal/domain/entities"

	"github.com/stretchr/testify/assert"
)

func TestNewJournal_CreatesWithCorrectFields(t *testing.T) {
	// given
	stagingDir := "/tmp/aisync-staging-abc"

	// when
	j := entities.NewJournal(stagingDir)

	// then
	assert.Equal(t, stagingDir, j.StagingDir)
	assert.False(t, j.Timestamp.IsZero(), "Timestamp should be set")
	assert.NotNil(t, j.Operations)
	assert.Len(t, j.Operations, 0)
}

func TestNewJournal_OperationsStartEmpty(t *testing.T) {
	// given / when
	j := entities.NewJournal("/tmp/staging")

	// then
	assert.Empty(t, j.Operations)
}

func TestJournal_AddOperation_AppendsPendingOperation(t *testing.T) {
	// given
	j := entities.NewJournal("/tmp/staging")

	// when
	j.AddOperation("/tmp/staging/file.md", "/home/user/.claude/rules/file.md", "old123", "new456")

	// then
	assert.Len(t, j.Operations, 1)
	op := j.Operations[0]
	assert.Equal(t, "/tmp/staging/file.md", op.SourcePath)
	assert.Equal(t, "/home/user/.claude/rules/file.md", op.TargetPath)
	assert.Equal(t, "old123", op.OldChecksum)
	assert.Equal(t, "new456", op.NewChecksum)
	assert.Equal(t, "pending", op.Status)
}

func TestJournal_AddOperation_MultipleOperations(t *testing.T) {
	// given
	j := entities.NewJournal("/tmp/staging")

	// when
	j.AddOperation("/tmp/a", "/target/a", "", "aaa")
	j.AddOperation("/tmp/b", "/target/b", "old-b", "bbb")
	j.AddOperation("/tmp/c", "/target/c", "old-c", "ccc")

	// then
	assert.Len(t, j.Operations, 3)
	assert.Equal(t, "pending", j.Operations[0].Status)
	assert.Equal(t, "pending", j.Operations[1].Status)
	assert.Equal(t, "pending", j.Operations[2].Status)
}

func TestJournal_AddOperation_EmptyOldChecksum(t *testing.T) {
	// given
	j := entities.NewJournal("/tmp/staging")

	// when
	j.AddOperation("/tmp/new-file", "/target/new-file", "", "newchecksum")

	// then
	assert.Equal(t, "", j.Operations[0].OldChecksum)
	assert.Equal(t, "newchecksum", j.Operations[0].NewChecksum)
}

func TestJournal_MarkApplied_SetsStatusToApplied(t *testing.T) {
	// given
	j := entities.NewJournal("/tmp/staging")
	j.AddOperation("/tmp/a", "/target/a", "", "aaa")
	j.AddOperation("/tmp/b", "/target/b", "", "bbb")

	// when
	j.MarkApplied("/target/a")

	// then
	assert.Equal(t, "applied", j.Operations[0].Status)
	assert.Equal(t, "pending", j.Operations[1].Status)
}

func TestJournal_MarkApplied_NonExistentTargetIsNoOp(t *testing.T) {
	// given
	j := entities.NewJournal("/tmp/staging")
	j.AddOperation("/tmp/a", "/target/a", "", "aaa")

	// when
	j.MarkApplied("/target/nonexistent")

	// then
	assert.Equal(t, "pending", j.Operations[0].Status)
}

func TestJournal_MarkApplied_AllOperations(t *testing.T) {
	// given
	j := entities.NewJournal("/tmp/staging")
	j.AddOperation("/tmp/a", "/target/a", "", "aaa")
	j.AddOperation("/tmp/b", "/target/b", "", "bbb")

	// when
	j.MarkApplied("/target/a")
	j.MarkApplied("/target/b")

	// then
	assert.Equal(t, "applied", j.Operations[0].Status)
	assert.Equal(t, "applied", j.Operations[1].Status)
}

func TestJournal_IsComplete_ReturnsTrueWhenAllApplied(t *testing.T) {
	// given
	j := entities.NewJournal("/tmp/staging")
	j.AddOperation("/tmp/a", "/target/a", "", "aaa")
	j.AddOperation("/tmp/b", "/target/b", "", "bbb")
	j.MarkApplied("/target/a")
	j.MarkApplied("/target/b")

	// when
	result := j.IsComplete()

	// then
	assert.True(t, result)
}

func TestJournal_IsComplete_ReturnsFalseWhenSomePending(t *testing.T) {
	// given
	j := entities.NewJournal("/tmp/staging")
	j.AddOperation("/tmp/a", "/target/a", "", "aaa")
	j.AddOperation("/tmp/b", "/target/b", "", "bbb")
	j.MarkApplied("/target/a")

	// when
	result := j.IsComplete()

	// then
	assert.False(t, result)
}

func TestJournal_IsComplete_ReturnsFalseWhenNoneApplied(t *testing.T) {
	// given
	j := entities.NewJournal("/tmp/staging")
	j.AddOperation("/tmp/a", "/target/a", "", "aaa")

	// when
	result := j.IsComplete()

	// then
	assert.False(t, result)
}

func TestJournal_IsComplete_ReturnsFalseForEmptyOperations(t *testing.T) {
	// given
	j := entities.NewJournal("/tmp/staging")

	// when
	result := j.IsComplete()

	// then
	assert.False(t, result, "empty journal should not be considered complete")
}

func TestJournal_IsComplete_SingleOperationApplied(t *testing.T) {
	// given
	j := entities.NewJournal("/tmp/staging")
	j.AddOperation("/tmp/a", "/target/a", "", "aaa")
	j.MarkApplied("/target/a")

	// when
	result := j.IsComplete()

	// then
	assert.True(t, result)
}

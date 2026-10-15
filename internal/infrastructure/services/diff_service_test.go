package services_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	services "github.com/rios0rios0/aisync/internal/infrastructure/services"
)

func newTestConfig(toolPath string) *entities.Config {
	return &entities.Config{
		Tools: map[string]entities.Tool{
			"claude": {Path: toolPath, Enabled: true},
		},
	}
}

func TestFSDiffService_ComputeSharedDiff_ShouldDetectNewFile(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "claude")
	repoDir := filepath.Join(tmpDir, "repo")
	assert.NoError(t, os.MkdirAll(toolDir, 0700))
	assert.NoError(t, os.MkdirAll(repoDir, 0700))

	config := newTestConfig(toolDir)
	svc := services.NewFSDiffService()

	incomingFiles := map[string][]byte{
		"shared/claude/rules/new-rule.md": []byte("# New Rule\nContent here."),
	}

	// when
	changes, err := svc.ComputeSharedDiff(config, repoDir, incomingFiles)

	// then
	require.NoError(t, err)
	assert.Len(t, changes, 1)
	assert.Equal(t, entities.ChangeAdded, changes[0].Direction)
	assert.Equal(t, "rules/new-rule.md", changes[0].Path)
	assert.Equal(t, "shared", changes[0].Namespace)
}

func TestFSDiffService_ComputeSharedDiff_ShouldDetectModifiedFile(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "claude")
	repoDir := filepath.Join(tmpDir, "repo")
	assert.NoError(t, os.MkdirAll(filepath.Join(toolDir, "rules"), 0700))
	assert.NoError(t, os.MkdirAll(repoDir, 0700))

	localContent := []byte("# Old Rule\nOld content.")
	require.NoError(t, os.WriteFile(filepath.Join(toolDir, "rules", "existing.md"), localContent, 0600))

	config := newTestConfig(toolDir)
	svc := services.NewFSDiffService()

	incomingFiles := map[string][]byte{
		"shared/claude/rules/existing.md": []byte("# Updated Rule\nNew content."),
	}

	// when
	changes, err := svc.ComputeSharedDiff(config, repoDir, incomingFiles)

	// then
	require.NoError(t, err)
	assert.Len(t, changes, 1)
	assert.Equal(t, entities.ChangeModified, changes[0].Direction)
	assert.Equal(t, "rules/existing.md", changes[0].Path)
}

func TestFSDiffService_ComputeSharedDiff_ShouldSkipUnchangedFile(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "claude")
	repoDir := filepath.Join(tmpDir, "repo")
	assert.NoError(t, os.MkdirAll(filepath.Join(toolDir, "rules"), 0700))
	assert.NoError(t, os.MkdirAll(repoDir, 0700))

	content := []byte("# Unchanged Rule\nSame content.")
	require.NoError(t, os.WriteFile(filepath.Join(toolDir, "rules", "same.md"), content, 0600))

	config := newTestConfig(toolDir)
	svc := services.NewFSDiffService()

	incomingFiles := map[string][]byte{
		"shared/claude/rules/same.md": content,
	}

	// when
	changes, err := svc.ComputeSharedDiff(config, repoDir, incomingFiles)

	// then
	require.NoError(t, err)
	assert.Empty(t, changes)
}

func TestFSDiffService_ComputeSharedDiff_ShouldSkipDisabledTools(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "cursor")
	repoDir := filepath.Join(tmpDir, "repo")
	assert.NoError(t, os.MkdirAll(toolDir, 0700))
	assert.NoError(t, os.MkdirAll(repoDir, 0700))

	config := &entities.Config{
		Tools: map[string]entities.Tool{
			"cursor": {Path: toolDir, Enabled: false},
		},
	}
	svc := services.NewFSDiffService()

	incomingFiles := map[string][]byte{
		"shared/cursor/rules/rule.md": []byte("content"),
	}

	// when
	changes, err := svc.ComputeSharedDiff(config, repoDir, incomingFiles)

	// then
	require.NoError(t, err)
	assert.Empty(t, changes)
}

func TestFSDiffService_ComputeSharedDiff_ShouldReturnEmptyWhenNoIncomingFiles(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "claude")
	repoDir := filepath.Join(tmpDir, "repo")
	assert.NoError(t, os.MkdirAll(toolDir, 0700))
	assert.NoError(t, os.MkdirAll(repoDir, 0700))

	config := newTestConfig(toolDir)
	svc := services.NewFSDiffService()

	// when
	changes, err := svc.ComputeSharedDiff(config, repoDir, map[string][]byte{})

	// then
	require.NoError(t, err)
	assert.Empty(t, changes)
}

func TestFSDiffService_ComputeSharedDiff_ShouldSkipDeniedPaths(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "claude")
	repoDir := filepath.Join(tmpDir, "repo")
	assert.NoError(t, os.MkdirAll(filepath.Join(toolDir, ".claude"), 0700))
	assert.NoError(t, os.MkdirAll(repoDir, 0700))

	// Write a local file that matches a denied path
	assert.NoError(t, os.WriteFile(
		filepath.Join(toolDir, ".claude", ".credentials.json"),
		[]byte("old"), 0600,
	))

	config := newTestConfig(toolDir)
	svc := services.NewFSDiffService()

	incomingFiles := map[string][]byte{
		"shared/claude/.claude/.credentials.json": []byte("new credentials"),
	}

	// when
	changes, err := svc.ComputeSharedDiff(config, repoDir, incomingFiles)

	// then
	require.NoError(t, err)
	assert.Empty(t, changes, "denied paths should be skipped")
}

func TestExpandHomePath_ShouldExpandTildePrefix(t *testing.T) {
	// given
	path := "~/some/path"

	// when
	result := services.ExpandHomePath(path)

	// then
	assert.NotEqual(t, path, result, "should expand ~/ prefix")
	assert.NotContains(t, result, "~/", "expanded path should not contain ~/")
	assert.Contains(t, result, "some/path")
}

func TestExpandHomePath_ShouldReturnAbsolutePathUnchanged(t *testing.T) {
	// given
	path := "/absolute/path/to/tool"

	// when
	result := services.ExpandHomePath(path)

	// then
	assert.Equal(t, path, result, "absolute path should not be changed")
}

func TestSourceFromIncoming_ShouldReturnToolName(t *testing.T) {
	// given
	relPath := "shared/claude/rules/arch.md"

	// when
	result := services.SourceFromIncoming(relPath, nil)

	// then
	assert.Equal(t, "claude", result)
}

func TestSourceFromIncoming_ShouldReturnUnknownForSingleSegment(t *testing.T) {
	// given
	relPath := "orphan"

	// when
	result := services.SourceFromIncoming(relPath, nil)

	// then
	assert.Equal(t, "unknown", result)
}

func TestChecksumData_ShouldReturnConsistentHash(t *testing.T) {
	// given
	data := []byte("test data")

	// when
	hash1 := services.ChecksumData(data)
	hash2 := services.ChecksumData(data)

	// then
	assert.Equal(t, hash1, hash2)
	assert.Contains(t, hash1, "sha256:")
}

//go:build unit

package services_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	services "github.com/rios0rios0/aisync/internal/infrastructure/services"
)

func newTestConfig(toolName, toolPath string) *entities.Config {
	return &entities.Config{
		Tools: map[string]entities.Tool{
			toolName: {Path: toolPath, Enabled: true},
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

	config := newTestConfig("claude", toolDir)
	svc := services.NewFSDiffService()

	incomingFiles := map[string][]byte{
		"shared/claude/rules/new-rule.md": []byte("# New Rule\nContent here."),
	}

	// when
	changes, err := svc.ComputeSharedDiff(config, repoDir, incomingFiles)

	// then
	assert.NoError(t, err)
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
	assert.NoError(t, os.WriteFile(filepath.Join(toolDir, "rules", "existing.md"), localContent, 0600))

	config := newTestConfig("claude", toolDir)
	svc := services.NewFSDiffService()

	incomingFiles := map[string][]byte{
		"shared/claude/rules/existing.md": []byte("# Updated Rule\nNew content."),
	}

	// when
	changes, err := svc.ComputeSharedDiff(config, repoDir, incomingFiles)

	// then
	assert.NoError(t, err)
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
	assert.NoError(t, os.WriteFile(filepath.Join(toolDir, "rules", "same.md"), content, 0600))

	config := newTestConfig("claude", toolDir)
	svc := services.NewFSDiffService()

	incomingFiles := map[string][]byte{
		"shared/claude/rules/same.md": content,
	}

	// when
	changes, err := svc.ComputeSharedDiff(config, repoDir, incomingFiles)

	// then
	assert.NoError(t, err)
	assert.Len(t, changes, 0)
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
	assert.NoError(t, err)
	assert.Len(t, changes, 0)
}

func TestFSDiffService_ComputeLocalDiff_ShouldDetectNewLocalFile(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "claude")
	repoDir := filepath.Join(tmpDir, "repo")

	// Create a local file with no counterpart in the repo
	assert.NoError(t, os.MkdirAll(toolDir, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(toolDir, "custom-rule.md"), []byte("my custom rule"), 0600))

	// Create personal dir in repo without the file
	personalDir := filepath.Join(repoDir, "personal", "claude")
	assert.NoError(t, os.MkdirAll(personalDir, 0700))

	config := newTestConfig("claude", toolDir)
	svc := services.NewFSDiffService()

	// when
	changes, err := svc.ComputeLocalDiff(config, repoDir)

	// then
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(changes), 1)

	found := false
	for _, c := range changes {
		if c.Direction == entities.ChangeAdded && c.Namespace == "personal" {
			found = true
			break
		}
	}
	assert.True(t, found, "should detect new local file as added personal change")
}

func TestFSDiffService_ComputeLocalDiff_ShouldDetectModifiedLocalFile(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "claude")
	repoDir := filepath.Join(tmpDir, "repo")

	// Create local file
	assert.NoError(t, os.MkdirAll(toolDir, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(toolDir, "settings.json"), []byte("local modified"), 0600))

	// Create repo file with different content
	personalDir := filepath.Join(repoDir, "personal", "claude")
	assert.NoError(t, os.MkdirAll(personalDir, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(personalDir, "settings.json"), []byte("repo version"), 0600))

	config := newTestConfig("claude", toolDir)
	svc := services.NewFSDiffService()

	// when
	changes, err := svc.ComputeLocalDiff(config, repoDir)

	// then
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(changes), 1)

	found := false
	for _, c := range changes {
		if c.Direction == entities.ChangeModified && c.Namespace == "personal" {
			found = true
			break
		}
	}
	assert.True(t, found, "should detect modified local file")
}

func TestFSDiffService_ComputeLocalDiff_ShouldSkipUnchangedLocalFile(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "claude")
	repoDir := filepath.Join(tmpDir, "repo")

	content := []byte("identical content")
	assert.NoError(t, os.MkdirAll(toolDir, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(toolDir, "same.md"), content, 0600))

	personalDir := filepath.Join(repoDir, "personal", "claude")
	assert.NoError(t, os.MkdirAll(personalDir, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(personalDir, "same.md"), content, 0600))

	config := newTestConfig("claude", toolDir)
	svc := services.NewFSDiffService()

	// when
	changes, err := svc.ComputeLocalDiff(config, repoDir)

	// then
	assert.NoError(t, err)
	assert.Len(t, changes, 0)
}

func TestFSDiffService_ComputePersonalDiff_ShouldDetectIncomingFileNotOnDisk(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "claude")
	repoDir := filepath.Join(tmpDir, "repo")

	assert.NoError(t, os.MkdirAll(toolDir, 0700))

	// Create file in repo that does NOT exist locally
	personalDir := filepath.Join(repoDir, "personal", "claude")
	assert.NoError(t, os.MkdirAll(personalDir, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(personalDir, "from-other-device.md"), []byte("remote content"), 0600))

	config := newTestConfig("claude", toolDir)
	svc := services.NewFSDiffService()

	// when
	changes, err := svc.ComputePersonalDiff(config, repoDir)

	// then
	assert.NoError(t, err)
	assert.Len(t, changes, 1)
	assert.Equal(t, entities.ChangeAdded, changes[0].Direction)
	assert.Equal(t, "personal", changes[0].Namespace)
	assert.Contains(t, changes[0].Path, "from-other-device.md")
}

func TestFSDiffService_ComputePersonalDiff_ShouldSkipAgeFiles(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "claude")
	repoDir := filepath.Join(tmpDir, "repo")

	assert.NoError(t, os.MkdirAll(toolDir, 0700))

	personalDir := filepath.Join(repoDir, "personal", "claude")
	assert.NoError(t, os.MkdirAll(personalDir, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(personalDir, "secrets.json.age"), []byte("encrypted"), 0600))

	config := newTestConfig("claude", toolDir)
	svc := services.NewFSDiffService()

	// when
	changes, err := svc.ComputePersonalDiff(config, repoDir)

	// then
	assert.NoError(t, err)
	assert.Len(t, changes, 0)
}

func TestFSDiffService_ComputePersonalDiff_ShouldDetectModifiedFileWhenRepoIsNewer(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "claude")
	repoDir := filepath.Join(tmpDir, "repo")

	assert.NoError(t, os.MkdirAll(toolDir, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(toolDir, "old-file.md"), []byte("old local"), 0600))

	personalDir := filepath.Join(repoDir, "personal", "claude")
	assert.NoError(t, os.MkdirAll(personalDir, 0700))

	repoFilePath := filepath.Join(personalDir, "old-file.md")
	assert.NoError(t, os.WriteFile(repoFilePath, []byte("updated from repo"), 0600))

	// Make the local file older by touching the repo file with a newer timestamp.
	// We set local to an old mtime and repo to a newer mtime.
	oldTime := mustParseTime("2020-01-01T00:00:00Z")
	assert.NoError(t, os.Chtimes(filepath.Join(toolDir, "old-file.md"), oldTime, oldTime))

	config := newTestConfig("claude", toolDir)
	svc := services.NewFSDiffService()

	// when
	changes, err := svc.ComputePersonalDiff(config, repoDir)

	// then
	assert.NoError(t, err)
	assert.Len(t, changes, 1)
	assert.Equal(t, entities.ChangeModified, changes[0].Direction)
}

func TestFSDiffService_ComputePersonalDiff_ShouldSkipWhenLocalIsNewer(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "claude")
	repoDir := filepath.Join(tmpDir, "repo")

	assert.NoError(t, os.MkdirAll(toolDir, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(toolDir, "newer-local.md"), []byte("newer local"), 0600))

	personalDir := filepath.Join(repoDir, "personal", "claude")
	assert.NoError(t, os.MkdirAll(personalDir, 0700))

	repoFilePath := filepath.Join(personalDir, "newer-local.md")
	assert.NoError(t, os.WriteFile(repoFilePath, []byte("older from repo"), 0600))

	// Make the repo file older
	oldTime := mustParseTime("2020-01-01T00:00:00Z")
	assert.NoError(t, os.Chtimes(repoFilePath, oldTime, oldTime))

	config := newTestConfig("claude", toolDir)
	svc := services.NewFSDiffService()

	// when
	changes, err := svc.ComputePersonalDiff(config, repoDir)

	// then
	assert.NoError(t, err)
	assert.Len(t, changes, 0, "should not report changes when local is newer")
}

func TestFSDiffService_ComputeSharedDiff_ShouldReturnEmptyWhenNoIncomingFiles(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "claude")
	repoDir := filepath.Join(tmpDir, "repo")
	assert.NoError(t, os.MkdirAll(toolDir, 0700))
	assert.NoError(t, os.MkdirAll(repoDir, 0700))

	config := newTestConfig("claude", toolDir)
	svc := services.NewFSDiffService()

	// when
	changes, err := svc.ComputeSharedDiff(config, repoDir, map[string][]byte{})

	// then
	assert.NoError(t, err)
	assert.Len(t, changes, 0)
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

	config := newTestConfig("claude", toolDir)
	svc := services.NewFSDiffService()

	incomingFiles := map[string][]byte{
		"shared/claude/.claude/.credentials.json": []byte("new credentials"),
	}

	// when
	changes, err := svc.ComputeSharedDiff(config, repoDir, incomingFiles)

	// then
	assert.NoError(t, err)
	assert.Len(t, changes, 0, "denied paths should be skipped")
}

func TestFSDiffService_ComputeLocalDiff_ShouldSkipDeniedPaths(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "claude")
	repoDir := filepath.Join(tmpDir, "repo")

	// Create a local file in a denied path
	credDir := filepath.Join(toolDir, ".claude")
	assert.NoError(t, os.MkdirAll(credDir, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(credDir, ".credentials.json"), []byte("secret"), 0600))

	// Create personal dir in repo
	personalDir := filepath.Join(repoDir, "personal", "claude")
	assert.NoError(t, os.MkdirAll(personalDir, 0700))

	config := newTestConfig("claude", toolDir)
	svc := services.NewFSDiffService()

	// when
	changes, err := svc.ComputeLocalDiff(config, repoDir)

	// then
	assert.NoError(t, err)

	for _, c := range changes {
		assert.NotContains(t, c.Path, ".credentials.json", "denied paths should be skipped")
	}
}

func TestFSDiffService_ComputePersonalDiff_ShouldHandleMultipleTools(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, "claude")
	cursorDir := filepath.Join(tmpDir, "cursor")
	repoDir := filepath.Join(tmpDir, "repo")

	assert.NoError(t, os.MkdirAll(claudeDir, 0700))
	assert.NoError(t, os.MkdirAll(cursorDir, 0700))

	// Create files in repo that do not exist locally
	claudePersonal := filepath.Join(repoDir, "personal", "claude")
	cursorPersonal := filepath.Join(repoDir, "personal", "cursor")
	assert.NoError(t, os.MkdirAll(claudePersonal, 0700))
	assert.NoError(t, os.MkdirAll(cursorPersonal, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(claudePersonal, "rule.md"), []byte("claude rule"), 0600))
	assert.NoError(t, os.WriteFile(filepath.Join(cursorPersonal, "skill.md"), []byte("cursor skill"), 0600))

	config := &entities.Config{
		Tools: map[string]entities.Tool{
			"claude": {Path: claudeDir, Enabled: true},
			"cursor": {Path: cursorDir, Enabled: true},
		},
	}
	svc := services.NewFSDiffService()

	// when
	changes, err := svc.ComputePersonalDiff(config, repoDir)

	// then
	assert.NoError(t, err)
	assert.Len(t, changes, 2)

	foundClaude := false
	foundCursor := false
	for _, c := range changes {
		if c.Path == "claude/rule.md" {
			foundClaude = true
		}
		if c.Path == "cursor/skill.md" {
			foundCursor = true
		}
	}
	assert.True(t, foundClaude, "should detect personal change for claude tool")
	assert.True(t, foundCursor, "should detect personal change for cursor tool")
}

func TestFSDiffService_ComputeLocalDiff_ShouldSkipDisabledTools(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "claude")
	repoDir := filepath.Join(tmpDir, "repo")

	assert.NoError(t, os.MkdirAll(toolDir, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(toolDir, "file.md"), []byte("content"), 0600))

	personalDir := filepath.Join(repoDir, "personal", "claude")
	assert.NoError(t, os.MkdirAll(personalDir, 0700))

	config := &entities.Config{
		Tools: map[string]entities.Tool{
			"claude": {Path: toolDir, Enabled: false},
		},
	}
	svc := services.NewFSDiffService()

	// when
	changes, err := svc.ComputeLocalDiff(config, repoDir)

	// then
	assert.NoError(t, err)
	assert.Len(t, changes, 0, "should skip disabled tools")
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

func TestFSDiffService_ComputeLocalDiff_ShouldSkipNonExistentToolDir(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "repo")
	assert.NoError(t, os.MkdirAll(repoDir, 0700))

	config := newTestConfig("claude", filepath.Join(tmpDir, "nonexistent"))
	svc := services.NewFSDiffService()

	// when
	changes, err := svc.ComputeLocalDiff(config, repoDir)

	// then
	assert.NoError(t, err)
	assert.Len(t, changes, 0)
}

func TestFSDiffService_ComputePersonalDiff_ShouldSkipNonExistentPersonalDir(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "claude")
	repoDir := filepath.Join(tmpDir, "repo")
	assert.NoError(t, os.MkdirAll(toolDir, 0700))
	// Do NOT create personal dir

	config := newTestConfig("claude", toolDir)
	svc := services.NewFSDiffService()

	// when
	changes, err := svc.ComputePersonalDiff(config, repoDir)

	// then
	assert.NoError(t, err)
	assert.Len(t, changes, 0)
}

func mustParseTime(value string) time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return t
}

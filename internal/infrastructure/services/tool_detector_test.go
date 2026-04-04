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

func TestFSToolDetector_DetectInstalled_ShouldSetEnabledTrueWhenDirExists(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, "claude")
	cursorDir := filepath.Join(tmpDir, "cursor")
	assert.NoError(t, os.MkdirAll(claudeDir, 0700))
	assert.NoError(t, os.MkdirAll(cursorDir, 0700))

	defaults := map[string]entities.Tool{
		"claude": {Path: claudeDir, Enabled: false},
		"cursor": {Path: cursorDir, Enabled: false},
	}
	detector := services.NewFSToolDetector()

	// when
	result := detector.DetectInstalled(defaults)

	// then
	assert.True(t, result["claude"].Enabled)
	assert.True(t, result["cursor"].Enabled)
}

func TestFSToolDetector_DetectInstalled_ShouldSetEnabledFalseWhenDirIsMissing(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	defaults := map[string]entities.Tool{
		"claude": {Path: filepath.Join(tmpDir, "nonexistent-claude"), Enabled: true},
		"codex":  {Path: filepath.Join(tmpDir, "nonexistent-codex"), Enabled: true},
	}
	detector := services.NewFSToolDetector()

	// when
	result := detector.DetectInstalled(defaults)

	// then
	assert.False(t, result["claude"].Enabled)
	assert.False(t, result["codex"].Enabled)
}

func TestFSToolDetector_DetectInstalled_ShouldHandleMixedExistingAndMissing(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	existingDir := filepath.Join(tmpDir, "claude")
	assert.NoError(t, os.MkdirAll(existingDir, 0700))

	defaults := map[string]entities.Tool{
		"claude": {Path: existingDir, Enabled: false},
		"cursor": {Path: filepath.Join(tmpDir, "missing-cursor"), Enabled: true},
	}
	detector := services.NewFSToolDetector()

	// when
	result := detector.DetectInstalled(defaults)

	// then
	assert.True(t, result["claude"].Enabled)
	assert.False(t, result["cursor"].Enabled)
}

func TestFSToolDetector_DetectInstalled_ShouldPreserveOriginalPaths(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, "claude")
	assert.NoError(t, os.MkdirAll(claudeDir, 0700))

	defaults := map[string]entities.Tool{
		"claude": {Path: claudeDir, Enabled: false},
	}
	detector := services.NewFSToolDetector()

	// when
	result := detector.DetectInstalled(defaults)

	// then
	assert.Equal(t, claudeDir, result["claude"].Path)
}

func TestFSToolDetector_DetectInstalled_ShouldReturnEmptyMapWhenNoToolsProvided(t *testing.T) {
	// given
	defaults := map[string]entities.Tool{}
	detector := services.NewFSToolDetector()

	// when
	result := detector.DetectInstalled(defaults)

	// then
	assert.Len(t, result, 0)
}

func TestFSToolDetector_DetectInstalled_ShouldExpandHomePath(t *testing.T) {
	// given
	home, err := os.UserHomeDir()
	assert.NoError(t, err)

	// Create a unique directory under the user's real home
	testDir := filepath.Join(home, ".aisync-test-detect-"+t.Name())
	assert.NoError(t, os.MkdirAll(testDir, 0700))
	defer os.RemoveAll(testDir)

	defaults := map[string]entities.Tool{
		"test-tool": {Path: "~/.aisync-test-detect-" + t.Name(), Enabled: false},
	}
	detector := services.NewFSToolDetector()

	// when
	result := detector.DetectInstalled(defaults)

	// then
	assert.True(t, result["test-tool"].Enabled, "should detect tool with ~/ path expansion")
	assert.Equal(t, "~/.aisync-test-detect-"+t.Name(), result["test-tool"].Path, "original path should be preserved")
}

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

func TestConflictDetector_DetectConflicts_ShouldDetectConflictWhenBothSidesDiverged(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "claude")
	assert.NoError(t, os.MkdirAll(toolDir, 0755))

	localContent := []byte("local version of the file")
	assert.NoError(t, os.WriteFile(filepath.Join(toolDir, "settings.json"), localContent, 0644))

	incomingContent := []byte("remote version from another device")
	incomingFiles := map[string][]byte{
		"settings.json": incomingContent,
	}

	manifest := &entities.Manifest{
		Device: "laptop",
		Files: map[string]entities.ManifestFile{
			"settings.json": {
				Checksum: "sha256:oldchecksum",
			},
		},
	}
	detector := services.NewConflictDetector()

	// when
	conflicts, err := detector.DetectConflicts(toolDir, incomingFiles, manifest, "desktop")

	// then
	assert.NoError(t, err)
	assert.Len(t, conflicts, 1)
	assert.Equal(t, "settings.json", conflicts[0].Path)
	assert.Equal(t, "laptop", conflicts[0].LocalDevice)
	assert.Equal(t, "desktop", conflicts[0].RemoteDevice)
	assert.Equal(t, localContent, conflicts[0].LocalContent)
	assert.Equal(t, incomingContent, conflicts[0].RemoteContent)

	// Verify conflict file was written
	conflictPath := filepath.Join(toolDir, "settings.json.conflict.desktop")
	conflictData, readErr := os.ReadFile(conflictPath)
	assert.NoError(t, readErr)
	assert.Equal(t, incomingContent, conflictData)
}

func TestConflictDetector_DetectConflicts_ShouldNotConflictWhenLocalIsUnchanged(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "claude")
	assert.NoError(t, os.MkdirAll(toolDir, 0755))

	localContent := []byte("unchanged content")
	assert.NoError(t, os.WriteFile(filepath.Join(toolDir, "rules.md"), localContent, 0644))

	localChecksum := services.ChecksumContent(localContent)

	incomingFiles := map[string][]byte{
		"rules.md": []byte("new content from remote"),
	}

	manifest := &entities.Manifest{
		Device: "laptop",
		Files: map[string]entities.ManifestFile{
			"rules.md": {
				Checksum: localChecksum,
			},
		},
	}
	detector := services.NewConflictDetector()

	// when
	conflicts, err := detector.DetectConflicts(toolDir, incomingFiles, manifest, "desktop")

	// then
	assert.NoError(t, err)
	assert.Len(t, conflicts, 0)
}

func TestConflictDetector_DetectConflicts_ShouldNotConflictWhenLocalFileDoesNotExist(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "claude")
	assert.NoError(t, os.MkdirAll(toolDir, 0755))

	incomingFiles := map[string][]byte{
		"new-file.md": []byte("brand new file from remote"),
	}

	manifest := entities.NewManifest("1.0.0", "laptop")
	detector := services.NewConflictDetector()

	// when
	conflicts, err := detector.DetectConflicts(toolDir, incomingFiles, manifest, "desktop")

	// then
	assert.NoError(t, err)
	assert.Len(t, conflicts, 0)
}

func TestConflictDetector_DetectConflicts_ShouldNotConflictWhenBothSidesHaveSameContent(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "claude")
	assert.NoError(t, os.MkdirAll(toolDir, 0755))

	content := []byte("identical content on both sides")
	assert.NoError(t, os.WriteFile(filepath.Join(toolDir, "same.md"), content, 0644))

	incomingFiles := map[string][]byte{
		"same.md": content,
	}

	manifest := entities.NewManifest("1.0.0", "laptop")
	detector := services.NewConflictDetector()

	// when
	conflicts, err := detector.DetectConflicts(toolDir, incomingFiles, manifest, "desktop")

	// then
	assert.NoError(t, err)
	assert.Len(t, conflicts, 0)
}

func TestConflictDetector_ResolveConflict_ShouldRemoveConflictFileWhenChoiceIsLocal(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "claude")
	assert.NoError(t, os.MkdirAll(toolDir, 0755))

	localContent := []byte("local version")
	assert.NoError(t, os.WriteFile(filepath.Join(toolDir, "file.md"), localContent, 0644))

	conflict := entities.Conflict{
		Path:          "file.md",
		LocalDevice:   "laptop",
		RemoteDevice:  "desktop",
		LocalContent:  localContent,
		RemoteContent: []byte("remote version"),
	}

	conflictPath := filepath.Join(toolDir, conflict.ConflictFileName())
	assert.NoError(t, os.WriteFile(conflictPath, conflict.RemoteContent, 0644))

	detector := services.NewConflictDetector()

	// when
	err := detector.ResolveConflict(toolDir, conflict, "local")

	// then
	assert.NoError(t, err)

	// Conflict file should be removed
	_, statErr := os.Stat(conflictPath)
	assert.True(t, os.IsNotExist(statErr))

	// Local file should remain unchanged
	data, readErr := os.ReadFile(filepath.Join(toolDir, "file.md"))
	assert.NoError(t, readErr)
	assert.Equal(t, localContent, data)
}

func TestConflictDetector_ResolveConflict_ShouldReplaceLocalFileWhenChoiceIsRemote(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "claude")
	assert.NoError(t, os.MkdirAll(toolDir, 0755))

	localContent := []byte("local version")
	remoteContent := []byte("remote version that wins")
	assert.NoError(t, os.WriteFile(filepath.Join(toolDir, "file.md"), localContent, 0644))

	conflict := entities.Conflict{
		Path:          "file.md",
		LocalDevice:   "laptop",
		RemoteDevice:  "desktop",
		LocalContent:  localContent,
		RemoteContent: remoteContent,
	}

	conflictPath := filepath.Join(toolDir, conflict.ConflictFileName())
	assert.NoError(t, os.WriteFile(conflictPath, remoteContent, 0644))

	detector := services.NewConflictDetector()

	// when
	err := detector.ResolveConflict(toolDir, conflict, "remote")

	// then
	assert.NoError(t, err)

	// Conflict file should be removed
	_, statErr := os.Stat(conflictPath)
	assert.True(t, os.IsNotExist(statErr))

	// Local file should be replaced with remote content
	data, readErr := os.ReadFile(filepath.Join(toolDir, "file.md"))
	assert.NoError(t, readErr)
	assert.Equal(t, remoteContent, data)
}

func TestConflictDetector_ResolveConflict_ShouldReturnErrorForInvalidChoice(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	conflict := entities.Conflict{
		Path:         "file.md",
		LocalDevice:  "laptop",
		RemoteDevice: "desktop",
	}
	detector := services.NewConflictDetector()

	// when
	err := detector.ResolveConflict(tmpDir, conflict, "invalid")

	// then
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid choice")
}

func TestConflictDetector_DetectConflicts_ShouldDetectMultipleConflictsInOneCall(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "claude")
	assert.NoError(t, os.MkdirAll(toolDir, 0755))

	assert.NoError(t, os.WriteFile(filepath.Join(toolDir, "file1.md"), []byte("local1"), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(toolDir, "file2.json"), []byte("local2"), 0644))

	incomingFiles := map[string][]byte{
		"file1.md":   []byte("remote1"),
		"file2.json": []byte("remote2"),
	}

	manifest := &entities.Manifest{
		Device: "laptop",
		Files: map[string]entities.ManifestFile{
			"file1.md":   {Checksum: "sha256:stale1"},
			"file2.json": {Checksum: "sha256:stale2"},
		},
	}
	detector := services.NewConflictDetector()

	// when
	conflicts, err := detector.DetectConflicts(toolDir, incomingFiles, manifest, "desktop")

	// then
	assert.NoError(t, err)
	assert.Len(t, conflicts, 2)

	paths := []string{conflicts[0].Path, conflicts[1].Path}
	assert.Contains(t, paths, "file1.md")
	assert.Contains(t, paths, "file2.json")
}

func TestConflictDetector_DetectConflicts_ShouldNotConflictWhenLocalFileIsEmpty(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "claude")
	assert.NoError(t, os.MkdirAll(toolDir, 0755))

	// Create an empty local file
	assert.NoError(t, os.WriteFile(filepath.Join(toolDir, "empty.md"), []byte(""), 0644))

	emptyChecksum := services.ChecksumContent([]byte(""))

	incomingFiles := map[string][]byte{
		"empty.md": []byte("remote content"),
	}

	// The manifest records the empty file checksum, so local is unchanged
	manifest := &entities.Manifest{
		Device: "laptop",
		Files: map[string]entities.ManifestFile{
			"empty.md": {Checksum: emptyChecksum},
		},
	}
	detector := services.NewConflictDetector()

	// when
	conflicts, err := detector.DetectConflicts(toolDir, incomingFiles, manifest, "desktop")

	// then
	assert.NoError(t, err)
	assert.Len(t, conflicts, 0, "empty local file unchanged from manifest should not conflict")
}

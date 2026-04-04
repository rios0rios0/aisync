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

func TestJSONManifestRepository_SaveThenLoad(t *testing.T) {
	// given
	repo := repositories.NewJSONManifestRepository()
	toolDir := t.TempDir()
	syncTime := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	original := &entities.Manifest{
		ManagedBy: "aisync",
		Version:   "0.1.0",
		LastSync:  syncTime,
		Device:    "workstation",
		Files: map[string]entities.ManifestFile{
			"rules/architecture.md": {
				Source:    "guide",
				Namespace: "rules",
				Checksum:  "abc123",
			},
			"agents/reviewer.md": {
				Source:    "guide",
				Namespace: "agents",
				Checksum:  "def456",
			},
		},
	}

	// when
	err := repo.Save(toolDir, original)
	assert.NoError(t, err)
	loaded, err := repo.Load(toolDir)

	// then
	assert.NoError(t, err)
	assert.Equal(t, original.ManagedBy, loaded.ManagedBy)
	assert.Equal(t, original.Version, loaded.Version)
	assert.True(t, original.LastSync.Equal(loaded.LastSync))
	assert.Equal(t, original.Device, loaded.Device)
	assert.Equal(t, original.Files, loaded.Files)
}

func TestJSONManifestRepository_ManifestFileName(t *testing.T) {
	// given
	repo := repositories.NewJSONManifestRepository()
	toolDir := t.TempDir()
	manifest := &entities.Manifest{
		ManagedBy: "aisync",
		Version:   "0.1.0",
		LastSync:  time.Now(),
		Device:    "test",
		Files:     make(map[string]entities.ManifestFile),
	}

	// when
	err := repo.Save(toolDir, manifest)

	// then
	assert.NoError(t, err)
	expectedPath := filepath.Join(toolDir, ".aisync-manifest.json")
	assert.FileExists(t, expectedPath)
}

func TestJSONManifestRepository_Load_MissingFile(t *testing.T) {
	// given
	repo := repositories.NewJSONManifestRepository()
	toolDir := t.TempDir()

	// when
	manifest, err := repo.Load(toolDir)

	// then
	assert.Error(t, err)
	assert.Nil(t, manifest)
	assert.Contains(t, err.Error(), "failed to read manifest")
}

func TestJSONManifestRepository_Exists_WithFile(t *testing.T) {
	// given
	repo := repositories.NewJSONManifestRepository()
	toolDir := t.TempDir()
	manifest := &entities.Manifest{
		ManagedBy: "aisync",
		Version:   "0.1.0",
		LastSync:  time.Now(),
		Device:    "test",
		Files:     make(map[string]entities.ManifestFile),
	}
	err := repo.Save(toolDir, manifest)
	assert.NoError(t, err)

	// when
	exists := repo.Exists(toolDir)

	// then
	assert.True(t, exists)
}

func TestJSONManifestRepository_Exists_WithoutFile(t *testing.T) {
	// given
	repo := repositories.NewJSONManifestRepository()
	toolDir := t.TempDir()

	// when
	exists := repo.Exists(toolDir)

	// then
	assert.False(t, exists)
}

func TestJSONManifestRepository_PreservesAllFields(t *testing.T) {
	// given
	repo := repositories.NewJSONManifestRepository()
	toolDir := t.TempDir()
	syncTime := time.Date(2026, 3, 15, 8, 30, 0, 0, time.UTC)
	original := &entities.Manifest{
		ManagedBy: "aisync",
		Version:   "1.2.3",
		LastSync:  syncTime,
		Device:    "laptop-linux",
		Files: map[string]entities.ManifestFile{
			"rules/git-flow.md": {
				Source:    "engineering-guide",
				Namespace: "rules",
				Checksum:  "sha256-aabbccdd",
			},
		},
	}

	// when
	err := repo.Save(toolDir, original)
	assert.NoError(t, err)
	loaded, err := repo.Load(toolDir)

	// then
	assert.NoError(t, err)
	assert.Equal(t, "aisync", loaded.ManagedBy)
	assert.Equal(t, "1.2.3", loaded.Version)
	assert.True(t, syncTime.Equal(loaded.LastSync))
	assert.Equal(t, "laptop-linux", loaded.Device)
	assert.Equal(t, 1, len(loaded.Files))
	file, ok := loaded.Files["rules/git-flow.md"]
	assert.True(t, ok)
	assert.Equal(t, "engineering-guide", file.Source)
	assert.Equal(t, "rules", file.Namespace)
	assert.Equal(t, "sha256-aabbccdd", file.Checksum)
}

func TestJSONManifestRepository_Save_ShouldCreateToolDirectoryIfMissing(t *testing.T) {
	// given
	repo := repositories.NewJSONManifestRepository()
	toolDir := filepath.Join(t.TempDir(), "nested", "tool", "dir")
	manifest := &entities.Manifest{
		ManagedBy: "aisync",
		Version:   "0.1.0",
		LastSync:  time.Now(),
		Device:    "test",
		Files:     make(map[string]entities.ManifestFile),
	}

	// when
	err := repo.Save(toolDir, manifest)

	// then
	assert.NoError(t, err)
	assert.True(t, repo.Exists(toolDir))
}

func TestJSONManifestRepository_Load_InvalidJSON(t *testing.T) {
	// given
	repo := repositories.NewJSONManifestRepository()
	toolDir := t.TempDir()
	manifestPath := filepath.Join(toolDir, ".aisync-manifest.json")
	assert.NoError(t, os.WriteFile(manifestPath, []byte("{invalid json}"), 0644))

	// when
	manifest, err := repo.Load(toolDir)

	// then
	assert.Error(t, err)
	assert.Nil(t, manifest)
	assert.Contains(t, err.Error(), "failed to parse manifest")
}

func TestJSONManifestRepository_Save_EmptyFilesMap(t *testing.T) {
	// given
	repo := repositories.NewJSONManifestRepository()
	toolDir := t.TempDir()
	manifest := &entities.Manifest{
		ManagedBy: "aisync",
		Version:   "0.1.0",
		LastSync:  time.Now(),
		Device:    "device",
		Files:     make(map[string]entities.ManifestFile),
	}

	// when
	err := repo.Save(toolDir, manifest)
	assert.NoError(t, err)
	loaded, err := repo.Load(toolDir)

	// then
	assert.NoError(t, err)
	assert.Len(t, loaded.Files, 0)
}

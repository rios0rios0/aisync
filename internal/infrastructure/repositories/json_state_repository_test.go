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

func TestJSONStateRepository_SaveThenLoad(t *testing.T) {
	// given
	repo := repositories.NewJSONStateRepository()
	repoPath := t.TempDir()
	lastPull := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	lastPush := time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC)
	deviceSync := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	original := &entities.State{
		Devices: []entities.Device{
			{
				ID:       "uuid-1234",
				Name:     "workstation",
				Platform: "amd64",
				OS:       "linux",
				LastSync: deviceSync,
			},
		},
		SourceETags: map[string]string{
			"guide":    "etag-abc",
			"external": "etag-def",
		},
		LastPull: lastPull,
		LastPush: lastPush,
	}

	// when
	err := repo.Save(repoPath, original)
	assert.NoError(t, err)
	loaded, err := repo.Load(repoPath)

	// then
	assert.NoError(t, err)
	assert.Equal(t, len(original.Devices), len(loaded.Devices))
	assert.Equal(t, original.Devices[0].ID, loaded.Devices[0].ID)
	assert.Equal(t, original.Devices[0].Name, loaded.Devices[0].Name)
	assert.Equal(t, original.Devices[0].Platform, loaded.Devices[0].Platform)
	assert.Equal(t, original.Devices[0].OS, loaded.Devices[0].OS)
	assert.True(t, original.Devices[0].LastSync.Equal(loaded.Devices[0].LastSync))
	assert.Equal(t, original.SourceETags, loaded.SourceETags)
	assert.True(t, original.LastPull.Equal(loaded.LastPull))
	assert.True(t, original.LastPush.Equal(loaded.LastPush))
}

func TestJSONStateRepository_StateFileLocation(t *testing.T) {
	// given
	repo := repositories.NewJSONStateRepository()
	repoPath := t.TempDir()
	state := &entities.State{
		Devices:     []entities.Device{},
		SourceETags: make(map[string]string),
	}

	// when
	err := repo.Save(repoPath, state)

	// then
	assert.NoError(t, err)
	expectedPath := filepath.Join(repoPath, ".aisync", "state.json")
	assert.FileExists(t, expectedPath)
}

func TestJSONStateRepository_Load_MissingFile(t *testing.T) {
	// given
	repo := repositories.NewJSONStateRepository()
	repoPath := t.TempDir()

	// when
	state, err := repo.Load(repoPath)

	// then
	assert.Error(t, err)
	assert.Nil(t, state)
	assert.Contains(t, err.Error(), "failed to read state file")
}

func TestJSONStateRepository_Exists_WithFile(t *testing.T) {
	// given
	repo := repositories.NewJSONStateRepository()
	repoPath := t.TempDir()
	state := &entities.State{
		Devices:     []entities.Device{},
		SourceETags: make(map[string]string),
	}
	err := repo.Save(repoPath, state)
	assert.NoError(t, err)

	// when
	exists := repo.Exists(repoPath)

	// then
	assert.True(t, exists)
}

func TestJSONStateRepository_Exists_WithoutFile(t *testing.T) {
	// given
	repo := repositories.NewJSONStateRepository()
	repoPath := t.TempDir()

	// when
	exists := repo.Exists(repoPath)

	// then
	assert.False(t, exists)
}

func TestJSONStateRepository_PreservesAllFields(t *testing.T) {
	// given
	repo := repositories.NewJSONStateRepository()
	repoPath := t.TempDir()
	lastPull := time.Date(2026, 1, 15, 14, 30, 0, 0, time.UTC)
	lastPush := time.Date(2026, 1, 15, 15, 0, 0, 0, time.UTC)
	device1Sync := time.Date(2026, 1, 14, 10, 0, 0, 0, time.UTC)
	device2Sync := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	original := &entities.State{
		Devices: []entities.Device{
			{
				ID:       "id-alpha",
				Name:     "desktop",
				Platform: "amd64",
				OS:       "linux",
				LastSync: device1Sync,
			},
			{
				ID:       "id-beta",
				Name:     "laptop",
				Platform: "arm64",
				OS:       "darwin",
				LastSync: device2Sync,
			},
		},
		SourceETags: map[string]string{
			"source-a": "etag-111",
			"source-b": "etag-222",
			"source-c": "etag-333",
		},
		LastPull: lastPull,
		LastPush: lastPush,
	}

	// when
	err := repo.Save(repoPath, original)
	assert.NoError(t, err)
	loaded, err := repo.Load(repoPath)

	// then
	assert.NoError(t, err)
	assert.Equal(t, 2, len(loaded.Devices))
	assert.Equal(t, "id-alpha", loaded.Devices[0].ID)
	assert.Equal(t, "desktop", loaded.Devices[0].Name)
	assert.Equal(t, "id-beta", loaded.Devices[1].ID)
	assert.Equal(t, "laptop", loaded.Devices[1].Name)
	assert.Equal(t, "arm64", loaded.Devices[1].Platform)
	assert.Equal(t, "darwin", loaded.Devices[1].OS)
	assert.True(t, device2Sync.Equal(loaded.Devices[1].LastSync))
	assert.Equal(t, 3, len(loaded.SourceETags))
	assert.Equal(t, "etag-111", loaded.SourceETags["source-a"])
	assert.Equal(t, "etag-222", loaded.SourceETags["source-b"])
	assert.Equal(t, "etag-333", loaded.SourceETags["source-c"])
	assert.True(t, lastPull.Equal(loaded.LastPull))
	assert.True(t, lastPush.Equal(loaded.LastPush))
}

func TestJSONStateRepository_Save_ShouldCreateAisyncDirectory(t *testing.T) {
	// given
	repo := repositories.NewJSONStateRepository()
	repoPath := t.TempDir()
	state := &entities.State{
		Devices:     []entities.Device{},
		SourceETags: make(map[string]string),
	}

	// Verify .aisync/ does not exist yet
	aisyncDir := filepath.Join(repoPath, ".aisync")
	_, statErr := os.Stat(aisyncDir)
	assert.True(t, os.IsNotExist(statErr))

	// when
	err := repo.Save(repoPath, state)

	// then
	assert.NoError(t, err)

	// .aisync/ directory should now exist
	info, statErr := os.Stat(aisyncDir)
	assert.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

func TestJSONStateRepository_Load_InvalidJSON(t *testing.T) {
	// given
	repo := repositories.NewJSONStateRepository()
	repoPath := t.TempDir()
	aisyncDir := filepath.Join(repoPath, ".aisync")
	assert.NoError(t, os.MkdirAll(aisyncDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(aisyncDir, "state.json"), []byte("{bad json"), 0644))

	// when
	state, err := repo.Load(repoPath)

	// then
	assert.Error(t, err)
	assert.Nil(t, state)
	assert.Contains(t, err.Error(), "failed to parse state file")
}

func TestJSONStateRepository_SaveThenLoad_WithEmptyDevicesList(t *testing.T) {
	// given
	repo := repositories.NewJSONStateRepository()
	repoPath := t.TempDir()
	original := &entities.State{
		Devices:     []entities.Device{},
		SourceETags: map[string]string{"src": "etag-1"},
		LastPull:    time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
		LastPush:    time.Date(2026, 3, 1, 11, 0, 0, 0, time.UTC),
	}

	// when
	err := repo.Save(repoPath, original)
	assert.NoError(t, err)
	loaded, err := repo.Load(repoPath)

	// then
	assert.NoError(t, err)
	assert.Len(t, loaded.Devices, 0)
	assert.Equal(t, "etag-1", loaded.SourceETags["src"])
	assert.True(t, original.LastPull.Equal(loaded.LastPull))
}

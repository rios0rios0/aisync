//go:build unit

package repositories_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/infrastructure/repositories"
)

func TestJSONBundleStateRepository_LoadAndSave(t *testing.T) {
	t.Parallel()

	t.Run("should return empty state when cache file does not exist", func(t *testing.T) {
		// given
		repo := repositories.NewJSONBundleStateRepository(t.TempDir())

		// when
		state, err := repo.Load()

		// then
		require.NoError(t, err)
		assert.NotNil(t, state)
		assert.Empty(t, state.Bundles)
	})

	t.Run("should round-trip entries through Save and Load", func(t *testing.T) {
		// given
		dir := t.TempDir()
		repo := repositories.NewJSONBundleStateRepository(dir)
		original := entities.NewBundleState()
		original.Bundles["abc123def4567890"] = entities.BundleStateEntry{
			OriginalName: "-home-user-aisync",
			Tool:         "claude",
			Target:       "projects",
			LastSeen:     time.Date(2026, 4, 24, 19, 0, 0, 0, time.UTC),
		}

		// when
		require.NoError(t, repo.Save(original))
		loaded, err := repo.Load()

		// then
		require.NoError(t, err)
		assert.Len(t, loaded.Bundles, 1)
		got, ok := loaded.Bundles["abc123def4567890"]
		require.True(t, ok)
		assert.Equal(t, "-home-user-aisync", got.OriginalName)
		assert.Equal(t, "claude", got.Tool)
		assert.Equal(t, "projects", got.Target)
		assert.True(t, got.LastSeen.Equal(original.Bundles["abc123def4567890"].LastSeen))
	})

	t.Run("should write the cache file at 0600 permissions", func(t *testing.T) {
		// given
		dir := t.TempDir()
		repo := repositories.NewJSONBundleStateRepository(dir)
		state := entities.NewBundleState()
		state.Bundles["x"] = entities.BundleStateEntry{OriginalName: "n", Tool: "claude"}

		// when
		require.NoError(t, repo.Save(state))

		// then
		info, err := os.Stat(filepath.Join(dir, "bundle-state.json"))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	})
}

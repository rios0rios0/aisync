package controllers_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/infrastructure/controllers"
)

func TestConfigWantsSystemGit(t *testing.T) {
	t.Parallel()

	t.Run("should be true when the config sets sync.use_system_git", func(t *testing.T) {
		t.Parallel()
		// given
		repoPath := t.TempDir()
		require.NoError(t, os.WriteFile(
			filepath.Join(repoPath, "config.yaml"),
			[]byte("sync:\n  branch: main\n  use_system_git: true\n"),
			0o600,
		))
		root, _ := controllers.NewRootCommand("test")
		require.NoError(t, root.ParseFlags([]string{"--repo", repoPath}))

		// when
		wants := controllers.ConfigWantsSystemGit(root)

		// then
		assert.True(t, wants)
	})

	t.Run("should be false when the config does not set it", func(t *testing.T) {
		t.Parallel()
		// given
		repoPath := t.TempDir()
		require.NoError(
			t,
			os.WriteFile(filepath.Join(repoPath, "config.yaml"), []byte("sync:\n  branch: main\n"), 0o600),
		)
		root, _ := controllers.NewRootCommand("test")
		require.NoError(t, root.ParseFlags([]string{"--repo", repoPath}))

		// when
		wants := controllers.ConfigWantsSystemGit(root)

		// then
		assert.False(t, wants)
	})

	t.Run("should be false when there is no config yet", func(t *testing.T) {
		t.Parallel()
		// given
		root, _ := controllers.NewRootCommand("test")
		require.NoError(t, root.ParseFlags([]string{"--repo", t.TempDir()}))

		// when
		wants := controllers.ConfigWantsSystemGit(root)

		// then
		assert.False(t, wants)
	})
}

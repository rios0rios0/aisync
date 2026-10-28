package controllers_test

import (
	"os"
	"path/filepath"
	"runtime"
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

// TestDefaultExcludePath is not parallel: every sub-test sets AISYNC_EXCLUDE_FILE, and one
// sets HOME, with t.Setenv.
func TestDefaultExcludePath(t *testing.T) {
	t.Run("should name the file AISYNC_EXCLUDE_FILE points at", func(t *testing.T) {
		// given
		path := filepath.Join(t.TempDir(), "phone-excludes")
		t.Setenv("AISYNC_EXCLUDE_FILE", path)

		// when
		got := controllers.DefaultExcludePath()

		// then
		assert.Equal(t, path, got)
	})

	t.Run("should default to exclude beside the repo and key in aisync's config directory", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("the config directory is under %APPDATA% on Windows")
		}
		// given
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("AISYNC_EXCLUDE_FILE", "")

		// when
		got := controllers.DefaultExcludePath()

		// then
		assert.Equal(t, filepath.Join(home, ".config", "aisync", "exclude"), got)
		assert.Equal(t, filepath.Dir(controllers.DefaultRepoPath()), filepath.Dir(got))
	})
}

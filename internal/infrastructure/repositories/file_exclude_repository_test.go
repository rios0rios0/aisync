package repositories_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repositories "github.com/rios0rios0/aisync/internal/infrastructure/repositories"
)

func TestFileExcludeRepository(t *testing.T) {
	t.Parallel()

	t.Run("should exclude nothing, silently, when the file does not exist", func(t *testing.T) {
		t.Parallel()
		// given
		path := filepath.Join(t.TempDir(), "exclude")
		repo := repositories.NewFileExcludeRepository(path)

		// when
		patterns, err := repo.Load()

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, patterns.Len())
		assert.Equal(t, path, repo.Path())
	})

	t.Run("should load the patterns and ignore a line that names no tool", func(t *testing.T) {
		t.Parallel()
		// given
		path := filepath.Join(t.TempDir(), "exclude")
		content := "# this phone keeps its own\nclaude/settings.json\nsettings.json\nopencode/AGENTS.md\n"
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
		repo := repositories.NewFileExcludeRepository(path)

		// when
		first, firstErr := repo.Load()
		second, secondErr := repo.Load()

		// then
		require.NoError(t, firstErr)
		require.NoError(t, secondErr)
		assert.Equal(t, 2, first.Len())
		assert.True(t, first.Matches("claude", "settings.json"))
		assert.True(t, first.Matches("opencode", "AGENTS.md"))
		assert.False(t, first.Matches("cursor", "settings.json"))
		assert.Equal(t, first.Patterns, second.Patterns, "every Load reads the same file the same way")
	})

	t.Run("should pick up an edit on the next load", func(t *testing.T) {
		t.Parallel()
		// given
		path := filepath.Join(t.TempDir(), "exclude")
		require.NoError(t, os.WriteFile(path, []byte("claude/settings.json\n"), 0o600))
		repo := repositories.NewFileExcludeRepository(path)
		_, err := repo.Load()
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, []byte("claude/CLAUDE.md\n"), 0o600))

		// when
		patterns, err := repo.Load()

		// then
		require.NoError(t, err)
		assert.False(t, patterns.Matches("claude", "settings.json"))
		assert.True(t, patterns.Matches("claude", "CLAUDE.md"))
	})

	t.Run("should fail when the file exists but cannot be read", func(t *testing.T) {
		t.Parallel()
		// given
		path := filepath.Join(t.TempDir(), "exclude")
		require.NoError(t, os.Mkdir(path, 0o700))
		repo := repositories.NewFileExcludeRepository(path)

		// when
		patterns, err := repo.Load()

		// then
		require.Error(t, err)
		assert.Nil(t, patterns)
		assert.Contains(t, err.Error(), path)
	})
}

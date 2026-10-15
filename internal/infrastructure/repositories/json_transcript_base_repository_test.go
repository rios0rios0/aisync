package repositories_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repositories "github.com/rios0rios0/aisync/internal/infrastructure/repositories"
)

func TestJSONTranscriptBaseRepository(t *testing.T) {
	t.Parallel()

	t.Run("should load an empty base before the first conversation sync", func(t *testing.T) {
		t.Parallel()
		// given
		repo := repositories.NewJSONTranscriptBaseRepository()

		// when
		base, err := repo.Load(t.TempDir())

		// then
		require.NoError(t, err)
		assert.Empty(t, base)
	})

	t.Run("should load what was saved", func(t *testing.T) {
		t.Parallel()
		// given
		repo := repositories.NewJSONTranscriptBaseRepository()
		repoPath := t.TempDir()
		saved := map[string]string{"claude/-proj/0e1f.jsonl": "sha256:abc"}

		// when
		require.NoError(t, repo.Save(repoPath, saved))
		loaded, err := repo.Load(repoPath)

		// then
		require.NoError(t, err)
		assert.Equal(t, saved, loaded)
		info, statErr := os.Stat(filepath.Join(repoPath, ".aisync", "transcripts-base.json"))
		require.NoError(t, statErr)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	})

	t.Run("should return an error when the file is corrupt", func(t *testing.T) {
		t.Parallel()
		// given
		repo := repositories.NewJSONTranscriptBaseRepository()
		repoPath := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(repoPath, ".aisync"), 0o700))
		require.NoError(
			t,
			os.WriteFile(filepath.Join(repoPath, ".aisync", "transcripts-base.json"), []byte("{"), 0o600),
		)

		// when
		_, err := repo.Load(repoPath)

		// then
		assert.Error(t, err)
	})
}

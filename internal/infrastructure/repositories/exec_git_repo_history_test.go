package repositories_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repositories "github.com/rios0rios0/aisync/internal/infrastructure/repositories"
)

// commitFile writes content to relPath in dir and commits it with message at
// the given author/committer time.
func commitFile(t *testing.T, dir, relPath, content, message string, when time.Time) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o700))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o600))
	stamp := when.Format(time.RFC3339)
	for _, args := range [][]string{{"add", relPath}, {"commit", "-q", "-m", message}} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+stamp, "GIT_COMMITTER_DATE="+stamp)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
}

func TestExecGitRepoHistory_LastChange(t *testing.T) {
	t.Parallel()

	t.Run("should return the time and subject of the latest commit touching the path", func(t *testing.T) {
		t.Parallel()
		// given
		_, dir := initTempRepo(t)
		first := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
		second := first.Add(48 * time.Hour)
		commitFile(t, dir, "personal/claude/rules/go.md", "v1", "sync(laptop): updated personal configurations", first)
		commitFile(
			t,
			dir,
			"personal/claude/rules/go.md",
			"v2",
			"sync(desktop): updated personal configurations",
			second,
		)
		commitFile(t, dir, "README.md", "unrelated", "docs: touched another file", second.Add(time.Hour))
		history := repositories.NewExecGitRepoHistory()

		// when
		when, subject, err := history.LastChange(dir, "personal/claude/rules/go.md")

		// then
		require.NoError(t, err)
		assert.True(t, second.Equal(when), "expected %s, got %s", second, when)
		assert.Equal(t, "sync(desktop): updated personal configurations", subject)
	})

	t.Run("should return a zero time when the path has no history", func(t *testing.T) {
		t.Parallel()
		// given
		_, dir := initTempRepo(t)
		commitFile(t, dir, "README.md", "x", "docs: initial", time.Now())
		history := repositories.NewExecGitRepoHistory()

		// when
		when, subject, err := history.LastChange(dir, "personal/claude/rules/missing.md")

		// then
		require.NoError(t, err)
		assert.True(t, when.IsZero())
		assert.Empty(t, subject)
	})

	t.Run("should return an error when the directory is not a repository", func(t *testing.T) {
		t.Parallel()
		// given
		skipIfNoGit(t)
		history := repositories.NewExecGitRepoHistory()

		// when
		_, _, err := history.LastChange(t.TempDir(), "personal/claude/rules/go.md")

		// then
		assert.Error(t, err)
	})
}

func TestExecGitRepoHistory_LastChange_EncryptionOnly(t *testing.T) {
	t.Parallel()

	t.Run("should skip an encryption-only commit and find the change under the plaintext name", func(t *testing.T) {
		t.Parallel()
		// given
		_, dir := initTempRepo(t)
		changed := time.Date(2026, 9, 25, 13, 0, 0, 0, time.UTC)
		encrypted := changed.Add(20 * 24 * time.Hour)
		commitFile(t, dir, "personal/claude/rules/go.md", "rule", "feat(rules): updated the Go rule", changed)
		out, rmErr := exec.Command("git", "-C", dir, "rm", "-q", "personal/claude/rules/go.md").CombinedOutput()
		require.NoError(t, rmErr, string(out))
		commitFile(
			t,
			dir,
			"personal/claude/rules/go.md.age",
			"ciphertext",
			"sync(laptop): encrypted 1 personal file(s) in place\n\nAisync-Change: encryption-only",
			encrypted,
		)
		history := repositories.NewExecGitRepoHistory()

		// when
		when, subject, err := history.LastChange(dir, "personal/claude/rules/go.md", "personal/claude/rules/go.md.age")

		// then
		require.NoError(t, err)
		assert.True(t, changed.Equal(when), "expected %s, got %s", changed, when)
		assert.Equal(t, "feat(rules): updated the Go rule", subject)
	})

	t.Run("should report a later content change of the encrypted copy", func(t *testing.T) {
		t.Parallel()
		// given
		_, dir := initTempRepo(t)
		first := time.Date(2026, 9, 25, 13, 0, 0, 0, time.UTC)
		commitFile(
			t,
			dir,
			"personal/claude/rules/go.md.age",
			"ciphertext v1",
			"sync(laptop): encrypted 1 personal file(s) in place\n\nAisync-Change: encryption-only",
			first,
		)
		commitFile(
			t,
			dir,
			"personal/claude/rules/go.md.age",
			"ciphertext v2",
			"sync(desktop): updated personal configurations",
			first.Add(time.Hour),
		)
		history := repositories.NewExecGitRepoHistory()

		// when
		when, subject, err := history.LastChange(dir, "personal/claude/rules/go.md", "personal/claude/rules/go.md.age")

		// then
		require.NoError(t, err)
		assert.True(t, first.Add(time.Hour).Equal(when))
		assert.Equal(t, "sync(desktop): updated personal configurations", subject)
	})

	t.Run("should return a zero time when every commit only encrypted the file", func(t *testing.T) {
		t.Parallel()
		// given
		_, dir := initTempRepo(t)
		commitFile(
			t,
			dir,
			"personal/claude/rules/go.md.age",
			"ciphertext",
			"sync(laptop): encrypted 1 personal file(s) in place\n\nAisync-Change: encryption-only",
			time.Now(),
		)
		history := repositories.NewExecGitRepoHistory()

		// when
		when, subject, err := history.LastChange(dir, "personal/claude/rules/go.md.age")

		// then
		require.NoError(t, err)
		assert.True(t, when.IsZero())
		assert.Empty(t, subject)
	})
}

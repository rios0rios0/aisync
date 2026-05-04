//go:build unit

package repositories_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repositories "github.com/rios0rios0/aisync/internal/infrastructure/repositories"
)

func TestSSHConfigAliasRepository_ResolveAliases(t *testing.T) {
	// Sub-tests use t.Setenv to point HOME at a per-test temp directory,
	// which is incompatible with t.Parallel() at any level of the tree.

	t.Run("should return aliases for matching HostName when single alias is configured", func(t *testing.T) {
		// given
		home := writeSSHConfig(t, `Host github.com-mine
  HostName github.com
  IdentityFile ~/.ssh/id_ed25519_mine
`)
		t.Setenv("HOME", home)
		repo := repositories.NewSSHConfigAliasRepository()

		// when
		aliases, err := repo.ResolveAliases("github.com")

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"github.com-mine"}, aliases)
	})

	t.Run("should return both aliases when multiple Host blocks share the same HostName", func(t *testing.T) {
		// given
		home := writeSSHConfig(t, `Host github.com-personal
  HostName github.com

Host github.com-work
  HostName github.com
`)
		t.Setenv("HOME", home)
		repo := repositories.NewSSHConfigAliasRepository()

		// when
		aliases, err := repo.ResolveAliases("github.com")

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"github.com-personal", "github.com-work"}, aliases)
	})

	t.Run("should return all space-separated aliases when one Host line lists multiple names", func(t *testing.T) {
		// given
		home := writeSSHConfig(t, `Host github.com-mine github.com-alt
  HostName github.com
`)
		t.Setenv("HOME", home)
		repo := repositories.NewSSHConfigAliasRepository()

		// when
		aliases, err := repo.ResolveAliases("github.com")

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"github.com-mine", "github.com-alt"}, aliases)
	})

	t.Run("should skip wildcard Host patterns since they cannot be used as clone hostnames", func(t *testing.T) {
		// given
		home := writeSSHConfig(t, `Host *
  IdentityFile ~/.ssh/id_default

Host github.com-?
  HostName github.com

Host github.com-mine
  HostName github.com
`)
		t.Setenv("HOME", home)
		repo := repositories.NewSSHConfigAliasRepository()

		// when
		aliases, err := repo.ResolveAliases("github.com")

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"github.com-mine"}, aliases)
	})

	t.Run("should match HostName case-insensitively per the SSH spec", func(t *testing.T) {
		// given
		home := writeSSHConfig(t, `Host github.com-mine
  HostName GitHub.Com
`)
		t.Setenv("HOME", home)
		repo := repositories.NewSSHConfigAliasRepository()

		// when
		aliases, err := repo.ResolveAliases("github.com")

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"github.com-mine"}, aliases)
	})

	t.Run("should return nil aliases without error when ~/.ssh/config does not exist", func(t *testing.T) {
		// given
		t.Setenv("HOME", t.TempDir())
		repo := repositories.NewSSHConfigAliasRepository()

		// when
		aliases, err := repo.ResolveAliases("github.com")

		// then
		require.NoError(t, err)
		assert.Nil(t, aliases)
	})

	t.Run("should return nil aliases for hostnames that do not match any Host block", func(t *testing.T) {
		// given
		home := writeSSHConfig(t, `Host github.com-mine
  HostName github.com
`)
		t.Setenv("HOME", home)
		repo := repositories.NewSSHConfigAliasRepository()

		// when
		aliases, err := repo.ResolveAliases("gitlab.com")

		// then
		require.NoError(t, err)
		assert.Nil(t, aliases)
	})

	t.Run("should propagate scan failure when ~/.ssh/config has a line longer than the scanner buffer", func(t *testing.T) {
		// given — bufio.Scanner's default buffer is 64 KiB; pad past it.
		longValue := make([]byte, 70_000)
		for i := range longValue {
			longValue[i] = 'x'
		}
		home := writeSSHConfig(t, "Host github.com-mine\n  HostName "+string(longValue)+"\n")
		t.Setenv("HOME", home)
		repo := repositories.NewSSHConfigAliasRepository()

		// when
		aliases, err := repo.ResolveAliases("github.com")

		// then
		require.Error(t, err, "very long lines must surface a scan error so callers can log it")
		assert.Nil(t, aliases)
	})
}

// writeSSHConfig writes content to <tempDir>/.ssh/config and returns the temp
// directory so the caller can point HOME at it.
func writeSSHConfig(t *testing.T, content string) string {
	t.Helper()
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	require.NoError(t, os.MkdirAll(sshDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(sshDir, "config"), []byte(content), 0600))
	return home
}

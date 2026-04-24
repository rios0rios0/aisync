//go:build unit

package commands_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/domain/commands"
	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/test/doubles"
)

func TestPruneBundlesCommand(t *testing.T) {
	t.Parallel()

	t.Run("should remove orphan bundles after user confirms", func(t *testing.T) {
		// given — local source has one project ("alive"); the sync repo
		// has two bundles: one for "alive" (matches HashName) and one
		// for an orphaned "deleted" (no source dir). The user confirms
		// every prune prompt.
		toolPath := t.TempDir()
		repoPath := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(toolPath, "projects", "alive"), 0o700))

		bundleDir := filepath.Join(repoPath, "personal", "claude", "projects")
		require.NoError(t, os.MkdirAll(bundleDir, 0o700))
		// MockBundleService.HashName returns "h_<name>"; produce the
		// matching .age files so prune sees both an orphan and a live one.
		require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "h_alive.age"), []byte("alive"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "h_deleted.age"), []byte("orphan"), 0o600))

		config := &entities.Config{
			Tools: map[string]entities.Tool{
				"claude": {
					Path:    toolPath,
					Enabled: true,
					Bundles: []entities.BundleSpec{{Source: "projects", Target: "projects"}},
				},
			},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		bundleSvc := &doubles.MockBundleService{}
		prompt := &doubles.MockPromptService{Confirmation: true}

		cmd := commands.NewPruneBundlesCommand(configRepo, bundleSvc, prompt)

		// when
		result, err := cmd.Execute("/dev/null", repoPath)

		// then
		require.NoError(t, err)
		assert.Equal(t, 2, result.Scanned)
		assert.Equal(t, 1, result.Removed)
		_, err = os.Stat(filepath.Join(bundleDir, "h_deleted.age"))
		assert.True(t, os.IsNotExist(err), "orphan bundle should be deleted")
		_, err = os.Stat(filepath.Join(bundleDir, "h_alive.age"))
		assert.NoError(t, err, "live bundle must survive")
	})

	t.Run("should keep orphan bundle when user declines", func(t *testing.T) {
		// given
		toolPath := t.TempDir()
		repoPath := t.TempDir()
		bundleDir := filepath.Join(repoPath, "personal", "claude", "projects")
		require.NoError(t, os.MkdirAll(bundleDir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "h_orphan.age"), []byte("o"), 0o600))

		config := &entities.Config{
			Tools: map[string]entities.Tool{
				"claude": {
					Path:    toolPath,
					Enabled: true,
					Bundles: []entities.BundleSpec{{Source: "projects", Target: "projects"}},
				},
			},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		bundleSvc := &doubles.MockBundleService{}
		prompt := &doubles.MockPromptService{Confirmation: false}

		cmd := commands.NewPruneBundlesCommand(configRepo, bundleSvc, prompt)

		// when
		result, err := cmd.Execute("/dev/null", repoPath)

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, result.Removed)
		_, err = os.Stat(filepath.Join(bundleDir, "h_orphan.age"))
		assert.NoError(t, err, "declined orphan must survive")
	})
}

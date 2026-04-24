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
	"github.com/rios0rios0/aisync/internal/domain/repositories"
	"github.com/rios0rios0/aisync/test/doubles"
)

// newPullCommandWithBundles returns a PullCommand wired solely for the
// post-file-apply bundle step. Most fields stay nil because applyBundles
// only touches bundleService, bundleStateRepo, and promptService.
func newPullCommandWithBundles(
	bundleSvc repositories.BundleService,
	stateRepo repositories.BundleStateRepository,
	prompt repositories.PromptService,
) *commands.PullCommand {
	return commands.NewPullCommand(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		prompt,
		bundleSvc, stateRepo,
	)
}

func TestPullCommand_PromptToRemoveBundleDeletedUpstream(t *testing.T) {
	t.Parallel()

	t.Run("should remove the local source dir when the user confirms", func(t *testing.T) {
		// given — cache says we last saw bundle "stale-hash" mapped to
		// project "old-project"; the freshly-pulled bundle dir is empty
		// (the project was pruned upstream), and the local source dir
		// still exists. The prompt service is wired to confirm.
		toolPath := t.TempDir()
		repoPath := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(toolPath, "projects", "old-project"), 0o700))
		require.NoError(t, os.WriteFile(
			filepath.Join(toolPath, "projects", "old-project", "MEMORY.md"),
			[]byte("local notes"), 0o600,
		))
		require.NoError(t, os.MkdirAll(filepath.Join(repoPath, "personal", "claude", "projects"), 0o700))

		cached := entities.NewBundleState()
		cached.Bundles["stale-hash"] = entities.BundleStateEntry{
			OriginalName: "old-project",
			Tool:         "claude",
			Target:       "projects",
		}
		stateRepo := &doubles.MockBundleStateRepository{State: cached}
		bundleSvc := &doubles.MockBundleService{}
		prompt := &doubles.MockPromptService{Confirmation: true}

		cmd := newPullCommandWithBundles(bundleSvc, stateRepo, prompt)

		// We have to call applyBundles via the public Execute path? It's
		// unexported. Use a config-driven Execute is heavy; instead reach
		// the unexported method by exposing the private hook through a
		// thin wrapper file ... but pull_bundles.go's applyBundles is on
		// PullCommand. Calling it directly requires same-package access.
		// We expose deletion behaviour by re-running applyBundles via a
		// trivial wiring: configure a config + repo that triggers it.
		config := &entities.Config{
			Encryption: entities.EncryptionConfig{Identity: "/dev/null"},
			Tools: map[string]entities.Tool{
				"claude": {
					Path:    toolPath,
					Enabled: true,
					Bundles: []entities.BundleSpec{{Source: "projects", Target: "projects"}},
				},
			},
		}

		// when
		commands.ExposeApplyBundles(cmd, config, repoPath)

		// then
		assert.GreaterOrEqual(t, prompt.ConfirmationCalls, 1)
		_, err := os.Stat(filepath.Join(toolPath, "projects", "old-project"))
		assert.True(t, os.IsNotExist(err), "expected local project dir to be removed")
		require.NotNil(t, stateRepo.Saved)
		assert.Empty(t, stateRepo.Saved.Bundles, "saved state should reflect the empty current set")
	})

	t.Run("should keep the local source dir when the user declines", func(t *testing.T) {
		// given
		toolPath := t.TempDir()
		repoPath := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(toolPath, "projects", "keep-me"), 0o700))
		require.NoError(t, os.WriteFile(
			filepath.Join(toolPath, "projects", "keep-me", "MEMORY.md"),
			[]byte("notes"), 0o600,
		))
		require.NoError(t, os.MkdirAll(filepath.Join(repoPath, "personal", "claude", "projects"), 0o700))

		cached := entities.NewBundleState()
		cached.Bundles["stale-hash"] = entities.BundleStateEntry{
			OriginalName: "keep-me",
			Tool:         "claude",
			Target:       "projects",
		}
		stateRepo := &doubles.MockBundleStateRepository{State: cached}
		bundleSvc := &doubles.MockBundleService{}
		prompt := &doubles.MockPromptService{Confirmation: false}

		cmd := newPullCommandWithBundles(bundleSvc, stateRepo, prompt)
		config := &entities.Config{
			Encryption: entities.EncryptionConfig{Identity: "/dev/null"},
			Tools: map[string]entities.Tool{
				"claude": {
					Path:    toolPath,
					Enabled: true,
					Bundles: []entities.BundleSpec{{Source: "projects", Target: "projects"}},
				},
			},
		}

		// when
		commands.ExposeApplyBundles(cmd, config, repoPath)

		// then
		assert.GreaterOrEqual(t, prompt.ConfirmationCalls, 1)
		_, err := os.Stat(filepath.Join(toolPath, "projects", "keep-me"))
		assert.NoError(t, err, "local project must survive declined deletion")
	})
}

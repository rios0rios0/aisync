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

// newPushCommandWithBundles wires a PushCommand with only the bits
// produceBundles needs. Most fields stay nil so each test can focus on
// the bundle producer alone.
func newPushCommandWithBundles(
	configRepo *doubles.MockConfigRepository,
	bundleSvc *doubles.MockBundleService,
) *commands.PushCommand {
	return commands.NewPushCommand(
		configRepo,
		&doubles.MockStateRepository{},
		&doubles.MockGitRepository{},
		&doubles.MockEncryptionService{},
		&doubles.MockManifestRepository{},
		&doubles.MockSecretScanner{},
		&doubles.MockNDAContentChecker{},
		bundleSvc,
	)
}

func TestPushCommand_ProduceBundles_WholeMode(t *testing.T) {
	t.Parallel()

	t.Run("should produce one bundle for the whole flat-file source dir", func(t *testing.T) {
		// given — ~/.claude/plans/ contains 3 flat .md files (no subdirs).
		// In whole mode the producer should call Bundle() exactly once
		// with the source root as the path and source label as the name.
		toolPath := t.TempDir()
		repoPath := t.TempDir()
		plansDir := filepath.Join(toolPath, "plans")
		require.NoError(t, os.MkdirAll(plansDir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(plansDir, "alpha.md"), []byte("a"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(plansDir, "beta.md"), []byte("b"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(plansDir, "gamma.md"), []byte("c"), 0o600))

		bundleSvc := &doubles.MockBundleService{
			BundleCipher: []byte("ciphertext"),
			BundleManifest: &entities.BundleManifest{
				OriginalName: "plans",
				FileCount:    3,
			},
		}
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity:   "/tmp/key.txt",
					Recipients: []string{"age1xyz"},
				},
				Tools: map[string]entities.Tool{
					"claude": {
						Path:    toolPath,
						Enabled: true,
						Bundles: []entities.BundleSpec{{
							Source: "plans",
							Target: "plans",
							Mode:   entities.BundleModeWhole,
						}},
					},
				},
			},
		}

		cmd := newPushCommandWithBundles(configRepo, bundleSvc)

		// when
		err := cmd.Execute("/tmp/cfg.yaml", repoPath, "", commands.PushOptions{DryRun: true})

		// then — one Bundle() call with source label as the original name.
		require.NoError(t, err)
		assert.Equal(t, 1, bundleSvc.BundleCalls,
			"whole mode must produce exactly one bundle for the entire source")
		assert.Equal(t, "plans", bundleSvc.LastBundleName,
			"whole mode passes the source label, not a subdir name")
		assert.Equal(t, plansDir, bundleSvc.LastBundleSrc,
			"whole mode bundles the source root itself")
	})

	t.Run("should produce one bundle per subdir in subdirs mode", func(t *testing.T) {
		// given — three subdirs under projects/, each with one file.
		// Confirms the existing subdirs-mode behaviour is preserved.
		toolPath := t.TempDir()
		repoPath := t.TempDir()
		for _, name := range []string{"alpha", "beta", "gamma"} {
			subDir := filepath.Join(toolPath, "projects", name)
			require.NoError(t, os.MkdirAll(subDir, 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(subDir, "MEMORY.md"), []byte("x"), 0o600))
		}

		bundleSvc := &doubles.MockBundleService{
			BundleCipher: []byte("ciphertext"),
			BundleManifest: &entities.BundleManifest{
				FileCount: 1,
			},
		}
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity:   "/tmp/key.txt",
					Recipients: []string{"age1xyz"},
				},
				Tools: map[string]entities.Tool{
					"claude": {
						Path:    toolPath,
						Enabled: true,
						Bundles: []entities.BundleSpec{{
							Source: "projects",
							Target: "projects",
							// Mode left unset — defaults to subdirs.
						}},
					},
				},
			},
		}

		cmd := newPushCommandWithBundles(configRepo, bundleSvc)

		// when
		err := cmd.Execute("/tmp/cfg.yaml", repoPath, "", commands.PushOptions{DryRun: true})

		// then
		require.NoError(t, err)
		assert.Equal(t, 3, bundleSvc.BundleCalls,
			"subdirs mode produces one bundle per immediate subdir")
	})
}

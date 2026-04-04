//go:build unit

package commands_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/domain/commands"
	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/test/doubles"
)

func TestStatusCommand_Execute(t *testing.T) {
	t.Run("should return no error when state and config are valid", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		config := &entities.Config{
			Sync: entities.SyncConfig{
				Remote: "origin",
				Branch: "main",
			},
			Encryption: entities.EncryptionConfig{
				Identity: "/tmp/nonexistent-key.txt",
			},
			Tools: map[string]entities.Tool{},
			Sources: []entities.Source{
				{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated", Refresh: "168h"},
			},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{
			ExistsVal: true,
			State: &entities.State{
				Devices: []entities.Device{
					{ID: "abc", Name: "laptop"},
				},
				LastPull:    time.Now().Add(-1 * time.Hour),
				LastPush:    time.Now().Add(-2 * time.Hour),
				SourceETags: map[string]string{"guide": "etag-1"},
			},
		}
		manifestRepo := &doubles.MockManifestRepository{}
		cmd := commands.NewStatusCommand(configRepo, stateRepo, manifestRepo)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, configRepo.LoadCalls)
	})

	t.Run("should return no error when state does not exist", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		config := &entities.Config{
			Sync: entities.SyncConfig{Branch: "main"},
			Encryption: entities.EncryptionConfig{
				Identity: "/tmp/nonexistent-key.txt",
			},
			Tools:   map[string]entities.Tool{},
			Sources: []entities.Source{},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{ExistsVal: false}
		manifestRepo := &doubles.MockManifestRepository{}
		cmd := commands.NewStatusCommand(configRepo, stateRepo, manifestRepo)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir)

		// then
		require.NoError(t, err)
	})

	t.Run("should return error when config load fails", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{LoadErr: assert.AnError}
		cmd := commands.NewStatusCommand(configRepo, nil, nil)

		// when
		err := cmd.Execute("/tmp/config.yaml", "/tmp/repo")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})

	t.Run("should show encryption enabled when identity file exists", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		identityPath := filepath.Join(tmpDir, "key.txt")
		require.NoError(t, os.WriteFile(identityPath, []byte("AGE-SECRET-KEY"), 0600))

		config := &entities.Config{
			Sync: entities.SyncConfig{
				Remote: "",
				Branch: "main",
			},
			Encryption: entities.EncryptionConfig{
				Identity: identityPath,
			},
			Tools:   map[string]entities.Tool{},
			Sources: []entities.Source{},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{ExistsVal: false}
		manifestRepo := &doubles.MockManifestRepository{}
		cmd := commands.NewStatusCommand(configRepo, stateRepo, manifestRepo)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir)

		// then
		require.NoError(t, err)
	})

	t.Run("should show tool status with managed files when manifest exists", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		toolDir := filepath.Join(tmpDir, "claude")
		require.NoError(t, os.MkdirAll(toolDir, 0755))

		manifest := entities.NewManifest("0.1.0", "test-host")
		manifest.SetFile("rules/test.md", "guide", "shared", "checksum")
		manifest.SetFile("rules/other.md", "guide", "shared", "checksum2")

		config := &entities.Config{
			Sync: entities.SyncConfig{Branch: "main"},
			Encryption: entities.EncryptionConfig{
				Identity: "/tmp/nonexistent-key.txt",
			},
			Tools: map[string]entities.Tool{
				"claude": {Path: toolDir, Enabled: true},
			},
			Sources: []entities.Source{},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{ExistsVal: false}
		manifestRepo := &doubles.MockManifestRepository{
			ExistsVal: true,
			Manifest:  manifest,
		}
		cmd := commands.NewStatusCommand(configRepo, stateRepo, manifestRepo)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir)

		// then
		require.NoError(t, err)
		assert.GreaterOrEqual(t, manifestRepo.LoadCalls, 1)
	})

	t.Run("should show source freshness as stale when last pull is old", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		config := &entities.Config{
			Sync: entities.SyncConfig{
				Remote: "origin",
				Branch: "main",
			},
			Encryption: entities.EncryptionConfig{
				Identity: "/tmp/nonexistent-key.txt",
			},
			Tools: map[string]entities.Tool{},
			Sources: []entities.Source{
				{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated", Refresh: "1h"},
			},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{
			ExistsVal: true,
			State: &entities.State{
				Devices:     []entities.Device{{ID: "abc", Name: "laptop"}},
				LastPull:    time.Now().Add(-48 * time.Hour),
				SourceETags: map[string]string{"guide": "etag-1"},
			},
		}
		manifestRepo := &doubles.MockManifestRepository{}
		cmd := commands.NewStatusCommand(configRepo, stateRepo, manifestRepo)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir)

		// then
		require.NoError(t, err)
	})

	t.Run("should show never fetched when source has no etag", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		config := &entities.Config{
			Sync:       entities.SyncConfig{Branch: "main"},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/nonexistent-key.txt"},
			Tools:      map[string]entities.Tool{},
			Sources: []entities.Source{
				{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
			},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{
			ExistsVal: true,
			State: &entities.State{
				Devices:     []entities.Device{},
				SourceETags: map[string]string{},
			},
		}
		cmd := commands.NewStatusCommand(configRepo, stateRepo, &doubles.MockManifestRepository{})

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir)

		// then
		require.NoError(t, err)
	})

	t.Run("should show incoming hint when push is newer than pull", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		config := &entities.Config{
			Sync: entities.SyncConfig{
				Remote: "origin",
				Branch: "main",
			},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/nonexistent-key.txt"},
			Tools:      map[string]entities.Tool{},
			Sources:    []entities.Source{},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{
			ExistsVal: true,
			State: &entities.State{
				Devices:  []entities.Device{{ID: "abc", Name: "laptop"}},
				LastPull: time.Now().Add(-2 * time.Hour),
				LastPush: time.Now().Add(-1 * time.Hour),
			},
		}
		cmd := commands.NewStatusCommand(configRepo, stateRepo, &doubles.MockManifestRepository{})

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir)

		// then
		require.NoError(t, err)
	})

	t.Run("should count pending local changes when local file differs from sync repo", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		toolDir := filepath.Join(tmpDir, "claude")
		require.NoError(t, os.MkdirAll(toolDir, 0755))

		require.NoError(t, os.WriteFile(filepath.Join(toolDir, "my-rule.md"), []byte("local content"), 0644))

		personalDir := filepath.Join(repoPath, "personal", "claude")
		require.NoError(t, os.MkdirAll(personalDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(personalDir, "my-rule.md"), []byte("repo content"), 0644))

		config := &entities.Config{
			Sync: entities.SyncConfig{Branch: "main"},
			Encryption: entities.EncryptionConfig{
				Identity: "/tmp/nonexistent-key.txt",
			},
			Tools: map[string]entities.Tool{
				"claude": {Path: toolDir, Enabled: true},
			},
			Sources: []entities.Source{},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{ExistsVal: false}
		manifestRepo := &doubles.MockManifestRepository{ExistsVal: false}
		cmd := commands.NewStatusCommand(configRepo, stateRepo, manifestRepo)

		// when
		err := cmd.Execute("/tmp/config.yaml", repoPath)

		// then
		require.NoError(t, err)
	})

	t.Run("should skip shared files from manifest in pending count", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		toolDir := filepath.Join(tmpDir, "claude")
		require.NoError(t, os.MkdirAll(filepath.Join(toolDir, "rules"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(toolDir, "rules/shared.md"), []byte("shared rule"), 0644))

		manifest := entities.NewManifest("0.1.0", "host")
		manifest.SetFile("rules/shared.md", "guide", "shared", "checksum")

		config := &entities.Config{
			Sync:       entities.SyncConfig{Branch: "main"},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/nonexistent-key.txt"},
			Tools: map[string]entities.Tool{
				"claude": {Path: toolDir, Enabled: true},
			},
			Sources: []entities.Source{},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{ExistsVal: false}
		manifestRepo := &doubles.MockManifestRepository{
			ExistsVal: true,
			Manifest:  manifest,
		}
		cmd := commands.NewStatusCommand(configRepo, stateRepo, manifestRepo)

		// when
		err := cmd.Execute("/tmp/config.yaml", repoPath)

		// then
		require.NoError(t, err)
	})
}

package commands_test

import (
	"bytes"
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
	t.Parallel()

	t.Run("should return no error when state and config are valid", func(t *testing.T) {
		t.Parallel()
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
		cmd := commands.NewStatusCommand(configRepo, stateRepo, manifestRepo, newStatusSyncer(manifestRepo))

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, configRepo.LoadCalls)
	})

	t.Run("should return no error when state does not exist", func(t *testing.T) {
		t.Parallel()
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
		cmd := commands.NewStatusCommand(configRepo, stateRepo, manifestRepo, newStatusSyncer(manifestRepo))

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir)

		// then
		require.NoError(t, err)
	})

	t.Run("should return error when config load fails", func(t *testing.T) {
		t.Parallel()
		// given
		configRepo := &doubles.MockConfigRepository{LoadErr: assert.AnError}
		cmd := commands.NewStatusCommand(configRepo, nil, nil, nil)

		// when
		err := cmd.Execute("/tmp/config.yaml", "/tmp/repo")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})

	t.Run("should show encryption enabled when identity file exists", func(t *testing.T) {
		t.Parallel()
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
		cmd := commands.NewStatusCommand(configRepo, stateRepo, manifestRepo, newStatusSyncer(manifestRepo))

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir)

		// then
		require.NoError(t, err)
	})

	t.Run("should show tool status with managed files when manifest exists", func(t *testing.T) {
		t.Parallel()
		// given
		tmpDir := t.TempDir()
		toolDir := filepath.Join(tmpDir, "claude")
		require.NoError(t, os.MkdirAll(toolDir, 0700))

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
		cmd := commands.NewStatusCommand(configRepo, stateRepo, manifestRepo, newStatusSyncer(manifestRepo))

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir)

		// then
		require.NoError(t, err)
		assert.GreaterOrEqual(t, manifestRepo.LoadCalls, 1)
	})

	t.Run("should show source freshness as stale when last pull is old", func(t *testing.T) {
		t.Parallel()
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
		cmd := commands.NewStatusCommand(configRepo, stateRepo, manifestRepo, newStatusSyncer(manifestRepo))

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir)

		// then
		require.NoError(t, err)
	})

	t.Run("should show never fetched when source has no etag", func(t *testing.T) {
		t.Parallel()
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
		manifestRepo := &doubles.MockManifestRepository{}
		cmd := commands.NewStatusCommand(configRepo, stateRepo, manifestRepo, newStatusSyncer(manifestRepo))

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir)

		// then
		require.NoError(t, err)
	})

	t.Run("should show incoming hint when push is newer than pull", func(t *testing.T) {
		t.Parallel()
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
		manifestRepo := &doubles.MockManifestRepository{}
		cmd := commands.NewStatusCommand(configRepo, stateRepo, manifestRepo, newStatusSyncer(manifestRepo))

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir)

		// then
		require.NoError(t, err)
	})
}

func TestStatusCommand_Execute_PendingChanges(t *testing.T) {
	t.Parallel()

	t.Run("should report nothing pending when an encrypted repo copy matches the file last synced", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.base("rules/go.md", "rule")
		f.writeLocal("rules/go.md", "rule")
		f.writeRepo("rules/go.md.age", "ciphertext")
		f.encryption.DecryptedData = []byte("rule")

		// when
		out := statusOutput(t, f)

		// then
		assert.Contains(t, out, "Pending Local Changes:\n  (none)\n")
		assert.NotContains(t, out, "not yet pushed")
	})

	t.Run("should not count a file .aisyncignore excludes", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		writeFile(t, filepath.Join(f.repoPath, ".aisyncignore"), "__pycache__/\n*.pyc\n")
		f.writeLocal("skills/demo/scripts/__pycache__/run.cpython-313.pyc", "bytecode")

		// when
		out := statusOutput(t, f)

		// then
		assert.Contains(t, out, "Pending Local Changes:\n  (none)\n")
	})

	t.Run("should not count shared files the manifest records", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.manifest.Manifest = entities.NewManifest("0.1.0", "host")
		f.manifest.ExistsVal = true
		f.manifest.Manifest.SetFile("rules/shared.md", "guide", "shared", "checksum")
		f.writeLocal("rules/shared.md", "shared rule")

		// when
		out := statusOutput(t, f)

		// then
		assert.Contains(t, out, "Pending Local Changes:\n  (none)\n")
	})

	t.Run("should count new, edited, and deleted local files as not yet pushed", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.base("rules/edited.md", "before")
		f.writeRepo("rules/edited.md", "before")
		f.writeLocal("rules/edited.md", "after")
		f.base("rules/deleted.md", "gone")
		f.writeRepo("rules/deleted.md", "gone")
		f.writeLocal("rules/new.md", "new")

		// when
		out := statusOutput(t, f)

		// then
		assert.Contains(t, out, "Pending Local Changes:\n  claude       3 file(s) not yet pushed\n")
	})

	t.Run("should report a file changed only in the sync repo as incoming", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.base("rules/go.md", "before")
		f.writeLocal("rules/go.md", "before")
		f.writeRepo("rules/go.md", "after")

		// when
		out := statusOutput(t, f)

		// then
		assert.Contains(t, out, "Pending Local Changes:\n  (none)\n")
		assert.Contains(t, out, "Incoming Changes:\n"+
			"  claude       1 file(s) in the sync repo not yet applied here\n"+
			"  (run 'aisync pull' to apply them)\n")
	})

	t.Run("should report conflicts beside the files not yet pushed", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.base("rules/both.md", "before")
		f.writeLocal("rules/both.md", "mine")
		f.writeRepo("rules/both.md", "theirs")
		f.writeLocal("rules/new.md", "new")

		// when
		out := statusOutput(t, f)

		// then
		assert.Contains(t, out, "  claude       1 file(s) not yet pushed, 1 conflict(s) to resolve\n")
	})
}

// newStatusSyncer builds the PersonalSyncer a StatusCommand plans with.
func newStatusSyncer(manifestRepo *doubles.MockManifestRepository) *commands.PersonalSyncer {
	return commands.NewPersonalSyncer(
		manifestRepo,
		&doubles.MockEncryptionService{},
		&doubles.MockApplyService{},
		&doubles.MockPromptService{},
		&doubles.MockTextDiffer{},
		&doubles.MockRepoHistory{},
	)
}

// statusOutput runs `aisync status` against the fixture's tool directory and
// sync repo and returns what it printed.
func statusOutput(t *testing.T, f *syncFixture) string {
	t.Helper()
	var out bytes.Buffer
	cmd := commands.NewStatusCommand(
		&doubles.MockConfigRepository{Config: f.config},
		&doubles.MockStateRepository{},
		f.manifest,
		f.syncer(),
	)
	commands.SetStatusOutput(cmd, &out)
	require.NoError(t, cmd.Execute("config.yaml", f.repoPath))
	return out.String()
}

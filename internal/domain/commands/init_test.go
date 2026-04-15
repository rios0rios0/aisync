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

func TestInitCommand_Execute(t *testing.T) {
	t.Run("should create dirs, save config, and save state when no clone URL", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "aifiles")

		configRepo := &doubles.MockConfigRepository{
			ExistsVal: false,
		}
		stateRepo := &doubles.MockStateRepository{}
		toolDetector := &doubles.MockToolDetector{
			DetectedTools: map[string]entities.Tool{
				"claude": {Path: "~/.claude", Enabled: true},
			},
		}
		gitRepo := &doubles.MockGitRepository{}

		// Create a fake identity file so the command takes the "reuse existing
		// identity" branch of ensureAgeKeyAndRecipient.
		identityDir := filepath.Join(tmpDir, ".config", "aisync")
		require.NoError(t, os.MkdirAll(identityDir, 0700))
		identityPath := filepath.Join(identityDir, "key.txt")
		require.NoError(t, os.WriteFile(identityPath, []byte("AGE-SECRET-KEY-FAKE"), 0600))

		// Override HOME so ExpandHome("~/.config/aisync/key.txt") resolves to our temp dir
		origHome := os.Getenv("HOME")
		t.Setenv("HOME", tmpDir)
		defer func() { _ = os.Setenv("HOME", origHome) }()

		encryptionService := &doubles.MockEncryptionService{
			// ExportedPublicKey is the value ExportPublicKey returns when the
			// command re-registers the existing identity as a recipient.
			ExportedPublicKey: "age1derivedfromexistingkey",
		}
		cmd := commands.NewInitCommand(configRepo, stateRepo, toolDetector, gitRepo, encryptionService)

		// when
		err := cmd.Execute(repoPath, "", "", "")

		// then
		require.NoError(t, err)
		// Config is saved exactly once, AFTER ensureAgeKeyAndRecipient has
		// populated the recipient list. A second Save would reopen the
		// interrupt window where the repo could land with `recipients: []`
		// on disk and silently push plaintext secrets.
		assert.Equal(t, 1, configRepo.SaveCalls)
		assert.Equal(t, 1, stateRepo.SaveCalls)
		assert.Equal(t, 1, gitRepo.InitCalls)
		assert.Equal(t, repoPath, gitRepo.InitDir)
		assert.Equal(t, 1, toolDetector.DetectCalls)
		// ExportPublicKey must have been called against the existing identity.
		assert.Equal(t, 1, encryptionService.ExportCalls)
		assert.Equal(t, identityPath, encryptionService.ExportIdentityPath)
		// The derived public key must end up in config.Encryption.Recipients
		// (checked on the last Save-d config snapshot held by the mock).
		require.NotNil(t, configRepo.SavedConfig)
		assert.Contains(t, configRepo.SavedConfig.Encryption.Recipients, "age1derivedfromexistingkey")

		// Verify only the minimal root directories were created. Tool
		// subdirectories must NOT be pre-created — they emerge as push/pull
		// discovers tools actually installed on the device.
		for _, dir := range []string{"personal", "shared", ".aisync"} {
			info, statErr := os.Stat(filepath.Join(repoPath, dir))
			require.NoError(t, statErr, "%s should exist", dir)
			assert.True(t, info.IsDir(), "%s should be a directory", dir)
		}
		_, statErr := os.Stat(filepath.Join(repoPath, "shared", "claude"))
		assert.True(t, os.IsNotExist(statErr), "shared/claude must NOT be pre-created")
		_, statErr = os.Stat(filepath.Join(repoPath, "personal", "claude"))
		assert.True(t, os.IsNotExist(statErr), "personal/claude must NOT be pre-created")
	})

	t.Run("should call gitRepo.Clone with correct URL when github user is provided", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "aifiles")

		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity: "/tmp/nonexistent-key.txt",
				},
			},
		}
		stateRepo := &doubles.MockStateRepository{}
		toolDetector := &doubles.MockToolDetector{}
		gitRepo := &doubles.MockGitRepository{}
		encryptionService := &doubles.MockEncryptionService{}
		cmd := commands.NewInitCommand(configRepo, stateRepo, toolDetector, gitRepo, encryptionService)

		// when
		err := cmd.Execute(repoPath, "testuser", "", "")

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, gitRepo.CloneCalls)
		assert.Equal(t, "git@github.com:testuser/aifiles.git", gitRepo.CloneURL)
		assert.Equal(t, repoPath, gitRepo.CloneDir)
		assert.Equal(t, "main", gitRepo.CloneBranch)
	})

	t.Run("should call gitRepo.Clone with remote URL when remoteURL is provided", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "aifiles")

		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity: "/tmp/nonexistent-key.txt",
				},
			},
		}
		stateRepo := &doubles.MockStateRepository{}
		toolDetector := &doubles.MockToolDetector{}
		gitRepo := &doubles.MockGitRepository{}
		encryptionService := &doubles.MockEncryptionService{}
		cmd := commands.NewInitCommand(configRepo, stateRepo, toolDetector, gitRepo, encryptionService)

		// when
		err := cmd.Execute(repoPath, "", "https://github.com/user/aifiles.git", "")

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, gitRepo.CloneCalls)
		assert.Equal(t, "https://github.com/user/aifiles.git", gitRepo.CloneURL)
	})

	t.Run("should return error when config already exists for create mode", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "aifiles")

		configRepo := &doubles.MockConfigRepository{
			ExistsVal: true,
		}
		stateRepo := &doubles.MockStateRepository{}
		toolDetector := &doubles.MockToolDetector{}
		gitRepo := &doubles.MockGitRepository{}
		encryptionService := &doubles.MockEncryptionService{}
		cmd := commands.NewInitCommand(configRepo, stateRepo, toolDetector, gitRepo, encryptionService)

		// when
		err := cmd.Execute(repoPath, "", "", "")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("should return error when clone fails", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "aifiles")

		configRepo := &doubles.MockConfigRepository{}
		stateRepo := &doubles.MockStateRepository{}
		toolDetector := &doubles.MockToolDetector{}
		gitRepo := &doubles.MockGitRepository{
			CloneErr: assert.AnError,
		}
		encryptionService := &doubles.MockEncryptionService{}
		cmd := commands.NewInitCommand(configRepo, stateRepo, toolDetector, gitRepo, encryptionService)

		// when
		err := cmd.Execute(repoPath, "testuser", "", "")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to clone repository")
	})

	t.Run("should import key when keyPath is provided during clone", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "aifiles")

		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity: filepath.Join(tmpDir, "identity", "key.txt"),
				},
			},
		}
		stateRepo := &doubles.MockStateRepository{}
		toolDetector := &doubles.MockToolDetector{}
		gitRepo := &doubles.MockGitRepository{}
		encryptionService := &doubles.MockEncryptionService{}
		cmd := commands.NewInitCommand(configRepo, stateRepo, toolDetector, gitRepo, encryptionService)

		// Create a source key file to import from
		sourceKeyPath := filepath.Join(tmpDir, "source-key.txt")
		require.NoError(t, os.WriteFile(sourceKeyPath, []byte("AGE-SECRET-KEY-TEST"), 0600))

		// when
		err := cmd.Execute(repoPath, "testuser", "", sourceKeyPath)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, gitRepo.CloneCalls)
		assert.Equal(t, 1, encryptionService.ImportCalls)
		assert.Equal(t, sourceKeyPath, encryptionService.ImportSourcePath)
	})

	t.Run("should auto-generate age key when identity file does not exist on create", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "aifiles")

		// Override HOME so ExpandHome resolves to temp dir
		t.Setenv("HOME", tmpDir)

		configRepo := &doubles.MockConfigRepository{
			ExistsVal: false,
		}
		stateRepo := &doubles.MockStateRepository{}
		toolDetector := &doubles.MockToolDetector{
			DetectedTools: map[string]entities.Tool{
				"claude": {Path: "~/.claude", Enabled: true},
			},
		}
		gitRepo := &doubles.MockGitRepository{}
		encryptionService := &doubles.MockEncryptionService{
			GeneratedPublicKey: "age1generatedkey123",
		}
		cmd := commands.NewInitCommand(configRepo, stateRepo, toolDetector, gitRepo, encryptionService)

		// when
		err := cmd.Execute(repoPath, "", "", "")

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, encryptionService.GenerateCalls)
		assert.Equal(t, 1, toolDetector.DetectCalls)
		// Config should be saved exactly once, with the new recipient already
		// populated — two separate writes would reopen the interrupt window
		// that previously left repos with `recipients: []` on disk.
		assert.Equal(t, 1, configRepo.SaveCalls)
	})

	t.Run("should return error when import key fails during clone", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "aifiles")

		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity: filepath.Join(tmpDir, "identity", "key.txt"),
				},
			},
		}
		stateRepo := &doubles.MockStateRepository{}
		gitRepo := &doubles.MockGitRepository{}
		encryptionService := &doubles.MockEncryptionService{
			ImportErr: assert.AnError,
		}
		cmd := commands.NewInitCommand(configRepo, stateRepo, &doubles.MockToolDetector{}, gitRepo, encryptionService)

		// when
		err := cmd.Execute(repoPath, "testuser", "", "/tmp/key.txt")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to import age identity")
	})

	t.Run("should return error when generate key fails on create", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "aifiles")

		t.Setenv("HOME", tmpDir)

		configRepo := &doubles.MockConfigRepository{
			ExistsVal: false,
		}
		stateRepo := &doubles.MockStateRepository{}
		toolDetector := &doubles.MockToolDetector{
			DetectedTools: map[string]entities.Tool{},
		}
		gitRepo := &doubles.MockGitRepository{}
		encryptionService := &doubles.MockEncryptionService{
			GenerateErr: assert.AnError,
		}
		cmd := commands.NewInitCommand(configRepo, stateRepo, toolDetector, gitRepo, encryptionService)

		// when
		err := cmd.Execute(repoPath, "", "", "")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to generate age key")
	})

	t.Run("should return error when git init fails on create", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "aifiles")

		// Create a fake identity file so the command does not try to generate a key
		identityDir := filepath.Join(tmpDir, ".config", "aisync")
		require.NoError(t, os.MkdirAll(identityDir, 0700))
		identityPath := filepath.Join(identityDir, "key.txt")
		require.NoError(t, os.WriteFile(identityPath, []byte("AGE-SECRET-KEY-FAKE"), 0600))

		t.Setenv("HOME", tmpDir)

		configRepo := &doubles.MockConfigRepository{ExistsVal: false}
		stateRepo := &doubles.MockStateRepository{}
		toolDetector := &doubles.MockToolDetector{
			DetectedTools: map[string]entities.Tool{},
		}
		gitRepo := &doubles.MockGitRepository{
			InitErr: assert.AnError,
		}
		encryptionService := &doubles.MockEncryptionService{}
		cmd := commands.NewInitCommand(configRepo, stateRepo, toolDetector, gitRepo, encryptionService)

		// when
		err := cmd.Execute(repoPath, "", "", "")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to initialize git repository")
	})

	t.Run("should write default .aisyncignore and .aisyncencrypt on create", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "aifiles")
		t.Setenv("HOME", tmpDir)

		configRepo := &doubles.MockConfigRepository{ExistsVal: false}
		stateRepo := &doubles.MockStateRepository{}
		toolDetector := &doubles.MockToolDetector{
			DetectedTools: map[string]entities.Tool{
				"claude": {Path: "~/.claude", Enabled: true},
			},
		}
		gitRepo := &doubles.MockGitRepository{}
		encryptionService := &doubles.MockEncryptionService{
			GeneratedPublicKey: "age1testkey",
		}
		cmd := commands.NewInitCommand(configRepo, stateRepo, toolDetector, gitRepo, encryptionService)

		// when
		err := cmd.Execute(repoPath, "", "", "")

		// then
		require.NoError(t, err)

		ignoreContent, readErr := os.ReadFile(filepath.Join(repoPath, ".aisyncignore"))
		require.NoError(t, readErr)
		assert.Contains(t, string(ignoreContent), "plans/", "default .aisyncignore should include plans/")
		assert.Contains(t, string(ignoreContent), "*.tmp")

		encryptContent, readErr := os.ReadFile(filepath.Join(repoPath, ".aisyncencrypt"))
		require.NoError(t, readErr)
		// Default patterns are tool-agnostic wildcards under personal/**/...
		// so the same defaults cover Claude, Cursor, Codex, and any future tool.
		assert.Contains(t, string(encryptContent), "personal/**/memories/**")
		assert.Contains(t, string(encryptContent), "personal/**/settings.local.json")
		// Spot-check a few critical new categories.
		assert.Contains(t, string(encryptContent), "personal/**/*.key", "private keys should be in default encrypt list")
		assert.Contains(t, string(encryptContent), "personal/**/id_ed25519", "SSH private keys should be in default encrypt list")
		assert.Contains(t, string(encryptContent), "personal/**/.netrc", ".netrc should be in default encrypt list")
		assert.Contains(t, string(encryptContent), "personal/**/auth.json", "auth.json should be in default encrypt list")
	})

	t.Run("should not overwrite existing .aisyncignore and .aisyncencrypt on clone", func(t *testing.T) {
		// given — simulate a clone that lands a repo with custom ignore/encrypt files.
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "aifiles")
		require.NoError(t, os.MkdirAll(repoPath, 0700))

		customIgnore := "# user-customised\nfoo/\n"
		customEncrypt := "# user-customised\npersonal/custom/**\n"
		require.NoError(t, os.WriteFile(filepath.Join(repoPath, ".aisyncignore"), []byte(customIgnore), 0600))
		require.NoError(t, os.WriteFile(filepath.Join(repoPath, ".aisyncencrypt"), []byte(customEncrypt), 0600))

		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{Identity: "/tmp/nonexistent-key.txt"},
			},
		}
		stateRepo := &doubles.MockStateRepository{}
		toolDetector := &doubles.MockToolDetector{}
		gitRepo := &doubles.MockGitRepository{}
		encryptionService := &doubles.MockEncryptionService{}
		cmd := commands.NewInitCommand(configRepo, stateRepo, toolDetector, gitRepo, encryptionService)

		// when
		err := cmd.Execute(repoPath, "testuser", "", "")

		// then
		require.NoError(t, err)

		ignoreContent, readErr := os.ReadFile(filepath.Join(repoPath, ".aisyncignore"))
		require.NoError(t, readErr)
		assert.Equal(t, customIgnore, string(ignoreContent), "existing .aisyncignore must not be overwritten")

		encryptContent, readErr := os.ReadFile(filepath.Join(repoPath, ".aisyncencrypt"))
		require.NoError(t, readErr)
		assert.Equal(t, customEncrypt, string(encryptContent), "existing .aisyncencrypt must not be overwritten")
	})

	t.Run("should write default .aisyncignore and .aisyncencrypt when missing after clone", func(t *testing.T) {
		// given — legacy cloned repo with no ignore/encrypt files yet.
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "aifiles")
		require.NoError(t, os.MkdirAll(repoPath, 0700))

		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{Identity: "/tmp/nonexistent-key.txt"},
			},
		}
		stateRepo := &doubles.MockStateRepository{}
		toolDetector := &doubles.MockToolDetector{}
		gitRepo := &doubles.MockGitRepository{}
		encryptionService := &doubles.MockEncryptionService{}
		cmd := commands.NewInitCommand(configRepo, stateRepo, toolDetector, gitRepo, encryptionService)

		// when
		err := cmd.Execute(repoPath, "testuser", "", "")

		// then
		require.NoError(t, err)

		_, statErr := os.Stat(filepath.Join(repoPath, ".aisyncignore"))
		assert.NoError(t, statErr, "clone should backfill .aisyncignore when missing")

		_, statErr = os.Stat(filepath.Join(repoPath, ".aisyncencrypt"))
		assert.NoError(t, statErr, "clone should backfill .aisyncencrypt when missing")
	})

	t.Run("should include only enabled tools in fresh config on create", func(t *testing.T) {
		// given — detector returns a mix: two enabled (installed) and two
		// disabled (not installed). Fresh config must list only the enabled
		// ones so the file is not polluted with placeholders for tools the
		// user does not have.
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "aifiles")
		t.Setenv("HOME", tmpDir)

		configRepo := &doubles.MockConfigRepository{ExistsVal: false}
		stateRepo := &doubles.MockStateRepository{}
		toolDetector := &doubles.MockToolDetector{
			DetectedTools: map[string]entities.Tool{
				"claude": {Path: "~/.claude", Enabled: true},
				"cursor": {Path: "~/.cursor", Enabled: true},
				"kiro":   {Path: "~/.kiro", Enabled: false},
				"warp":   {Path: "~/.warp", Enabled: false},
			},
		}
		gitRepo := &doubles.MockGitRepository{}
		encryptionService := &doubles.MockEncryptionService{
			GeneratedPublicKey: "age1freshkey",
		}
		cmd := commands.NewInitCommand(configRepo, stateRepo, toolDetector, gitRepo, encryptionService)

		// when
		err := cmd.Execute(repoPath, "", "", "")

		// then
		require.NoError(t, err)
		require.NotNil(t, configRepo.SavedConfig)
		assert.Len(t, configRepo.SavedConfig.Tools, 2, "fresh config should contain only enabled tools")
		assert.Contains(t, configRepo.SavedConfig.Tools, "claude")
		assert.Contains(t, configRepo.SavedConfig.Tools, "cursor")
		assert.NotContains(t, configRepo.SavedConfig.Tools, "kiro", "disabled tool must not pollute fresh config")
		assert.NotContains(t, configRepo.SavedConfig.Tools, "warp", "disabled tool must not pollute fresh config")
	})

	t.Run("should prefer remoteURL over githubUser for clone URL", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "aifiles")

		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity: "/tmp/nonexistent-key.txt",
				},
			},
		}
		stateRepo := &doubles.MockStateRepository{}
		gitRepo := &doubles.MockGitRepository{}
		encryptionService := &doubles.MockEncryptionService{}
		cmd := commands.NewInitCommand(configRepo, stateRepo, &doubles.MockToolDetector{}, gitRepo, encryptionService)

		// when
		err := cmd.Execute(repoPath, "testuser", "https://custom.git/repo.git", "")

		// then
		require.NoError(t, err)
		assert.Equal(t, "https://custom.git/repo.git", gitRepo.CloneURL)
	})
}

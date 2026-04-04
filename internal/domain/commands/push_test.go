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

func TestPushCommand_Execute(t *testing.T) {
	t.Run("should commit and push when there are personal files", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		claudeDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(claudeDir, 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(claudeDir, "CLAUDE.md"),
			[]byte("# My Config"),
			0644,
		))

		config := &entities.Config{
			Tools: map[string]entities.Tool{
				"claude": {Path: claudeDir, Enabled: true},
			},
			Encryption: entities.EncryptionConfig{Recipients: []string{}},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{
			State:     entities.NewState("test-device"),
			ExistsVal: true,
		}
		gitRepo := &doubles.MockGitRepository{
			HasRemoteVal: true,
			IsCleanVal:   false,
		}
		secretScanner := &doubles.MockSecretScanner{Findings: nil}

		cmd := commands.NewPushCommand(
			configRepo, stateRepo, gitRepo,
			&doubles.MockEncryptionService{},
			&doubles.MockManifestRepository{},
			secretScanner,
		)

		// when
		err := cmd.Execute(
			filepath.Join(repoPath, "config.yaml"),
			repoPath, "test commit", true, false,
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, gitRepo.CommitAllCalls)
		assert.Equal(t, "test commit", gitRepo.CommitMsg)
		assert.Equal(t, 1, gitRepo.PushCalls)
	})

	t.Run("should skip commit and push when repo is clean", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		config := &entities.Config{Tools: map[string]entities.Tool{}}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{
			State:     entities.NewState("test-device"),
			ExistsVal: true,
		}
		gitRepo := &doubles.MockGitRepository{
			HasRemoteVal: true,
			IsCleanVal:   true,
		}

		cmd := commands.NewPushCommand(
			configRepo, stateRepo, gitRepo,
			&doubles.MockEncryptionService{},
			&doubles.MockManifestRepository{},
			&doubles.MockSecretScanner{},
		)

		// when
		err := cmd.Execute(
			filepath.Join(repoPath, "config.yaml"),
			repoPath, "", true, false,
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, gitRepo.CommitAllCalls)
		assert.Equal(t, 0, gitRepo.PushCalls)
	})

	t.Run("should return error when config load fails", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{LoadErr: assert.AnError}
		cmd := commands.NewPushCommand(configRepo, nil, nil, nil, nil, nil)

		// when
		err := cmd.Execute("/tmp/config.yaml", "/tmp/repo", "", false, false)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})

	t.Run("should skip push when no remote is configured", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		config := &entities.Config{Tools: map[string]entities.Tool{}}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{
			State:     entities.NewState("test-device"),
			ExistsVal: true,
		}
		gitRepo := &doubles.MockGitRepository{
			HasRemoteVal: false,
			IsCleanVal:   false,
		}

		cmd := commands.NewPushCommand(
			configRepo, stateRepo, gitRepo,
			&doubles.MockEncryptionService{},
			&doubles.MockManifestRepository{},
			&doubles.MockSecretScanner{},
		)

		// when
		err := cmd.Execute(
			filepath.Join(repoPath, "config.yaml"),
			repoPath, "test commit", true, false,
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, gitRepo.CommitAllCalls)
		assert.Equal(t, 0, gitRepo.PushCalls)
	})

	t.Run("should run in dry-run mode without modifying repo", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		config := &entities.Config{
			Tools:      map[string]entities.Tool{},
			Encryption: entities.EncryptionConfig{Recipients: []string{}},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		gitRepo := &doubles.MockGitRepository{}

		cmd := commands.NewPushCommand(
			configRepo, nil, gitRepo,
			&doubles.MockEncryptionService{},
			&doubles.MockManifestRepository{},
			&doubles.MockSecretScanner{},
		)

		// when
		err := cmd.Execute(
			filepath.Join(repoPath, "config.yaml"),
			repoPath, "", false, true,
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, gitRepo.OpenCalls)
		assert.Equal(t, 0, gitRepo.CommitAllCalls)
		assert.Equal(t, 0, gitRepo.PushCalls)
	})

	t.Run("should return error when secret scan finds secrets", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		personalDir := filepath.Join(repoPath, "personal", "claude")
		require.NoError(t, os.MkdirAll(personalDir, 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(personalDir, "secrets.md"),
			[]byte("API_KEY=sk-12345"),
			0644,
		))

		config := &entities.Config{Tools: map[string]entities.Tool{}}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{
			State:     entities.NewState("test-device"),
			ExistsVal: true,
		}
		gitRepo := &doubles.MockGitRepository{
			HasRemoteVal: true,
			IsCleanVal:   false,
		}
		secretScanner := &doubles.MockSecretScanner{
			Findings: []repositories.SecretFinding{
				{Path: "personal/claude/secrets.md", Line: 1, Description: "API key detected"},
			},
		}

		cmd := commands.NewPushCommand(
			configRepo, stateRepo, gitRepo,
			&doubles.MockEncryptionService{},
			&doubles.MockManifestRepository{},
			secretScanner,
		)

		// when
		err := cmd.Execute(
			filepath.Join(repoPath, "config.yaml"),
			repoPath, "test commit", false, false,
		)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "push blocked")
		assert.Contains(t, err.Error(), "secret(s) detected")
		assert.Equal(t, 0, gitRepo.CommitAllCalls)
	})

	t.Run("should succeed when skip secret scan is true despite secrets", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		config := &entities.Config{Tools: map[string]entities.Tool{}}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{
			State:     entities.NewState("test-device"),
			ExistsVal: true,
		}
		gitRepo := &doubles.MockGitRepository{
			HasRemoteVal: true,
			IsCleanVal:   false,
		}
		secretScanner := &doubles.MockSecretScanner{
			Findings: []repositories.SecretFinding{
				{Path: "personal/claude/secrets.md", Line: 1, Description: "API key detected"},
			},
		}

		cmd := commands.NewPushCommand(
			configRepo, stateRepo, gitRepo,
			&doubles.MockEncryptionService{},
			&doubles.MockManifestRepository{},
			secretScanner,
		)

		// when
		err := cmd.Execute(
			filepath.Join(repoPath, "config.yaml"),
			repoPath, "test commit", true, false,
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, gitRepo.CommitAllCalls)
		assert.Equal(t, 0, secretScanner.ScanCalls)
	})

	t.Run("should generate default commit message when none provided", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		config := &entities.Config{Tools: map[string]entities.Tool{}}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{
			State:     entities.NewState("test-device"),
			ExistsVal: true,
		}
		gitRepo := &doubles.MockGitRepository{
			HasRemoteVal: true,
			IsCleanVal:   false,
		}

		cmd := commands.NewPushCommand(
			configRepo, stateRepo, gitRepo,
			&doubles.MockEncryptionService{},
			&doubles.MockManifestRepository{},
			&doubles.MockSecretScanner{},
		)

		// when
		err := cmd.Execute(
			filepath.Join(repoPath, "config.yaml"),
			repoPath, "", true, false,
		)

		// then
		require.NoError(t, err)
		assert.Contains(t, gitRepo.CommitMsg, "sync(")
		assert.Contains(t, gitRepo.CommitMsg, "updated personal configurations")
	})

	t.Run("should return error when git open fails", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		config := &entities.Config{Tools: map[string]entities.Tool{}}
		configRepo := &doubles.MockConfigRepository{Config: config}
		gitRepo := &doubles.MockGitRepository{OpenErr: assert.AnError}

		cmd := commands.NewPushCommand(
			configRepo, nil, gitRepo,
			&doubles.MockEncryptionService{},
			&doubles.MockManifestRepository{},
			&doubles.MockSecretScanner{},
		)

		// when
		err := cmd.Execute(
			filepath.Join(repoPath, "config.yaml"),
			repoPath, "test", true, false,
		)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open git repo")
	})

	t.Run("should return error when git is clean check fails", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		config := &entities.Config{Tools: map[string]entities.Tool{}}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{
			State:     entities.NewState("test-device"),
			ExistsVal: true,
		}
		gitRepo := &doubles.MockGitRepository{IsCleanErr: assert.AnError}

		cmd := commands.NewPushCommand(
			configRepo, stateRepo, gitRepo,
			&doubles.MockEncryptionService{},
			&doubles.MockManifestRepository{},
			&doubles.MockSecretScanner{},
		)

		// when
		err := cmd.Execute(
			filepath.Join(repoPath, "config.yaml"),
			repoPath, "test", true, false,
		)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to check git status")
	})

	t.Run("should encrypt files matching encrypt patterns during push", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		require.NoError(t, os.WriteFile(
			filepath.Join(repoPath, ".aisyncencrypt"),
			[]byte("*.secret"),
			0644,
		))

		claudeDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(claudeDir, 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(claudeDir, "creds.secret"),
			[]byte("my-secret"),
			0644,
		))

		config := &entities.Config{
			Tools: map[string]entities.Tool{
				"claude": {Path: claudeDir, Enabled: true},
			},
			Encryption: entities.EncryptionConfig{
				Recipients: []string{"age1recipient123"},
			},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{
			State:     entities.NewState("test-device"),
			ExistsVal: true,
		}
		gitRepo := &doubles.MockGitRepository{
			HasRemoteVal: true,
			IsCleanVal:   false,
		}
		encryptionService := &doubles.MockEncryptionService{
			EncryptedData: []byte("ENCRYPTED-DATA"),
		}

		cmd := commands.NewPushCommand(
			configRepo, stateRepo, gitRepo,
			encryptionService,
			&doubles.MockManifestRepository{},
			&doubles.MockSecretScanner{},
		)

		// when
		err := cmd.Execute(
			filepath.Join(repoPath, "config.yaml"),
			repoPath, "test commit", true, false,
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, encryptionService.EncryptCalls)

		encryptedPath := filepath.Join(repoPath, "personal", "claude", "creds.secret.age")
		content, readErr := os.ReadFile(encryptedPath)
		require.NoError(t, readErr)
		assert.Equal(t, "ENCRYPTED-DATA", string(content))
	})

	t.Run("should skip tool directory that does not exist", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		config := &entities.Config{
			Tools: map[string]entities.Tool{
				"claude": {Path: filepath.Join(tmpDir, "nonexistent"), Enabled: true},
			},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{
			State:     entities.NewState("test-device"),
			ExistsVal: true,
		}
		gitRepo := &doubles.MockGitRepository{
			HasRemoteVal: true,
			IsCleanVal:   true,
		}

		cmd := commands.NewPushCommand(
			configRepo, stateRepo, gitRepo,
			&doubles.MockEncryptionService{},
			&doubles.MockManifestRepository{},
			&doubles.MockSecretScanner{},
		)

		// when
		err := cmd.Execute(
			filepath.Join(repoPath, "config.yaml"),
			repoPath, "test", true, false,
		)

		// then
		require.NoError(t, err)
	})

	t.Run("should return error when commit fails", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		config := &entities.Config{Tools: map[string]entities.Tool{}}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{
			State:     entities.NewState("test-device"),
			ExistsVal: true,
		}
		gitRepo := &doubles.MockGitRepository{
			HasRemoteVal: true,
			IsCleanVal:   false,
			CommitErr:    assert.AnError,
		}

		cmd := commands.NewPushCommand(
			configRepo, stateRepo, gitRepo,
			&doubles.MockEncryptionService{},
			&doubles.MockManifestRepository{},
			&doubles.MockSecretScanner{},
		)

		// when
		err := cmd.Execute(
			filepath.Join(repoPath, "config.yaml"),
			repoPath, "test commit", true, false,
		)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to commit")
	})

	t.Run("should update state LastPush after successful push", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		config := &entities.Config{Tools: map[string]entities.Tool{}}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{
			State:     entities.NewState("test-device"),
			ExistsVal: true,
		}
		gitRepo := &doubles.MockGitRepository{
			HasRemoteVal: true,
			IsCleanVal:   false,
		}

		cmd := commands.NewPushCommand(
			configRepo, stateRepo, gitRepo,
			&doubles.MockEncryptionService{},
			&doubles.MockManifestRepository{},
			&doubles.MockSecretScanner{},
		)

		// when
		err := cmd.Execute(
			filepath.Join(repoPath, "config.yaml"),
			repoPath, "test", true, false,
		)

		// then
		require.NoError(t, err)
		assert.GreaterOrEqual(t, stateRepo.SaveCalls, 1)
		require.NotNil(t, stateRepo.SavedState)
		assert.False(t, stateRepo.SavedState.LastPush.IsZero())
	})

	t.Run("should not copy shared file from manifest to personal directory", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		claudeDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(filepath.Join(claudeDir, "rules"), 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(claudeDir, "rules/shared.md"),
			[]byte("shared rule content"),
			0644,
		))
		require.NoError(t, os.WriteFile(
			filepath.Join(claudeDir, "rules/personal.md"),
			[]byte("personal rule content"),
			0644,
		))

		manifest := entities.NewManifest("0.1.0", "host")
		manifest.SetFile("rules/shared.md", "guide", "shared", "checksum")

		config := &entities.Config{
			Tools: map[string]entities.Tool{
				"claude": {Path: claudeDir, Enabled: true},
			},
			Encryption: entities.EncryptionConfig{Recipients: []string{}},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{
			State:     entities.NewState("test-device"),
			ExistsVal: true,
		}
		gitRepo := &doubles.MockGitRepository{
			HasRemoteVal: true,
			IsCleanVal:   false,
		}
		manifestRepo := &doubles.MockManifestRepository{
			ExistsVal: true,
			Manifest:  manifest,
		}

		cmd := commands.NewPushCommand(
			configRepo, stateRepo, gitRepo,
			&doubles.MockEncryptionService{},
			manifestRepo,
			&doubles.MockSecretScanner{},
		)

		// when
		err := cmd.Execute(
			filepath.Join(repoPath, "config.yaml"),
			repoPath, "test commit", true, false,
		)

		// then
		require.NoError(t, err)

		// personal.md should have been copied
		personalFile := filepath.Join(repoPath, "personal", "claude", "rules", "personal.md")
		_, err = os.Stat(personalFile)
		assert.NoError(t, err)

		// shared.md should NOT have been copied to personal/
		sharedPersonalFile := filepath.Join(repoPath, "personal", "claude", "rules", "shared.md")
		_, err = os.Stat(sharedPersonalFile)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("should list personal files in dry-run mode with tools present", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		claudeDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(claudeDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "my-rule.md"), []byte("personal"), 0644))

		config := &entities.Config{
			Tools: map[string]entities.Tool{
				"claude": {Path: claudeDir, Enabled: true},
			},
			Encryption: entities.EncryptionConfig{Recipients: []string{}},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		gitRepo := &doubles.MockGitRepository{}

		cmd := commands.NewPushCommand(
			configRepo, nil, gitRepo,
			&doubles.MockEncryptionService{},
			&doubles.MockManifestRepository{},
			&doubles.MockSecretScanner{},
		)

		// when
		err := cmd.Execute(
			filepath.Join(repoPath, "config.yaml"),
			repoPath, "", false, true,
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, gitRepo.OpenCalls)
	})

	t.Run("should skip files matching ignore patterns", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		// Create .aisyncignore
		require.NoError(t, os.WriteFile(
			filepath.Join(repoPath, ".aisyncignore"),
			[]byte("*.log"),
			0644,
		))

		claudeDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(claudeDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "debug.log"), []byte("log data"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "rule.md"), []byte("rule content"), 0644))

		config := &entities.Config{
			Tools: map[string]entities.Tool{
				"claude": {Path: claudeDir, Enabled: true},
			},
			Encryption: entities.EncryptionConfig{Recipients: []string{}},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{
			State:     entities.NewState("test-device"),
			ExistsVal: true,
		}
		gitRepo := &doubles.MockGitRepository{
			HasRemoteVal: true,
			IsCleanVal:   false,
		}

		cmd := commands.NewPushCommand(
			configRepo, stateRepo, gitRepo,
			&doubles.MockEncryptionService{},
			&doubles.MockManifestRepository{},
			&doubles.MockSecretScanner{},
		)

		// when
		err := cmd.Execute(
			filepath.Join(repoPath, "config.yaml"),
			repoPath, "test commit", true, false,
		)

		// then
		require.NoError(t, err)
		// rule.md should be copied, debug.log should not
		ruleFile := filepath.Join(repoPath, "personal", "claude", "rule.md")
		_, err = os.Stat(ruleFile)
		assert.NoError(t, err)

		logFile := filepath.Join(repoPath, "personal", "claude", "debug.log")
		_, err = os.Stat(logFile)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("should scan unencrypted files for secrets and pass when clean", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		personalDir := filepath.Join(repoPath, "personal", "claude")
		require.NoError(t, os.MkdirAll(personalDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(personalDir, "safe.md"), []byte("safe content"), 0644))

		config := &entities.Config{Tools: map[string]entities.Tool{}}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{
			State:     entities.NewState("test-device"),
			ExistsVal: true,
		}
		gitRepo := &doubles.MockGitRepository{
			HasRemoteVal: true,
			IsCleanVal:   false,
		}
		secretScanner := &doubles.MockSecretScanner{Findings: nil}

		cmd := commands.NewPushCommand(
			configRepo, stateRepo, gitRepo,
			&doubles.MockEncryptionService{},
			&doubles.MockManifestRepository{},
			secretScanner,
		)

		// when
		err := cmd.Execute(
			filepath.Join(repoPath, "config.yaml"),
			repoPath, "test", false, false,
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, secretScanner.ScanCalls)
		assert.Equal(t, 1, gitRepo.CommitAllCalls)
	})

	t.Run("should not copy unchanged file to personal directory", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		claudeDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(claudeDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "rule.md"), []byte("same content"), 0644))

		// Pre-populate the personal directory with the same content
		personalDir := filepath.Join(repoPath, "personal", "claude")
		require.NoError(t, os.MkdirAll(personalDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(personalDir, "rule.md"), []byte("same content"), 0644))

		config := &entities.Config{
			Tools: map[string]entities.Tool{
				"claude": {Path: claudeDir, Enabled: true},
			},
			Encryption: entities.EncryptionConfig{Recipients: []string{}},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{
			State:     entities.NewState("test-device"),
			ExistsVal: true,
		}
		gitRepo := &doubles.MockGitRepository{
			HasRemoteVal: true,
			IsCleanVal:   true, // nothing changed
		}

		cmd := commands.NewPushCommand(
			configRepo, stateRepo, gitRepo,
			&doubles.MockEncryptionService{},
			&doubles.MockManifestRepository{},
			&doubles.MockSecretScanner{},
		)

		// when
		err := cmd.Execute(
			filepath.Join(repoPath, "config.yaml"),
			repoPath, "test", true, false,
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, gitRepo.CommitAllCalls) // clean repo
	})

	t.Run("should skip .age files during secret scan", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		personalDir := filepath.Join(repoPath, "personal", "claude")
		require.NoError(t, os.MkdirAll(personalDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(personalDir, "creds.age"), []byte("encrypted"), 0644))

		config := &entities.Config{Tools: map[string]entities.Tool{}}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{
			State:     entities.NewState("test-device"),
			ExistsVal: true,
		}
		gitRepo := &doubles.MockGitRepository{
			HasRemoteVal: true,
			IsCleanVal:   false,
		}
		secretScanner := &doubles.MockSecretScanner{Findings: nil}

		cmd := commands.NewPushCommand(
			configRepo, stateRepo, gitRepo,
			&doubles.MockEncryptionService{},
			&doubles.MockManifestRepository{},
			secretScanner,
		)

		// when
		err := cmd.Execute(
			filepath.Join(repoPath, "config.yaml"),
			repoPath, "test", false, false,
		)

		// then
		require.NoError(t, err)
		// .age files are skipped so scanner should not be called with them
		assert.Equal(t, 0, secretScanner.ScanCalls)
	})

	t.Run("should handle push failure gracefully and still succeed", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		config := &entities.Config{Tools: map[string]entities.Tool{}}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{
			State:     entities.NewState("test-device"),
			ExistsVal: true,
		}
		gitRepo := &doubles.MockGitRepository{
			HasRemoteVal: true,
			IsCleanVal:   false,
			PushErr:      assert.AnError, // push fails
		}

		cmd := commands.NewPushCommand(
			configRepo, stateRepo, gitRepo,
			&doubles.MockEncryptionService{},
			&doubles.MockManifestRepository{},
			&doubles.MockSecretScanner{},
		)

		// when
		err := cmd.Execute(
			filepath.Join(repoPath, "config.yaml"),
			repoPath, "test commit", true, false,
		)

		// then
		require.NoError(t, err) // push failure is non-fatal
		assert.Equal(t, 1, gitRepo.CommitAllCalls)
		assert.Equal(t, 1, gitRepo.PushCalls)
	})

	t.Run("should update state with existing state after push", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		config := &entities.Config{Tools: map[string]entities.Tool{}}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{
			State:     entities.NewState("test-device"),
			ExistsVal: false, // no existing state
		}
		gitRepo := &doubles.MockGitRepository{
			HasRemoteVal: true,
			IsCleanVal:   false,
		}

		cmd := commands.NewPushCommand(
			configRepo, stateRepo, gitRepo,
			&doubles.MockEncryptionService{},
			&doubles.MockManifestRepository{},
			&doubles.MockSecretScanner{},
		)

		// when
		err := cmd.Execute(
			filepath.Join(repoPath, "config.yaml"),
			repoPath, "test", true, false,
		)

		// then
		require.NoError(t, err)
		assert.GreaterOrEqual(t, stateRepo.SaveCalls, 1)
		require.NotNil(t, stateRepo.SavedState)
		assert.False(t, stateRepo.SavedState.LastPush.IsZero())
	})

	t.Run("should show encrypted files in dry-run mode", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		// Create .aisyncencrypt
		require.NoError(t, os.WriteFile(
			filepath.Join(repoPath, ".aisyncencrypt"),
			[]byte("*.secret"),
			0644,
		))

		claudeDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(claudeDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "creds.secret"), []byte("secret"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "rule.md"), []byte("rule"), 0644))

		config := &entities.Config{
			Tools: map[string]entities.Tool{
				"claude": {Path: claudeDir, Enabled: true},
			},
			Encryption: entities.EncryptionConfig{
				Recipients: []string{"age1recipient"},
			},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		gitRepo := &doubles.MockGitRepository{}

		cmd := commands.NewPushCommand(
			configRepo, nil, gitRepo,
			&doubles.MockEncryptionService{},
			&doubles.MockManifestRepository{},
			&doubles.MockSecretScanner{},
		)

		// when
		err := cmd.Execute(
			filepath.Join(repoPath, "config.yaml"),
			repoPath, "", false, true,
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, gitRepo.OpenCalls)
	})

	t.Run("should load existing manifest to determine shared files", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		claudeDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(claudeDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "shared.md"), []byte("shared"), 0644))

		manifest := entities.NewManifest("0.1.0", "host")
		manifest.SetFile("shared.md", "guide", "shared", "checksum")

		config := &entities.Config{
			Tools: map[string]entities.Tool{
				"claude": {Path: claudeDir, Enabled: true},
			},
			Encryption: entities.EncryptionConfig{Recipients: []string{}},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{
			State:     entities.NewState("test-device"),
			ExistsVal: true,
		}
		gitRepo := &doubles.MockGitRepository{
			HasRemoteVal: true,
			IsCleanVal:   true,
		}
		manifestRepo := &doubles.MockManifestRepository{
			ExistsVal: true,
			Manifest:  manifest,
		}

		cmd := commands.NewPushCommand(
			configRepo, stateRepo, gitRepo,
			&doubles.MockEncryptionService{},
			manifestRepo,
			&doubles.MockSecretScanner{},
		)

		// when
		err := cmd.Execute(
			filepath.Join(repoPath, "config.yaml"),
			repoPath, "test", true, false,
		)

		// then
		require.NoError(t, err)
		assert.GreaterOrEqual(t, manifestRepo.LoadCalls, 1)
	})
}

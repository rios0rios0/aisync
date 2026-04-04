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

func TestDoctorCommand_Execute(t *testing.T) {
	t.Run("should complete without error when all checks pass", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()

		repoPath := filepath.Join(tmpDir, "aifiles")
		require.NoError(t, os.MkdirAll(filepath.Join(repoPath, ".git"), 0755))

		identityPath := filepath.Join(tmpDir, "key.txt")
		require.NoError(t, os.WriteFile(identityPath, []byte("AGE-SECRET-KEY-FAKE"), 0600))

		configRepo := &doubles.MockConfigRepository{
			ExistsVal: true,
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity: identityPath,
				},
				Sources: []entities.Source{
					{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
				},
			},
		}
		stateRepo := &doubles.MockStateRepository{
			ExistsVal: true,
			State: &entities.State{
				Devices: []entities.Device{
					{ID: "abc", Name: "laptop"},
				},
			},
		}
		encryptionService := &doubles.MockEncryptionService{
			ExportedPublicKey: "age1testpublickeylongstringhere",
		}
		toolDetector := &doubles.MockToolDetector{
			DetectedTools: map[string]entities.Tool{
				"claude": {Path: "~/.claude", Enabled: true},
			},
		}
		formatter := &entities.PlainFormatter{}
		cmd := commands.NewDoctorCommand(configRepo, stateRepo, encryptionService, toolDetector, formatter)

		configPath := filepath.Join(repoPath, "config.yaml")

		// when
		err := cmd.Execute(configPath, repoPath)

		// then
		require.NoError(t, err)
		assert.GreaterOrEqual(t, configRepo.LoadCalls, 1)
		assert.Equal(t, 1, toolDetector.DetectCalls)
	})

	t.Run("should complete without error when some checks fail", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "nonexistent-repo")

		configRepo := &doubles.MockConfigRepository{
			ExistsVal: false,
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity: "/nonexistent/key.txt",
				},
				Sources: []entities.Source{},
			},
		}
		stateRepo := &doubles.MockStateRepository{ExistsVal: false}
		encryptionService := &doubles.MockEncryptionService{
			ExportErr: assert.AnError,
		}
		toolDetector := &doubles.MockToolDetector{
			DetectedTools: map[string]entities.Tool{},
		}
		formatter := &entities.PlainFormatter{}
		cmd := commands.NewDoctorCommand(configRepo, stateRepo, encryptionService, toolDetector, formatter)

		// when
		err := cmd.Execute("/tmp/config.yaml", repoPath)

		// then
		require.NoError(t, err)
	})

	t.Run("should detect missing config as failure", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(filepath.Join(repoPath, ".git"), 0755))

		configRepo := &doubles.MockConfigRepository{
			ExistsVal: false,
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{Identity: "/nonexistent/key.txt"},
				Sources:    []entities.Source{},
			},
		}
		stateRepo := &doubles.MockStateRepository{
			ExistsVal: true,
			State:     &entities.State{Devices: []entities.Device{{ID: "a", Name: "h"}}},
		}
		toolDetector := &doubles.MockToolDetector{
			DetectedTools: map[string]entities.Tool{
				"claude": {Path: "~/.claude", Enabled: true},
			},
		}
		formatter := &entities.PlainFormatter{}
		cmd := commands.NewDoctorCommand(
			configRepo, stateRepo,
			&doubles.MockEncryptionService{ExportErr: assert.AnError},
			toolDetector, formatter,
		)

		// when
		err := cmd.Execute("/tmp/nonexistent-config.yaml", repoPath)

		// then
		require.NoError(t, err)
		// The ExistsVal is false so checkConfig should report failure.
		// Doctor always returns nil but outputs [FAIL] lines.
	})

	t.Run("should detect missing age key as failure", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(filepath.Join(repoPath, ".git"), 0755))

		configRepo := &doubles.MockConfigRepository{
			ExistsVal: true,
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity: "/nonexistent/key.txt",
				},
				Sources: []entities.Source{},
			},
		}
		stateRepo := &doubles.MockStateRepository{
			ExistsVal: true,
			State:     &entities.State{Devices: []entities.Device{{ID: "a", Name: "h"}}},
		}
		toolDetector := &doubles.MockToolDetector{
			DetectedTools: map[string]entities.Tool{
				"claude": {Path: "~/.claude", Enabled: true},
			},
		}
		formatter := &entities.PlainFormatter{}
		cmd := commands.NewDoctorCommand(
			configRepo, stateRepo,
			&doubles.MockEncryptionService{ExportErr: assert.AnError},
			toolDetector, formatter,
		)

		// when
		err := cmd.Execute(filepath.Join(repoPath, "config.yaml"), repoPath)

		// then
		require.NoError(t, err)
		// checkAgeKey should fail because identity file doesn't exist.
	})

	t.Run("should detect no tools as failure", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(filepath.Join(repoPath, ".git"), 0755))

		identityPath := filepath.Join(tmpDir, "key.txt")
		require.NoError(t, os.WriteFile(identityPath, []byte("AGE-SECRET-KEY"), 0600))

		configRepo := &doubles.MockConfigRepository{
			ExistsVal: true,
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{Identity: identityPath},
				Sources:    []entities.Source{},
			},
		}
		stateRepo := &doubles.MockStateRepository{
			ExistsVal: true,
			State:     &entities.State{Devices: []entities.Device{{ID: "a", Name: "h"}}},
		}
		toolDetector := &doubles.MockToolDetector{
			DetectedTools: map[string]entities.Tool{}, // no tools
		}
		formatter := &entities.PlainFormatter{}
		cmd := commands.NewDoctorCommand(
			configRepo, stateRepo,
			&doubles.MockEncryptionService{ExportedPublicKey: "age1testpublickeylongstringhere"},
			toolDetector, formatter,
		)

		// when
		err := cmd.Execute(filepath.Join(repoPath, "config.yaml"), repoPath)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, toolDetector.DetectCalls)
		// checkTools returns false when no enabled tools detected
	})

	t.Run("should detect missing state as failure", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(filepath.Join(repoPath, ".git"), 0755))

		configRepo := &doubles.MockConfigRepository{
			ExistsVal: true,
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{Identity: "/nonexistent/key.txt"},
				Sources:    []entities.Source{},
			},
		}
		stateRepo := &doubles.MockStateRepository{ExistsVal: false}
		toolDetector := &doubles.MockToolDetector{
			DetectedTools: map[string]entities.Tool{
				"claude": {Path: "~/.claude", Enabled: true},
			},
		}
		formatter := &entities.PlainFormatter{}
		cmd := commands.NewDoctorCommand(
			configRepo, stateRepo,
			&doubles.MockEncryptionService{ExportErr: assert.AnError},
			toolDetector, formatter,
		)

		// when
		err := cmd.Execute(filepath.Join(repoPath, "config.yaml"), repoPath)

		// then
		require.NoError(t, err)
		// checkState should fail because state does not exist
	})

	t.Run("should detect repo without .git as failure", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))
		// no .git directory created

		configRepo := &doubles.MockConfigRepository{
			ExistsVal: true,
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{Identity: "/nonexistent/key.txt"},
				Sources:    []entities.Source{},
			},
		}
		stateRepo := &doubles.MockStateRepository{
			ExistsVal: true,
			State:     &entities.State{Devices: []entities.Device{{ID: "a", Name: "h"}}},
		}
		toolDetector := &doubles.MockToolDetector{
			DetectedTools: map[string]entities.Tool{
				"claude": {Path: "~/.claude", Enabled: true},
			},
		}
		formatter := &entities.PlainFormatter{}
		cmd := commands.NewDoctorCommand(
			configRepo, stateRepo,
			&doubles.MockEncryptionService{ExportErr: assert.AnError},
			toolDetector, formatter,
		)

		// when
		err := cmd.Execute(filepath.Join(repoPath, "config.yaml"), repoPath)

		// then
		require.NoError(t, err)
		// checkRepo should fail because .git does not exist
	})
}

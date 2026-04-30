//go:build unit

package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/domain/commands"
	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/test/doubles"
)

func TestKeyCommand_Generate(t *testing.T) {
	t.Run("should call GenerateKey and update config recipients", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity:   "/tmp/test-key.txt",
					Recipients: []string{},
				},
			},
		}
		encryptionService := &doubles.MockEncryptionService{
			GeneratedPublicKey: "age1testpublickey123",
		}
		cmd := commands.NewKeyCommand(configRepo, encryptionService, nil)

		// when
		err := cmd.Generate("/tmp/config.yaml")

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, encryptionService.GenerateCalls)
		assert.Equal(t, "/tmp/test-key.txt", encryptionService.GenerateOutputPath)
		assert.Equal(t, 1, configRepo.SaveCalls)
		require.NotNil(t, configRepo.SavedConfig)
		assert.Contains(t, configRepo.SavedConfig.Encryption.Recipients, "age1testpublickey123")
	})

	t.Run("should not duplicate recipient when key already exists", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity:   "/tmp/test-key.txt",
					Recipients: []string{"age1testpublickey123"},
				},
			},
		}
		encryptionService := &doubles.MockEncryptionService{
			GeneratedPublicKey: "age1testpublickey123",
		}
		cmd := commands.NewKeyCommand(configRepo, encryptionService, nil)

		// when
		err := cmd.Generate("/tmp/config.yaml")

		// then
		require.NoError(t, err)
		assert.Len(t, configRepo.SavedConfig.Encryption.Recipients, 1)
	})
}

func TestKeyCommand_Export(t *testing.T) {
	t.Run("should call ExportPublicKey and return no error", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity: "/tmp/test-key.txt",
				},
			},
		}
		encryptionService := &doubles.MockEncryptionService{
			ExportedPublicKey: "age1exportedkey456",
		}
		cmd := commands.NewKeyCommand(configRepo, encryptionService, nil)

		// when
		err := cmd.Export("/tmp/config.yaml")

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, encryptionService.ExportCalls)
		assert.Equal(t, "/tmp/test-key.txt", encryptionService.ExportIdentityPath)
	})

	t.Run("should return error when ExportPublicKey fails", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity: "/tmp/test-key.txt",
				},
			},
		}
		encryptionService := &doubles.MockEncryptionService{
			ExportErr: assert.AnError,
		}
		cmd := commands.NewKeyCommand(configRepo, encryptionService, nil)

		// when
		err := cmd.Export("/tmp/config.yaml")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to export public key")
	})
}

func TestKeyCommand_AddRecipient(t *testing.T) {
	t.Run("should append public key to config recipients", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Recipients: []string{"age1existing"},
				},
			},
		}
		cmd := commands.NewKeyCommand(configRepo, nil, nil)

		// when
		err := cmd.AddRecipient("/tmp/config.yaml", "age1newrecipient")

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, configRepo.SaveCalls)
		require.NotNil(t, configRepo.SavedConfig)
		assert.Contains(t, configRepo.SavedConfig.Encryption.Recipients, "age1existing")
		assert.Contains(t, configRepo.SavedConfig.Encryption.Recipients, "age1newrecipient")
	})

	t.Run("should not duplicate existing recipient", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Recipients: []string{"age1existing"},
				},
			},
		}
		cmd := commands.NewKeyCommand(configRepo, nil, nil)

		// when
		err := cmd.AddRecipient("/tmp/config.yaml", "age1existing")

		// then
		require.NoError(t, err)
		assert.Len(t, configRepo.SavedConfig.Encryption.Recipients, 1)
	})
}

func TestKeyCommand_Generate_ErrorCases(t *testing.T) {
	t.Run("should return error when config load fails", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{LoadErr: assert.AnError}
		cmd := commands.NewKeyCommand(configRepo, nil, nil)

		// when
		err := cmd.Generate("/tmp/config.yaml")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})

	t.Run("should return error when GenerateKey fails", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity: "/tmp/test-key.txt",
				},
			},
		}
		encryptionService := &doubles.MockEncryptionService{
			GenerateErr: assert.AnError,
		}
		cmd := commands.NewKeyCommand(configRepo, encryptionService, nil)

		// when
		err := cmd.Generate("/tmp/config.yaml")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to generate age key")
	})

	t.Run("should return error when config save fails after generate", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity:   "/tmp/test-key.txt",
					Recipients: []string{},
				},
			},
			SaveErr: assert.AnError,
		}
		encryptionService := &doubles.MockEncryptionService{
			GeneratedPublicKey: "age1key",
		}
		cmd := commands.NewKeyCommand(configRepo, encryptionService, nil)

		// when
		err := cmd.Generate("/tmp/config.yaml")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to save config")
	})
}

func TestKeyCommand_Export_ErrorCases(t *testing.T) {
	t.Run("should return error when config load fails", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{LoadErr: assert.AnError}
		cmd := commands.NewKeyCommand(configRepo, nil, nil)

		// when
		err := cmd.Export("/tmp/config.yaml")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})
}

func TestKeyCommand_AddRecipient_ErrorCases(t *testing.T) {
	t.Run("should return error when config load fails", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{LoadErr: assert.AnError}
		cmd := commands.NewKeyCommand(configRepo, nil, nil)

		// when
		err := cmd.AddRecipient("/tmp/config.yaml", "age1key")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})

	t.Run("should return error when config save fails", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Recipients: []string{},
				},
			},
			SaveErr: assert.AnError,
		}
		cmd := commands.NewKeyCommand(configRepo, nil, nil)

		// when
		err := cmd.AddRecipient("/tmp/config.yaml", "age1newkey")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to save config")
	})
}

func TestKeyCommand_Import(t *testing.T) {
	t.Run("should call ImportKey and update config with exported public key", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity:   "/tmp/dest-key.txt",
					Recipients: []string{},
				},
			},
		}
		encryptionService := &doubles.MockEncryptionService{
			ExportedPublicKey: "age1importedkey789",
		}
		cmd := commands.NewKeyCommand(configRepo, encryptionService, nil)

		// when
		err := cmd.Import("/tmp/config.yaml", "/tmp/source-key.txt")

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, encryptionService.ImportCalls)
		assert.Equal(t, "/tmp/source-key.txt", encryptionService.ImportSourcePath)
		assert.Equal(t, "/tmp/dest-key.txt", encryptionService.ImportDestPath)
		assert.Equal(t, 1, encryptionService.ExportCalls)
		assert.Equal(t, 1, configRepo.SaveCalls)
		assert.Contains(t, configRepo.SavedConfig.Encryption.Recipients, "age1importedkey789")
	})

	t.Run("should return error when ImportKey fails", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity: "/tmp/dest-key.txt",
				},
			},
		}
		encryptionService := &doubles.MockEncryptionService{
			ImportErr: assert.AnError,
		}
		cmd := commands.NewKeyCommand(configRepo, encryptionService, nil)

		// when
		err := cmd.Import("/tmp/config.yaml", "/tmp/source-key.txt")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to import age key")
	})
}

func TestKeyCommand_ImportFromOp(t *testing.T) {
	t.Run("should fetch identity from 1Password and update config recipients", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity:   "/tmp/dest-key.txt",
					Recipients: []string{},
					Op: &entities.OpConfig{
						Enabled: true,
						Vault:   "Personal",
						Item:    "aisync.age",
					},
				},
			},
		}
		encryptionService := &doubles.MockEncryptionService{
			ExportedPublicKey: "age1importedfromop",
		}
		opRepo := &doubles.MockOpSecretRepository{
			Identity: "AGE-SECRET-KEY-1FAKEIDENTITY",
		}
		cmd := commands.NewKeyCommand(configRepo, encryptionService, opRepo)

		// when
		err := cmd.ImportFromOp("/tmp/config.yaml")

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, opRepo.GetIdentityCalls)
		assert.Equal(t, "Personal", opRepo.RequestedVault)
		assert.Equal(t, "aisync.age", opRepo.RequestedItem)
		assert.Equal(t, 1, encryptionService.ImportContentCalls)
		assert.Equal(t, []byte("AGE-SECRET-KEY-1FAKEIDENTITY"), encryptionService.ImportContent)
		assert.Equal(t, "/tmp/dest-key.txt", encryptionService.ImportContentDest)
		assert.Equal(t, 1, encryptionService.ExportCalls)
		assert.Equal(t, 1, configRepo.SaveCalls)
		assert.Contains(t, configRepo.SavedConfig.Encryption.Recipients, "age1importedfromop")
	})

	t.Run("should default item name to aisync.age when Op.Item is empty", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity: "/tmp/dest-key.txt",
					Op:       &entities.OpConfig{Enabled: true, Vault: "Work"},
				},
			},
		}
		encryptionService := &doubles.MockEncryptionService{
			ExportedPublicKey: "age1key",
		}
		opRepo := &doubles.MockOpSecretRepository{Identity: "AGE-SECRET-KEY-1X"}
		cmd := commands.NewKeyCommand(configRepo, encryptionService, opRepo)

		// when
		err := cmd.ImportFromOp("/tmp/config.yaml")

		// then
		require.NoError(t, err)
		assert.Equal(t, "aisync.age", opRepo.RequestedItem)
		assert.Equal(t, "Work", opRepo.RequestedVault)
	})

	t.Run("should return error when 1Password integration is disabled", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Op: &entities.OpConfig{Enabled: false},
				},
			},
		}
		cmd := commands.NewKeyCommand(configRepo, nil, nil)

		// when
		err := cmd.ImportFromOp("/tmp/config.yaml")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "1Password integration is disabled")
	})

	t.Run("should return error when Op block is missing", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{},
			},
		}
		cmd := commands.NewKeyCommand(configRepo, nil, nil)

		// when
		err := cmd.ImportFromOp("/tmp/config.yaml")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "1Password integration is disabled")
	})

	t.Run("should return error when GetIdentity fails", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity: "/tmp/dest-key.txt",
					Op:       &entities.OpConfig{Enabled: true, Vault: "Personal"},
				},
			},
		}
		opRepo := &doubles.MockOpSecretRepository{GetIdentityErr: assert.AnError}
		cmd := commands.NewKeyCommand(configRepo, &doubles.MockEncryptionService{}, opRepo)

		// when
		err := cmd.ImportFromOp("/tmp/config.yaml")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to fetch age identity from 1Password")
	})

	t.Run("should return error when ImportKeyContent fails", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity: "/tmp/dest-key.txt",
					Op:       &entities.OpConfig{Enabled: true, Vault: "Personal"},
				},
			},
		}
		encryptionService := &doubles.MockEncryptionService{ImportContentErr: assert.AnError}
		opRepo := &doubles.MockOpSecretRepository{Identity: "AGE-SECRET-KEY-1X"}
		cmd := commands.NewKeyCommand(configRepo, encryptionService, opRepo)

		// when
		err := cmd.ImportFromOp("/tmp/config.yaml")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to write age identity")
	})

	t.Run("should return error when ExportPublicKey fails after import", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity: "/tmp/dest-key.txt",
					Op:       &entities.OpConfig{Enabled: true, Vault: "Personal"},
				},
			},
		}
		encryptionService := &doubles.MockEncryptionService{ExportErr: assert.AnError}
		opRepo := &doubles.MockOpSecretRepository{Identity: "AGE-SECRET-KEY-1X"}
		cmd := commands.NewKeyCommand(configRepo, encryptionService, opRepo)

		// when
		err := cmd.ImportFromOp("/tmp/config.yaml")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to export public key after import")
	})

	t.Run("should return error when config save fails", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity: "/tmp/dest-key.txt",
					Op:       &entities.OpConfig{Enabled: true, Vault: "Personal"},
				},
			},
			SaveErr: assert.AnError,
		}
		encryptionService := &doubles.MockEncryptionService{ExportedPublicKey: "age1k"}
		opRepo := &doubles.MockOpSecretRepository{Identity: "AGE-SECRET-KEY-1X"}
		cmd := commands.NewKeyCommand(configRepo, encryptionService, opRepo)

		// when
		err := cmd.ImportFromOp("/tmp/config.yaml")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to save config")
	})

	t.Run("should return error when config load fails", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{LoadErr: assert.AnError}
		cmd := commands.NewKeyCommand(configRepo, nil, nil)

		// when
		err := cmd.ImportFromOp("/tmp/config.yaml")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})
}

//go:build unit

package repositories_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	infraRepos "github.com/rios0rios0/aisync/internal/infrastructure/repositories"
	"github.com/rios0rios0/aisync/test/doubles"
)

func TestAgeForbiddenTermsRepository(t *testing.T) {
	t.Parallel()

	t.Run("should return nil terms when file is absent", func(t *testing.T) {
		t.Parallel()

		// given
		repoPath := t.TempDir()
		encSvc := &doubles.MockEncryptionService{}
		loadConfig := func(string) (*entities.Config, error) {
			return &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity:   "/tmp/key.txt",
					Recipients: []string{"age1test"},
				},
			}, nil
		}
		repo := infraRepos.NewAgeForbiddenTermsRepository(encSvc, loadConfig)

		// when
		terms, err := repo.Load(repoPath)

		// then
		require.NoError(t, err)
		assert.Nil(t, terms)
	})

	t.Run("should refuse to save with empty recipients", func(t *testing.T) {
		t.Parallel()

		// given
		repoPath := t.TempDir()
		encSvc := &doubles.MockEncryptionService{}
		loadConfig := func(string) (*entities.Config, error) {
			return &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity:   "/tmp/key.txt",
					Recipients: []string{}, // no recipients
				},
			}, nil
		}
		repo := infraRepos.NewAgeForbiddenTermsRepository(encSvc, loadConfig)

		// when
		err := repo.Save(repoPath, []entities.ForbiddenTerm{})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no encryption recipients")
	})

	t.Run("should round-trip terms through encrypt+decrypt", func(t *testing.T) {
		t.Parallel()

		// given — a mock encryption service where "encrypt" just wraps the
		// plaintext in sentinel bytes and "decrypt" unwraps them, so the
		// plaintext round-trips unchanged.
		repoPath := t.TempDir()
		encSvc := &doubles.MockEncryptionService{
			EncryptedData: []byte("<cipher>plaintext</cipher>"),
			DecryptedData: nil, // filled in after Save
		}
		loadConfig := func(string) (*entities.Config, error) {
			return &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity:   "/tmp/key.txt",
					Recipients: []string{"age1test"},
				},
			}, nil
		}
		repo := infraRepos.NewAgeForbiddenTermsRepository(encSvc, loadConfig)

		canonical, err := entities.NewCanonicalTerm("ZestSecurity", "user")
		require.NoError(t, err)
		word, err := entities.NewCanonicalWordTerm("QA", "user")
		require.NoError(t, err)
		regexTerm, err := entities.NewRegexTerm(`\bZest-[A-Z]\w+\b`, "user")
		require.NoError(t, err)
		terms := []entities.ForbiddenTerm{canonical, word, regexTerm}

		// when — save, capture the plaintext the mock received, set up the
		// mock to return that plaintext on decrypt, then load.
		err = repo.Save(repoPath, terms)
		require.NoError(t, err)

		// The mock stored the plaintext it received via Encrypt. We feed that
		// back as DecryptedData so Load sees the same content.
		encSvc.DecryptedData = encSvc.EncryptPlaintext

		loaded, err := repo.Load(repoPath)
		require.NoError(t, err)

		// then — three terms round-trip with original spellings preserved
		require.Len(t, loaded, 3)
		assert.Equal(t, "ZestSecurity", loaded[0].Original)
		assert.Equal(t, entities.ForbiddenModeCanonical, loaded[0].Mode)
		assert.Equal(t, "QA", loaded[1].Original)
		assert.Equal(t, entities.ForbiddenModeCanonicalWord, loaded[1].Mode)
		assert.Equal(t, `\bZest-[A-Z]\w+\b`, loaded[2].Original)
		assert.Equal(t, entities.ForbiddenModeRegex, loaded[2].Mode)
	})

	t.Run("should return path alongside config.yaml", func(t *testing.T) {
		t.Parallel()

		// given
		repoPath := "/tmp/fake-sync-repo"
		encSvc := &doubles.MockEncryptionService{}
		loadConfig := func(string) (*entities.Config, error) { return &entities.Config{}, nil }
		repo := infraRepos.NewAgeForbiddenTermsRepository(encSvc, loadConfig)

		// when
		path := repo.Path(repoPath)

		// then
		assert.Equal(t, filepath.Join(repoPath, ".aisync-forbidden.age"), path)
	})

	t.Run("should fail to decrypt when identity is missing from config", func(t *testing.T) {
		t.Parallel()

		// given
		repoPath := t.TempDir()
		require.NoError(t, os.WriteFile(
			filepath.Join(repoPath, ".aisync-forbidden.age"),
			[]byte("<cipher>"),
			0600,
		))
		encSvc := &doubles.MockEncryptionService{}
		loadConfig := func(string) (*entities.Config, error) {
			return &entities.Config{
				Encryption: entities.EncryptionConfig{
					Identity: "", // missing
				},
			}, nil
		}
		repo := infraRepos.NewAgeForbiddenTermsRepository(encSvc, loadConfig)

		// when
		_, err := repo.Load(repoPath)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no encryption identity")
	})
}

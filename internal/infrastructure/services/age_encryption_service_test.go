//go:build unit

package services_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	services "github.com/rios0rios0/aisync/internal/infrastructure/services"
)

func TestAgeEncryptionService_GenerateKey_ShouldCreateFileAndReturnPublicKeyStartingWithAge1(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "keys", "identity.txt")
	svc := services.NewAgeEncryptionService()

	// when
	publicKey, err := svc.GenerateKey(outputPath)

	// then
	assert.NoError(t, err)
	assert.True(t, strings.HasPrefix(publicKey, "age1"), "public key should start with 'age1'")

	_, statErr := os.Stat(outputPath)
	assert.NoError(t, statErr, "identity file should exist")

	content, readErr := os.ReadFile(outputPath)
	assert.NoError(t, readErr)
	assert.Contains(t, string(content), "AGE-SECRET-KEY-")
	assert.Contains(t, string(content), publicKey)
}

func TestAgeEncryptionService_EncryptDecrypt_ShouldRoundtripSuccessfully(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	identityPath := filepath.Join(tmpDir, "identity.txt")
	svc := services.NewAgeEncryptionService()

	publicKey, err := svc.GenerateKey(identityPath)
	assert.NoError(t, err)

	plaintext := []byte("secret message for roundtrip test")

	// when
	ciphertext, encErr := svc.Encrypt(plaintext, []string{publicKey})
	assert.NoError(t, encErr)
	assert.NotEmpty(t, ciphertext)

	decrypted, decErr := svc.Decrypt(ciphertext, identityPath)

	// then
	assert.NoError(t, decErr)
	assert.Equal(t, plaintext, decrypted)
}

func TestAgeEncryptionService_ImportKey_ShouldValidateIdentityFormat(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "source-identity.txt")
	destPath := filepath.Join(tmpDir, "imported", "identity.txt")
	svc := services.NewAgeEncryptionService()

	// Generate a valid identity first
	_, err := svc.GenerateKey(sourcePath)
	assert.NoError(t, err)

	// when
	importErr := svc.ImportKey(sourcePath, destPath)

	// then
	assert.NoError(t, importErr)

	_, statErr := os.Stat(destPath)
	assert.NoError(t, statErr, "imported identity file should exist")
}

func TestAgeEncryptionService_ImportKey_ShouldFailWhenSourceHasInvalidIdentity(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "invalid-identity.txt")
	destPath := filepath.Join(tmpDir, "dest", "identity.txt")
	svc := services.NewAgeEncryptionService()

	err := os.WriteFile(sourcePath, []byte("this is not a valid age identity"), 0600)
	assert.NoError(t, err)

	// when
	importErr := svc.ImportKey(sourcePath, destPath)

	// then
	assert.Error(t, importErr)
	assert.Contains(t, importErr.Error(), "failed to parse age identity")
}

func TestAgeEncryptionService_ExportPublicKey_ShouldReturnCorrectKey(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	identityPath := filepath.Join(tmpDir, "identity.txt")
	svc := services.NewAgeEncryptionService()

	expectedPubKey, err := svc.GenerateKey(identityPath)
	assert.NoError(t, err)

	// when
	exportedPubKey, exportErr := svc.ExportPublicKey(identityPath)

	// then
	assert.NoError(t, exportErr)
	assert.Equal(t, expectedPubKey, exportedPubKey)
	assert.True(t, strings.HasPrefix(exportedPubKey, "age1"))
}

func TestAgeEncryptionService_Decrypt_ShouldFailWithWrongKey(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	svc := services.NewAgeEncryptionService()

	// Generate first identity and encrypt with it
	identity1Path := filepath.Join(tmpDir, "identity1.txt")
	pubKey1, err := svc.GenerateKey(identity1Path)
	assert.NoError(t, err)

	plaintext := []byte("encrypted with identity 1")
	ciphertext, encErr := svc.Encrypt(plaintext, []string{pubKey1})
	assert.NoError(t, encErr)

	// Generate a second, different identity
	identity2Path := filepath.Join(tmpDir, "identity2.txt")
	_, err = svc.GenerateKey(identity2Path)
	assert.NoError(t, err)

	// when
	_, decErr := svc.Decrypt(ciphertext, identity2Path)

	// then
	assert.Error(t, decErr)
	assert.Contains(t, decErr.Error(), "failed to decrypt")
}

func TestAgeEncryptionService_Encrypt_ShouldFailWithNoRecipients(t *testing.T) {
	// given
	svc := services.NewAgeEncryptionService()

	// when
	_, err := svc.Encrypt([]byte("data"), nil)

	// then
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one recipient is required")
}

func TestAgeEncryptionService_Encrypt_ShouldFailWithInvalidRecipient(t *testing.T) {
	// given
	svc := services.NewAgeEncryptionService()

	// when
	_, err := svc.Encrypt([]byte("data"), []string{"not-a-valid-recipient"})

	// then
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse recipient")
}

func TestAgeEncryptionService_ExportPublicKey_ShouldFailWhenFileDoesNotExist(t *testing.T) {
	// given
	svc := services.NewAgeEncryptionService()

	// when
	_, err := svc.ExportPublicKey("/nonexistent/path/identity.txt")

	// then
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read identity file")
}

func TestAgeEncryptionService_ImportKey_ShouldFailWhenSourceDoesNotExist(t *testing.T) {
	// given
	svc := services.NewAgeEncryptionService()
	destPath := filepath.Join(t.TempDir(), "dest", "identity.txt")

	// when
	err := svc.ImportKey("/nonexistent/source.txt", destPath)

	// then
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read source identity file")
}

func TestAgeEncryptionService_Decrypt_ShouldFailWhenIdentityFileDoesNotExist(t *testing.T) {
	// given
	svc := services.NewAgeEncryptionService()

	// when
	_, err := svc.Decrypt([]byte("data"), "/nonexistent/identity.txt")

	// then
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read identity file for decryption")
}

func TestAgeEncryptionService_Decrypt_ShouldFailWhenIdentityFileIsInvalid(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	identityPath := filepath.Join(tmpDir, "invalid.txt")
	assert.NoError(t, os.WriteFile(identityPath, []byte("not a valid identity"), 0600))
	svc := services.NewAgeEncryptionService()

	// when
	_, err := svc.Decrypt([]byte("data"), identityPath)

	// then
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse age identity for decryption")
}

func TestAgeEncryptionService_ExportPublicKey_ShouldFailWhenFileContainsInvalidIdentity(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	identityPath := filepath.Join(tmpDir, "invalid.txt")
	assert.NoError(t, os.WriteFile(identityPath, []byte("not a valid age key"), 0600))
	svc := services.NewAgeEncryptionService()

	// when
	_, err := svc.ExportPublicKey(identityPath)

	// then
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse age identity")
}

func TestAgeEncryptionService_Encrypt_ShouldSucceedWithMultipleRecipients(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	svc := services.NewAgeEncryptionService()

	id1Path := filepath.Join(tmpDir, "id1.txt")
	pubKey1, err := svc.GenerateKey(id1Path)
	assert.NoError(t, err)

	id2Path := filepath.Join(tmpDir, "id2.txt")
	pubKey2, err := svc.GenerateKey(id2Path)
	assert.NoError(t, err)

	plaintext := []byte("multi-recipient test")

	// when
	ciphertext, encErr := svc.Encrypt(plaintext, []string{pubKey1, pubKey2})

	// then
	assert.NoError(t, encErr)
	assert.NotEmpty(t, ciphertext)

	// Both identities should be able to decrypt
	dec1, err := svc.Decrypt(ciphertext, id1Path)
	assert.NoError(t, err)
	assert.Equal(t, plaintext, dec1)

	dec2, err := svc.Decrypt(ciphertext, id2Path)
	assert.NoError(t, err)
	assert.Equal(t, plaintext, dec2)
}

package services

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"filippo.io/age"
)

// AgeEncryptionService implements EncryptionService using the age encryption library.
type AgeEncryptionService struct{}

// NewAgeEncryptionService creates a new AgeEncryptionService.
func NewAgeEncryptionService() *AgeEncryptionService {
	return &AgeEncryptionService{}
}

// GenerateKey creates a new age X25519 key pair, writes the identity to outputPath,
// and returns the public key (recipient) string.
func (s *AgeEncryptionService) GenerateKey(outputPath string) (string, error) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return "", fmt.Errorf("failed to generate age identity: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0700); err != nil {
		return "", fmt.Errorf("failed to create directory for identity file: %w", err)
	}

	content := fmt.Sprintf(
		"# created: age identity\n# public key: %s\n%s\n",
		identity.Recipient().String(),
		identity.String(),
	)

	if err := os.WriteFile(outputPath, []byte(content), 0600); err != nil {
		return "", fmt.Errorf("failed to write identity file: %w", err)
	}

	return identity.Recipient().String(), nil
}

// ImportKey copies an existing age identity file from sourcePath to destPath,
// validating that it contains a valid age identity.
func (s *AgeEncryptionService) ImportKey(sourcePath, destPath string) error {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to read source identity file: %w", err)
	}

	identities, err := age.ParseIdentities(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to parse age identity from source file: %w", err)
	}
	if len(identities) == 0 {
		return fmt.Errorf("no valid age identities found in %s", sourcePath)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0700); err != nil {
		return fmt.Errorf("failed to create directory for identity file: %w", err)
	}

	if err := os.WriteFile(destPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write identity file: %w", err)
	}

	return nil
}

// ExportPublicKey reads an age identity file and returns the corresponding
// public key (recipient) string.
func (s *AgeEncryptionService) ExportPublicKey(identityPath string) (string, error) {
	data, err := os.ReadFile(identityPath)
	if err != nil {
		return "", fmt.Errorf("failed to read identity file: %w", err)
	}

	identities, err := age.ParseIdentities(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to parse age identity: %w", err)
	}
	if len(identities) == 0 {
		return "", fmt.Errorf("no valid age identities found in %s", identityPath)
	}

	x25519Identity, ok := identities[0].(*age.X25519Identity)
	if !ok {
		return "", fmt.Errorf("identity in %s is not an X25519 identity", identityPath)
	}

	return x25519Identity.Recipient().String(), nil
}

// Encrypt encrypts plaintext bytes for the given age recipients.
func (s *AgeEncryptionService) Encrypt(plaintext []byte, recipients []string) ([]byte, error) {
	if len(recipients) == 0 {
		return nil, fmt.Errorf("at least one recipient is required for encryption")
	}

	parsedRecipients := make([]age.Recipient, 0, len(recipients))
	for _, r := range recipients {
		recipient, err := age.ParseX25519Recipient(r)
		if err != nil {
			return nil, fmt.Errorf("failed to parse recipient '%s': %w", r, err)
		}
		parsedRecipients = append(parsedRecipients, recipient)
	}

	var buf bytes.Buffer
	writer, err := age.Encrypt(&buf, parsedRecipients...)
	if err != nil {
		return nil, fmt.Errorf("failed to create age encryption writer: %w", err)
	}

	if _, err := writer.Write(plaintext); err != nil {
		return nil, fmt.Errorf("failed to write plaintext to age encryption writer: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize age encryption: %w", err)
	}

	return buf.Bytes(), nil
}

// Decrypt decrypts ciphertext bytes using the identity at the given path.
func (s *AgeEncryptionService) Decrypt(ciphertext []byte, identityPath string) ([]byte, error) {
	data, err := os.ReadFile(identityPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read identity file for decryption: %w", err)
	}

	identities, err := age.ParseIdentities(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse age identity for decryption: %w", err)
	}
	if len(identities) == 0 {
		return nil, fmt.Errorf("no valid age identities found in %s", identityPath)
	}

	reader, err := age.Decrypt(bytes.NewReader(ciphertext), identities...)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt ciphertext: %w", err)
	}

	decrypted, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read decrypted data: %w", err)
	}

	return decrypted, nil
}

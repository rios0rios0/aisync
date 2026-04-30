package repositories

// EncryptionService defines the contract for encrypting and decrypting files using age.
type EncryptionService interface {
	// GenerateKey creates a new age X25519 key pair, writes the identity to outputPath,
	// and returns the public key (recipient) string.
	GenerateKey(outputPath string) (publicKey string, err error)

	// ImportKey copies an existing age identity file from sourcePath to destPath,
	// validating that it contains a valid age identity.
	ImportKey(sourcePath, destPath string) error

	// ImportKeyContent writes raw age identity content to destPath after
	// validating that it parses as a valid age identity. Used by import
	// flows that obtain the key from a non-file source (e.g. 1Password)
	// so the secret never lives in a temporary file on disk.
	ImportKeyContent(content []byte, destPath string) error

	// ExportPublicKey reads an age identity file and returns the corresponding
	// public key (recipient) string.
	ExportPublicKey(identityPath string) (string, error)

	// Encrypt encrypts plaintext bytes for the given age recipients.
	Encrypt(plaintext []byte, recipients []string) ([]byte, error)

	// Decrypt decrypts ciphertext bytes using the identity at the given path.
	Decrypt(ciphertext []byte, identityPath string) ([]byte, error)
}

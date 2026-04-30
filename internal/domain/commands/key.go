package commands

import (
	"fmt"
	"os"
	"slices"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// KeyCommand manages age encryption keys for the aisync configuration.
type KeyCommand struct {
	configRepo        repositories.ConfigRepository
	encryptionService repositories.EncryptionService
	opSecretRepo      repositories.OpSecretRepository
}

// NewKeyCommand creates a new KeyCommand.
func NewKeyCommand(
	configRepo repositories.ConfigRepository,
	encryptionService repositories.EncryptionService,
	opSecretRepo repositories.OpSecretRepository,
) *KeyCommand {
	return &KeyCommand{
		configRepo:        configRepo,
		encryptionService: encryptionService,
		opSecretRepo:      opSecretRepo,
	}
}

// Generate creates a new age key pair, saves the identity to disk,
// and updates the config with the new public key as a recipient.
func (c *KeyCommand) Generate(configPath string) error {
	config, err := c.configRepo.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	identityPath := c.resolveIdentityPath(config.Encryption.Identity)
	logger.Infof("generating new age key pair at %s", identityPath)

	publicKey, err := c.encryptionService.GenerateKey(identityPath)
	if err != nil {
		return fmt.Errorf("failed to generate age key: %w", err)
	}

	config.Encryption.Recipients = appendUniqueRecipient(config.Encryption.Recipients, publicKey)

	if err = c.configRepo.Save(configPath, config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	logger.Infof("age key generated successfully")
	fmt.Fprintf(os.Stdout, "Public key: %s\n", publicKey)
	fmt.Fprintf(os.Stdout, "Identity:   %s\n", identityPath)

	return nil
}

// Import copies an existing age identity file into the configured location
// and updates the config with its public key.
func (c *KeyCommand) Import(configPath, keyPath string) error {
	config, err := c.configRepo.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	identityPath := c.resolveIdentityPath(config.Encryption.Identity)
	logger.Infof("importing age key from %s to %s", keyPath, identityPath)

	if err = c.encryptionService.ImportKey(keyPath, identityPath); err != nil {
		return fmt.Errorf("failed to import age key: %w", err)
	}

	publicKey, err := c.encryptionService.ExportPublicKey(identityPath)
	if err != nil {
		return fmt.Errorf("failed to export public key after import: %w", err)
	}

	config.Encryption.Recipients = appendUniqueRecipient(config.Encryption.Recipients, publicKey)

	if err = c.configRepo.Save(configPath, config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	logger.Infof("age key imported successfully")
	fmt.Fprintf(os.Stdout, "Public key: %s\n", publicKey)
	fmt.Fprintf(os.Stdout, "Identity:   %s\n", identityPath)

	return nil
}

// ImportFromOp fetches the age identity from a 1Password item via the
// `op` CLI and writes it to the path aisync uses for every other key
// operation (resolveIdentityPath: config.Encryption.Identity →
// AISYNC_KEY_FILE → ~/.config/aisync/key.txt). The pubkey derived from
// the imported identity is appended to the recipients list so future
// pushes encrypt to this device too.
//
// Errors out when encryption.op is absent or encryption.op.enabled is
// false to keep the 1Password integration strictly opt-in.
func (c *KeyCommand) ImportFromOp(configPath string) error {
	config, err := c.configRepo.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if config.Encryption.Op == nil || !config.Encryption.Op.Enabled {
		return fmt.Errorf(
			"1Password integration is disabled: set encryption.op.enabled: true in %s",
			configPath,
		)
	}

	vault := config.Encryption.Op.Vault
	item := config.Encryption.Op.ItemOrDefault()
	identityPath := c.resolveIdentityPath(config.Encryption.Identity)

	logger.Infof("fetching age identity from 1Password item %q in vault %q", item, vault)
	content, err := c.opSecretRepo.GetIdentity(vault, item)
	if err != nil {
		return fmt.Errorf("failed to fetch age identity from 1Password: %w", err)
	}

	if err = c.encryptionService.ImportKeyContent([]byte(content), identityPath); err != nil {
		return fmt.Errorf("failed to write age identity to %s: %w", identityPath, err)
	}

	publicKey, err := c.encryptionService.ExportPublicKey(identityPath)
	if err != nil {
		return fmt.Errorf("failed to export public key after import: %w", err)
	}

	config.Encryption.Recipients = appendUniqueRecipient(config.Encryption.Recipients, publicKey)

	if err = c.configRepo.Save(configPath, config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	logger.Infof("age key imported from 1Password successfully")
	fmt.Fprintf(os.Stdout, "Public key: %s\n", publicKey)
	fmt.Fprintf(os.Stdout, "Identity:   %s\n", identityPath)

	return nil
}

// Export reads the configured identity file and prints the public key to stdout.
func (c *KeyCommand) Export(configPath string) error {
	config, err := c.configRepo.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	identityPath := c.resolveIdentityPath(config.Encryption.Identity)

	publicKey, err := c.encryptionService.ExportPublicKey(identityPath)
	if err != nil {
		return fmt.Errorf("failed to export public key: %w", err)
	}

	fmt.Fprintln(os.Stdout, publicKey)

	return nil
}

// AddRecipient appends a public key to the config's recipients list.
func (c *KeyCommand) AddRecipient(configPath, publicKey string) error {
	config, err := c.configRepo.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	config.Encryption.Recipients = appendUniqueRecipient(config.Encryption.Recipients, publicKey)

	if err = c.configRepo.Save(configPath, config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	logger.Infof("added recipient %s", publicKey)

	return nil
}

// resolveIdentityPath expands the identity path from the config, falling back
// to the AISYNC_KEY_FILE environment variable and then the default location.
func (c *KeyCommand) resolveIdentityPath(identity string) string {
	if identity == "" {
		if envKey := os.Getenv("AISYNC_KEY_FILE"); envKey != "" {
			return ExpandHome(envKey)
		}
		identity = "~/.config/aisync/key.txt"
	}
	return ExpandHome(identity)
}

// appendUniqueRecipient appends a public key to the recipients slice only if
// it is not already present.
func appendUniqueRecipient(recipients []string, publicKey string) []string {
	if slices.Contains(recipients, publicKey) {
		return recipients
	}
	return append(recipients, publicKey)
}

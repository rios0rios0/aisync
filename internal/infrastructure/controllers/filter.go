package controllers

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// newFilterSubcmds returns hidden _clean and _smudge commands used as git
// clean/smudge filters for transparent encryption. These commands read from
// stdin and write to stdout, conforming to git's filter protocol.
func newFilterSubcmds(encSvc repositories.EncryptionService) []*cobra.Command {
	//nolint:exhaustruct
	cleanCmd := &cobra.Command{
		Use:    "_clean",
		Short:  "Git clean filter: encrypt stdin to stdout",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			identityPath, err := findIdentityPath()
			if err != nil {
				return err
			}
			pubKey, err := encSvc.ExportPublicKey(identityPath)
			if err != nil {
				return fmt.Errorf("failed to export public key: %w", err)
			}
			plaintext, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("failed to read stdin: %w", err)
			}
			ciphertext, err := encSvc.Encrypt(plaintext, []string{pubKey})
			if err != nil {
				return fmt.Errorf("failed to encrypt: %w", err)
			}
			_, err = os.Stdout.Write(ciphertext)
			return err
		},
	}

	//nolint:exhaustruct
	smudgeCmd := &cobra.Command{
		Use:    "_smudge",
		Short:  "Git smudge filter: decrypt stdin to stdout",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			identityPath, err := findIdentityPath()
			if err != nil {
				return err
			}
			ciphertext, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("failed to read stdin: %w", err)
			}
			plaintext, err := encSvc.Decrypt(ciphertext, identityPath)
			if err != nil {
				return fmt.Errorf("failed to decrypt: %w", err)
			}
			_, err = os.Stdout.Write(plaintext)
			return err
		},
	}

	return []*cobra.Command{cleanCmd, smudgeCmd}
}

// findIdentityPath discovers the age identity file by walking up from the
// current directory looking for config.yaml, or falling back to the default.
func findIdentityPath() (string, error) {
	// Check AISYNC_KEY_FILE env var first.
	if envKey := os.Getenv("AISYNC_KEY_FILE"); envKey != "" {
		return envKey, nil
	}

	// Walk up from CWD looking for config.yaml to read the identity path.
	dir, err := os.Getwd()
	if err != nil {
		return defaultIdentityPath(), nil
	}
	for {
		candidate := filepath.Join(dir, "config.yaml")
		if _, statErr := os.Stat(candidate); statErr == nil {
			// Found config.yaml; a real implementation would parse it for
			// encryption.identity. For now, use the default.
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return defaultIdentityPath(), nil
}

func defaultIdentityPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "aisync", "key.txt")
}

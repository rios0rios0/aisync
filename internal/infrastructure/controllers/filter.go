package controllers

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// newFilterSubcmds returns hidden _clean and _smudge commands used as git
// clean/smudge filters for transparent encryption. These commands read from
// stdin and write to stdout, conforming to git's filter protocol.
func newFilterSubcmds(encSvc repositories.EncryptionService) []*cobra.Command {
	//nolint:exhaustruct // cobra command does not require all fields
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

	//nolint:exhaustruct // cobra command does not require all fields
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
//
//nolint:unparam // error return reserved for future config parsing failures
func findIdentityPath() (string, error) {
	// Check AISYNC_KEY_FILE env var first.
	if envKey := os.Getenv("AISYNC_KEY_FILE"); envKey != "" {
		return envKey, nil
	}

	// Walk up from CWD looking for config.yaml to read the identity path.
	dir, err := os.Getwd()
	if err != nil {
		return defaultIdentityPath(), nil //nolint:nilerr // non-fatal: fall back to default path
	}
	for {
		candidate := filepath.Join(dir, "config.yaml")
		if data, readErr := os.ReadFile(candidate); readErr == nil {
			var cfg struct {
				Encryption struct {
					Identity string `yaml:"identity"`
				} `yaml:"encryption"`
			}
			if yamlErr := yaml.Unmarshal(data, &cfg); yamlErr == nil && cfg.Encryption.Identity != "" {
				return expandIdentityPath(cfg.Encryption.Identity), nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return defaultIdentityPath(), nil
}

// expandIdentityPath resolves ~/... and %ENVVAR%... paths for the identity file.
func expandIdentityPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	if strings.Contains(path, "%") {
		return expandWindowsEnvVars(path)
	}
	return os.ExpandEnv(path)
}

// expandWindowsEnvVars expands Windows-style %VAR% environment variables by
// replacing each %VAR% with the corresponding [os.Getenv] value, then falls
// back to [os.ExpandEnv] for any remaining $VAR/${VAR} patterns.
func expandWindowsEnvVars(path string) string {
	result := path
	for {
		start := strings.Index(result, "%")
		if start < 0 {
			break
		}
		end := strings.Index(result[start+1:], "%")
		if end < 0 {
			break
		}
		end += start + 1
		varName := result[start+1 : end]
		value := os.Getenv(varName)
		result = result[:start] + value + result[end+1:]
	}
	return os.ExpandEnv(result)
}

func defaultIdentityPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "aisync", "key.txt")
}

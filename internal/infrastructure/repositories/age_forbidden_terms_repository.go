package repositories

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// ForbiddenTermsFilename is the fixed filename aisync uses to store the
// encrypted forbidden-terms list inside a sync repo. It lives at the repo
// root next to `config.yaml`, `.aisyncignore`, etc.
const ForbiddenTermsFilename = ".aisync-forbidden.age"

// AgeForbiddenTermsRepository persists the user's explicit forbidden-terms
// list as an age-encrypted file inside the sync repo. It implements
// [repositories.ForbiddenTermsRepository].
//
// The repository depends on an [repositories.EncryptionService] to encrypt
// on save and decrypt on load, and on a loader that returns the current
// config snapshot (for access to the age identity path and the list of
// recipients). Taking the config loader as a function rather than the
// config itself lets the repository pick up changes to `config.yaml`
// between aisync invocations without a restart.
type AgeForbiddenTermsRepository struct {
	encryptionService repositories.EncryptionService
	loadConfig        func(repoPath string) (*entities.Config, error)
}

// NewAgeForbiddenTermsRepository builds a repository wired to the given
// encryption service and config loader.
func NewAgeForbiddenTermsRepository(
	encryptionService repositories.EncryptionService,
	loadConfig func(repoPath string) (*entities.Config, error),
) *AgeForbiddenTermsRepository {
	return &AgeForbiddenTermsRepository{
		encryptionService: encryptionService,
		loadConfig:        loadConfig,
	}
}

// Path returns the absolute on-disk path of the encrypted forbidden file
// for the given sync repo.
func (r *AgeForbiddenTermsRepository) Path(repoPath string) string {
	return filepath.Join(repoPath, ForbiddenTermsFilename)
}

// Load decrypts the forbidden file at the canonical path inside the repo
// and parses the contents into terms. Returns (nil, nil) when the file is
// absent so a first-run repo without any explicit terms works naturally.
func (r *AgeForbiddenTermsRepository) Load(repoPath string) ([]entities.ForbiddenTerm, error) {
	path := r.Path(repoPath)
	ciphertext, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}

	config, err := r.loadConfig(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config for forbidden-terms decryption: %w", err)
	}
	identityPath := expandConfigIdentity(config)
	if identityPath == "" {
		return nil, fmt.Errorf(
			"cannot decrypt %s: config has no encryption identity configured",
			ForbiddenTermsFilename,
		)
	}

	plaintext, err := r.encryptionService.Decrypt(ciphertext, identityPath)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt %s: %w", ForbiddenTermsFilename, err)
	}

	terms, err := entities.ParseForbiddenTermsFile(plaintext)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", ForbiddenTermsFilename, err)
	}
	return terms, nil
}

// Save encrypts the given terms using the config's age recipients and
// writes the ciphertext to the canonical path with 0600 permissions. It is
// an error to call Save when no recipients are configured — the resulting
// file would either leak plaintext or be unreadable by any device.
func (r *AgeForbiddenTermsRepository) Save(repoPath string, terms []entities.ForbiddenTerm) error {
	config, err := r.loadConfig(repoPath)
	if err != nil {
		return fmt.Errorf("failed to load config for forbidden-terms encryption: %w", err)
	}
	if len(config.Encryption.Recipients) == 0 {
		return fmt.Errorf("cannot save %s: config has no encryption recipients", ForbiddenTermsFilename)
	}

	plaintext := serializeForbiddenTerms(terms)
	ciphertext, err := r.encryptionService.Encrypt(plaintext, config.Encryption.Recipients)
	if err != nil {
		return fmt.Errorf("failed to encrypt forbidden-terms list: %w", err)
	}

	path := r.Path(repoPath)
	if writeErr := os.WriteFile(path, ciphertext, 0600); writeErr != nil {
		return fmt.Errorf("failed to write %s: %w", path, writeErr)
	}
	return nil
}

// serializeForbiddenTerms renders a slice of terms back into the plain-text
// forbidden-file format that [entities.ParseForbiddenTermsFile] consumes.
// Round-trip safe: Load(Save(x)) returns a slice with the same terms.
func serializeForbiddenTerms(terms []entities.ForbiddenTerm) []byte {
	var b strings.Builder
	b.WriteString("# aisync encrypted forbidden-terms list.\n")
	b.WriteString("# Managed by `aisync nda add/remove/import`. Do not edit by hand.\n")
	b.WriteString("#\n")
	b.WriteString("# Each non-comment line is a pattern that blocks `aisync push` when it\n")
	b.WriteString("# matches file content. Default is canonical-form substring match\n")
	b.WriteString("# (catches spacing/casing/separator/accent variants). Prefix with\n")
	b.WriteString("# `word:` for word-boundary match, `regex:` for raw Go regex.\n")
	b.WriteString("\n")
	for _, term := range terms {
		switch term.Mode {
		case entities.ForbiddenModeCanonical:
			b.WriteString(term.Original)
		case entities.ForbiddenModeCanonicalWord:
			b.WriteString("word:")
			b.WriteString(term.Original)
		case entities.ForbiddenModeRegex:
			b.WriteString("regex:")
			b.WriteString(term.Original)
		}
		b.WriteString("\n")
	}
	return []byte(b.String())
}

// expandConfigIdentity resolves the age identity path from the config,
// returning an empty string if no identity is configured. Uses the
// ExpandHome helper pattern the rest of aisync relies on (the same
// ~-expansion the init/pull/push commands use).
func expandConfigIdentity(config *entities.Config) string {
	if config == nil {
		return ""
	}
	identity := config.Encryption.Identity
	if identity == "" {
		return ""
	}
	if strings.HasPrefix(identity, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return identity
		}
		return filepath.Join(home, identity[2:])
	}
	return identity
}

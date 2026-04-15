package repositories

import "github.com/rios0rios0/aisync/internal/domain/entities"

// ForbiddenTermsRepository persists the explicit NDA-term list as an
// encrypted file inside the sync repo. The repository handles
// encryption/decryption internally so callers only see plaintext
// [entities.ForbiddenTerm] slices.
//
// The canonical on-disk location is `<repoPath>/.aisync-forbidden.age`, and
// the ciphertext is an age encryption of the plain-text forbidden-file
// format (`word:`/`regex:` prefixes and `#` comments as defined in
// [entities.ParseForbiddenTermsFile]). Because the ciphertext lives inside
// the sync repo, it travels across devices via the normal `aisync push` /
// `aisync pull` flow — users only need to carry the age identity itself
// between machines (typically via 1Password).
type ForbiddenTermsRepository interface {
	// Load decrypts the forbidden file at the canonical path inside the
	// given repo and parses the contents into a list of terms. Returns
	// (nil, nil) — not an error — when the file is absent, so a first-run
	// repo without any explicit terms works naturally.
	Load(repoPath string) ([]entities.ForbiddenTerm, error)

	// Save encrypts the given terms using the config's age recipients and
	// writes the ciphertext to the canonical path. It is an error to call
	// Save when no recipients are configured.
	Save(repoPath string, terms []entities.ForbiddenTerm) error

	// Path returns the absolute on-disk path of the encrypted forbidden
	// file for the given sync repo. Useful for user-facing messages that
	// point at "where to look" after a command runs.
	Path(repoPath string) string
}

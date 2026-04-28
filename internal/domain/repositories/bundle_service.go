package repositories

import "github.com/rios0rios0/aisync/internal/domain/entities"

// BundleFile is one extracted file from a bundle tarball, with the
// metadata needed for mtime-wins merging on the pull side.
type BundleFile struct {
	RelativePath string
	Content      []byte
	ModTime      int64
	Mode         uint32
}

// BundleMergeReport summarises what changed in the local directory after
// a bundle was merged in. Used by the pull command for user-facing
// progress output.
type BundleMergeReport struct {
	Added      []string
	Overwrote  []string
	SkippedNew []string
}

// BundleService is the contract for producing and consuming opaque
// project bundles. Implementations are responsible for the tar+gzip
// transport, the age round-trip, and the merge semantics declared by
// each [entities.BundleSpec].
type BundleService interface {
	// HashName returns the deterministic 16-hex-character bundle filename
	// used for a source directory name. The hash is keyed by an HMAC key
	// derived from the age identity at identityPath, so two devices that
	// share the same age identity MUST produce the same hash (git delta
	// detection still works) but an attacker without the identity cannot
	// compute or verify a hash for a guessed source name. This closes
	// the SHA-256 confirmation oracle that existed in earlier versions.
	HashName(sourceDirName, identityPath string) (string, error)

	// Bundle packages the contents of sourceDir (recursively) into a
	// single age-encrypted gzip-compressed tarball, with a manifest entry
	// declaring originalName. The returned bytes are ready to write
	// directly to <repo>/personal/<tool>/<target>/<HashName(originalName)>.age.
	Bundle(
		sourceDir, originalName string,
		recipients []string,
	) ([]byte, *entities.BundleManifest, error)

	// Extract reverses Bundle: age-decrypts ciphertext using the identity
	// at identityPath, gunzips the inner tarball, and returns the
	// manifest plus every file member. Returns an error if the manifest
	// is missing or its schema version is newer than this binary
	// understands.
	Extract(
		ciphertext []byte,
		identityPath string,
	) (*entities.BundleManifest, []BundleFile, error)

	// MergeIntoLocal applies extracted files to targetDir using the
	// configured strategy. For [entities.BundleMergeMTime], existing
	// local files newer than the bundle copy are preserved; bundle files
	// not present locally are added; local-only files are never touched.
	// For [entities.BundleMergeReplace], every bundle file overwrites the
	// local copy.
	MergeIntoLocal(
		files []BundleFile,
		targetDir string,
		strategy entities.BundleMergeStrategy,
	) (*BundleMergeReport, error)
}

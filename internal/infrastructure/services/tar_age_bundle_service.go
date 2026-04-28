package services

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/hmac"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/hkdf"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// bundleHashLength is the number of hex characters kept from
// HMAC-SHA256(name). Sixteen is enough for ~10^19 namespace entries —
// vastly more than any device will ever have project directories —
// while keeping the .age filename short enough to display.
const bundleHashLength = 16

// bundleNameKeyInfoV1 namespaces the HKDF derivation that produces the
// per-repo HMAC key used for bundle filename hashing. Versioning here
// lets a future migration command derive a v2 key from the same
// identity material without colliding with the v1 names.
const bundleNameKeyInfoV1 = "aisync-bundle-name-v1"

// bundleNameKeyLength is the byte length of the HMAC-SHA256 key derived
// from the age identity. 32 bytes matches SHA-256's output size and
// provides 256-bit key material for the derived HMAC key.
const bundleNameKeyLength = 32

// maxBundleManifestSize bounds how many bytes Extract is willing to read
// for the manifest entry. The manifest is a small JSON document; an
// outsized one indicates a corrupt or malicious archive and is rejected
// instead of being trusted.
const maxBundleManifestSize = 1 << 20 // 1 MiB

// bundleManifestFileMode is the in-tar mode bits stamped on the manifest
// header. Owner read+write only — the on-disk file is recreated on
// extract so this just keeps tar listings tidy.
const bundleManifestFileMode = 0o600

// bundlePermissionMask masks tar-header mode bits down to the standard
// 12-bit Unix permission triplet (set-uid + set-gid + sticky + rwx).
// Anything above that is suspicious and would also trip a narrowing
// cast warning, so we strip it before storing.
const bundlePermissionMask = 0o7777

// bundleSizeBuckets is the set of target padded gzipped-tarball
// plaintext sizes (in bytes) for size padding. Each gzipped tarball
// is padded with cryptographic random bytes up to the smallest
// bucket >= the gzipped size before age encryption, so the final
// ciphertext size is the selected bucket plus age overhead. Buckets
// are spaced as powers of 2 from 16 KiB to 128 MiB — fine enough
// that small projects don't waste much space, coarse enough that an
// attacker reading the public-clone view cannot distinguish two
// bundles within the same bucket. Anything larger than the top
// bucket is left unpadded (very rare for AI assistant project trees;
// the privacy benefit at that scale is marginal and the storage cost
// is large).
//
//nolint:gochecknoglobals // compile-time padding schedule, intentionally package-level
var bundleSizeBuckets = []int{
	16 << 10,  // 16 KiB
	32 << 10,  // 32 KiB
	64 << 10,  // 64 KiB
	128 << 10, // 128 KiB
	256 << 10, // 256 KiB
	512 << 10, // 512 KiB
	1 << 20,   // 1 MiB
	2 << 20,   // 2 MiB
	4 << 20,   // 4 MiB
	8 << 20,   // 8 MiB
	16 << 20,  // 16 MiB
	32 << 20,  // 32 MiB
	64 << 20,  // 64 MiB
	128 << 20, // 128 MiB
}

// TarAgeBundleService implements [repositories.BundleService] using a
// gzip-compressed tar archive as the on-the-wire format and age as the
// outer encryption layer. The manifest is the very first member of the
// tarball so Extract can decide whether to refuse the archive (e.g. an
// unsupported schema version) before processing any payload bytes.
type TarAgeBundleService struct {
	encryption repositories.EncryptionService
	now        func() time.Time

	// randSource is the io.Reader used to draw bytes for size-bucket
	// padding. Production code uses [crypto/rand.Reader]; tests can
	// inject a deterministic reader (e.g. [bytes.Reader] over a fixed
	// payload) to assert padding behaviour without depending on
	// crypto/rand output.
	randSource io.Reader

	// nameKeysMu guards nameKeys.
	nameKeysMu sync.RWMutex
	// nameKeys caches the HMAC key derived from each identity file path
	// so HashName does not re-read and re-HKDF the identity on every
	// call. Keyed by identityPath, value is the 32-byte HMAC key.
	nameKeys map[string][]byte
}

// NewTarAgeBundleService builds a TarAgeBundleService that delegates the
// outer encryption layer to the provided encryption service. The
// randomness source for size-bucket padding defaults to
// [crypto/rand.Reader]; tests use [NewTarAgeBundleServiceWithRand] to
// inject a deterministic reader.
func NewTarAgeBundleService(encryption repositories.EncryptionService) *TarAgeBundleService {
	return NewTarAgeBundleServiceWithRand(encryption, crand.Reader)
}

// NewTarAgeBundleServiceWithRand is like [NewTarAgeBundleService] but
// accepts an explicit random source. Production callers should use
// [NewTarAgeBundleService]; this constructor exists so tests can pass
// a deterministic reader and assert padding bytes directly without
// relying on crypto/rand output.
func NewTarAgeBundleServiceWithRand(
	encryption repositories.EncryptionService,
	randSource io.Reader,
) *TarAgeBundleService {
	return &TarAgeBundleService{
		encryption: encryption,
		now:        time.Now,
		randSource: randSource,
		nameKeys:   make(map[string][]byte),
	}
}

// HashName implements [repositories.BundleService] using HMAC-SHA256
// keyed by an HKDF derivation from the age identity at identityPath.
// Without that identity an attacker cannot compute or verify a bundle
// filename for a guessed source name — closing the confirmation oracle
// that existed when filenames were `sha256(name)[:16]`.
func (s *TarAgeBundleService) HashName(sourceDirName, identityPath string) (string, error) {
	if identityPath == "" {
		return "", errors.New("bundle: HashName requires an identity path to derive the per-repo HMAC key")
	}
	key, err := s.loadOrDeriveNameKey(identityPath)
	if err != nil {
		return "", fmt.Errorf("derive bundle name key: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(sourceDirName))
	return hex.EncodeToString(mac.Sum(nil))[:bundleHashLength], nil
}

// loadOrDeriveNameKey returns the cached HMAC key for identityPath, or
// derives one via HKDF-SHA256 from the AGE-SECRET-KEY entries inside
// the identity file and caches it. The cache is keyed by the absolute,
// cleaned identity path so callers passing the same file via different
// spellings (e.g. `~/.config/aisync/key.txt` vs the expanded
// `/home/u/.config/aisync/key.txt`, or with redundant `./` segments)
// hit the same entry.
func (s *TarAgeBundleService) loadOrDeriveNameKey(identityPath string) ([]byte, error) {
	cacheKey, err := filepath.Abs(identityPath)
	if err != nil {
		return nil, fmt.Errorf("resolve absolute identity path: %w", err)
	}
	cacheKey = filepath.Clean(cacheKey)

	s.nameKeysMu.RLock()
	if cached, ok := s.nameKeys[cacheKey]; ok {
		s.nameKeysMu.RUnlock()
		return cached, nil
	}
	s.nameKeysMu.RUnlock()

	derived, err := deriveBundleNameKey(cacheKey)
	if err != nil {
		return nil, err
	}

	s.nameKeysMu.Lock()
	defer s.nameKeysMu.Unlock()
	if cached, ok := s.nameKeys[cacheKey]; ok {
		// Another goroutine raced ahead; reuse its result.
		return cached, nil
	}
	s.nameKeys[cacheKey] = derived
	return derived, nil
}

// deriveBundleNameKey reads the age identity file, gathers every line
// that begins with `AGE-SECRET-KEY-` as input keying material, and
// HKDF-extracts a 32-byte HMAC key namespaced by [bundleNameKeyInfoV1].
//
// Using the literal Bech32-encoded secret key string as IKM avoids
// pulling a Bech32 decoder into the bundle service while still binding
// the derivation to the secret material — anyone who can read the
// identity file can already decrypt every bundle, so deriving an HMAC
// key from it adds no new exposure.
func deriveBundleNameKey(identityPath string) ([]byte, error) {
	file, err := os.Open(identityPath)
	if err != nil {
		return nil, fmt.Errorf("open identity file %s: %w", identityPath, err)
	}
	defer func() { _ = file.Close() }()

	var ikm []byte
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "AGE-SECRET-KEY-") {
			ikm = append(ikm, []byte(line)...)
			ikm = append(ikm, '\n')
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return nil, fmt.Errorf("scan identity file %s: %w", identityPath, scanErr)
	}
	if len(ikm) == 0 {
		return nil, fmt.Errorf("no AGE-SECRET-KEY entry found in identity file %s", identityPath)
	}

	reader := hkdf.New(sha256.New, ikm, nil, []byte(bundleNameKeyInfoV1))
	key := make([]byte, bundleNameKeyLength)
	if _, readErr := io.ReadFull(reader, key); readErr != nil {
		return nil, fmt.Errorf("hkdf expand: %w", readErr)
	}
	return key, nil
}

// Bundle implements [repositories.BundleService].
func (s *TarAgeBundleService) Bundle(
	sourceDir, originalName string,
	recipients []string,
) ([]byte, *entities.BundleManifest, error) {
	if originalName == "" {
		return nil, nil, errors.New("bundle: originalName must not be empty")
	}
	if len(recipients) == 0 {
		return nil, nil, errors.New("bundle: at least one age recipient required")
	}

	files, err := s.collectSourceFiles(sourceDir)
	if err != nil {
		return nil, nil, fmt.Errorf("bundle: scan %s: %w", sourceDir, err)
	}

	manifest := &entities.BundleManifest{
		OriginalName: originalName,
		CreatedAt:    s.now().UTC(),
		FileCount:    len(files),
		SchemaVer:    entities.CurrentBundleSchemaVersion,
	}

	tarball, err := s.writeTarball(manifest, files)
	if err != nil {
		return nil, nil, fmt.Errorf("bundle: write tarball: %w", err)
	}

	padded, err := s.padToSizeBucket(tarball)
	if err != nil {
		return nil, nil, fmt.Errorf("bundle: pad tarball: %w", err)
	}

	ciphertext, err := s.encryption.Encrypt(padded, recipients)
	if err != nil {
		return nil, nil, fmt.Errorf("bundle: age encrypt: %w", err)
	}
	return ciphertext, manifest, nil
}

// pickSizeBucket returns the smallest [bundleSizeBuckets] entry >=
// size, or size unchanged when size exceeds the top bucket. Returning
// size unchanged for very large bundles avoids quadrupling repo space
// for the rare giant-project case; the privacy benefit at that scale
// is marginal because there are usually few such bundles to confuse
// against each other.
func pickSizeBucket(size int) int {
	for _, bucket := range bundleSizeBuckets {
		if bucket >= size {
			return bucket
		}
	}
	return size
}

// padToSizeBucket appends cryptographic-random bytes (drawn from
// s.randSource) to gzipped so its final length equals the smallest
// bucket >= len(gzipped). The padding goes AFTER the gzip end marker
// so a gzip reader with `gz.Multistream(false)` will ignore it during
// extract. Random bytes do not start with the gzip magic number with
// any meaningful probability, so even a Multistream(true) reader would
// fail loudly rather than silently mis-decode a fake second stream.
//
// The output slice is allocated once at the bucket size and the gzipped
// prefix copied in directly, so peak memory is one bucket-sized
// allocation per call instead of two.
func (s *TarAgeBundleService) padToSizeBucket(gzipped []byte) ([]byte, error) {
	target := pickSizeBucket(len(gzipped))
	if target <= len(gzipped) {
		return gzipped, nil
	}
	out := make([]byte, target)
	copy(out, gzipped)
	if _, err := io.ReadFull(s.randSource, out[len(gzipped):]); err != nil {
		return nil, fmt.Errorf("read random padding: %w", err)
	}
	return out, nil
}

// Extract implements [repositories.BundleService].
func (s *TarAgeBundleService) Extract(
	ciphertext []byte,
	identityPath string,
) (*entities.BundleManifest, []repositories.BundleFile, error) {
	plaintext, err := s.encryption.Decrypt(ciphertext, identityPath)
	if err != nil {
		return nil, nil, fmt.Errorf("extract: age decrypt: %w", err)
	}

	gz, err := gzip.NewReader(bytes.NewReader(plaintext))
	if err != nil {
		return nil, nil, fmt.Errorf("extract: gzip: %w", err)
	}
	// Multistream(false) so the gzip reader stops at the end of the
	// first stream and ignores any trailing bytes. Bundles produced by
	// [TarAgeBundleService.Bundle] append cryptographic-random padding
	// after the gzip end marker (size-bucket privacy padding) which
	// would otherwise cause Multistream(true) to fail on the next
	// "stream" attempt.
	gz.Multistream(false)
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	manifest, files, err := s.readTarball(tr)
	if err != nil {
		return nil, nil, err
	}
	if manifest == nil {
		return nil, nil, errors.New("extract: manifest entry missing")
	}
	if manifest.SchemaVer > entities.CurrentBundleSchemaVersion {
		return nil, nil, fmt.Errorf(
			"extract: bundle schema version %d is newer than supported %d — upgrade aisync",
			manifest.SchemaVer, entities.CurrentBundleSchemaVersion,
		)
	}
	return manifest, files, nil
}

// MergeIntoLocal implements [repositories.BundleService].
func (s *TarAgeBundleService) MergeIntoLocal(
	files []repositories.BundleFile,
	targetDir string,
	strategy entities.BundleMergeStrategy,
) (*repositories.BundleMergeReport, error) {
	if strategy == "" {
		strategy = entities.BundleMergeMTime
	}
	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		return nil, fmt.Errorf("merge: create %s: %w", targetDir, err)
	}

	report := &repositories.BundleMergeReport{}
	for _, f := range files {
		dest := filepath.Join(targetDir, filepath.FromSlash(f.RelativePath))
		if !strings.HasPrefix(filepath.Clean(dest)+string(filepath.Separator),
			filepath.Clean(targetDir)+string(filepath.Separator)) {
			return nil, fmt.Errorf("merge: refused traversal path %q", f.RelativePath)
		}

		stat, statErr := os.Stat(dest)
		switch {
		case os.IsNotExist(statErr):
			if err := s.writeMergedFile(dest, f); err != nil {
				return nil, err
			}
			report.Added = append(report.Added, f.RelativePath)
		case statErr != nil:
			return nil, fmt.Errorf("merge: stat %s: %w", dest, statErr)
		case strategy == entities.BundleMergeReplace:
			if err := s.writeMergedFile(dest, f); err != nil {
				return nil, err
			}
			report.Overwrote = append(report.Overwrote, f.RelativePath)
		case f.ModTime > stat.ModTime().Unix():
			if err := s.writeMergedFile(dest, f); err != nil {
				return nil, err
			}
			report.Overwrote = append(report.Overwrote, f.RelativePath)
		default:
			report.SkippedNew = append(report.SkippedNew, f.RelativePath)
		}
	}
	return report, nil
}

// collectSourceFiles walks sourceDir and returns every regular file as a
// [repositories.BundleFile]. Symlinks are skipped to avoid following them
// outside the project tree; non-regular files (sockets, devices) are
// skipped because they are meaningless to sync.
func (s *TarAgeBundleService) collectSourceFiles(sourceDir string) ([]repositories.BundleFile, error) {
	var files []repositories.BundleFile
	walkErr := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, relErr := filepath.Rel(sourceDir, path)
		if relErr != nil {
			return relErr
		}
		//nolint:gosec // walking only paths under a user-owned tool dir
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		files = append(files, repositories.BundleFile{
			RelativePath: filepath.ToSlash(rel),
			Content:      content,
			ModTime:      info.ModTime().Unix(),
			Mode:         uint32(info.Mode().Perm()),
		})
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	// Deterministic ordering keeps bundle bytes stable when nothing
	// changed, which is exactly what git delta detection needs.
	sort.Slice(files, func(i, j int) bool {
		return files[i].RelativePath < files[j].RelativePath
	})
	return files, nil
}

// writeTarball assembles the gzip-compressed tar archive carried inside
// the bundle ciphertext. The manifest is always the first entry so a
// schema check can short-circuit the rest of the read.
func (s *TarAgeBundleService) writeTarball(
	manifest *entities.BundleManifest,
	files []repositories.BundleFile,
) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	manifestBytes, marshalErr := json.Marshal(manifest)
	if marshalErr != nil {
		return nil, marshalErr
	}
	manifestHeader := &tar.Header{
		Name:    entities.BundleManifestFileName,
		Mode:    bundleManifestFileMode,
		Size:    int64(len(manifestBytes)),
		ModTime: manifest.CreatedAt,
	}
	if writeErr := tw.WriteHeader(manifestHeader); writeErr != nil {
		return nil, writeErr
	}
	if _, writeErr := tw.Write(manifestBytes); writeErr != nil {
		return nil, writeErr
	}

	for _, f := range files {
		hdr := &tar.Header{
			Name:    f.RelativePath,
			Mode:    int64(f.Mode),
			Size:    int64(len(f.Content)),
			ModTime: time.Unix(f.ModTime, 0).UTC(),
		}
		if writeErr := tw.WriteHeader(hdr); writeErr != nil {
			return nil, writeErr
		}
		if _, writeErr := tw.Write(f.Content); writeErr != nil {
			return nil, writeErr
		}
	}

	if closeErr := tw.Close(); closeErr != nil {
		return nil, closeErr
	}
	if closeErr := gz.Close(); closeErr != nil {
		return nil, closeErr
	}
	return buf.Bytes(), nil
}

// readTarball pulls the manifest and every regular-file member out of the
// tar reader, enforcing the manifest size limit and skipping directory
// entries (we recreate directories implicitly when writing files).
func (s *TarAgeBundleService) readTarball(
	tr *tar.Reader,
) (*entities.BundleManifest, []repositories.BundleFile, error) {
	var manifest *entities.BundleManifest
	var files []repositories.BundleFile

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("extract: tar: %w", err)
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		if hdr.Name == entities.BundleManifestFileName {
			m, parseErr := readManifestEntry(tr, hdr)
			if parseErr != nil {
				return nil, nil, parseErr
			}
			manifest = m
			continue
		}
		if manifest == nil {
			return nil, nil, fmt.Errorf(
				"extract: manifest must be the first non-directory entry, found %s before %s",
				hdr.Name,
				entities.BundleManifestFileName,
			)
		}
		file, readErr := readFileEntry(tr, hdr)
		if readErr != nil {
			return nil, nil, readErr
		}
		files = append(files, file)
	}
	return manifest, files, nil
}

// readManifestEntry parses one manifest tar member, enforcing the size
// limit so a hostile archive cannot force us to allocate megabytes for
// what should be a small JSON document.
func readManifestEntry(tr *tar.Reader, hdr *tar.Header) (*entities.BundleManifest, error) {
	if hdr.Size > maxBundleManifestSize {
		return nil, fmt.Errorf("extract: manifest too large (%d bytes)", hdr.Size)
	}
	data, readErr := io.ReadAll(io.LimitReader(tr, maxBundleManifestSize))
	if readErr != nil {
		return nil, fmt.Errorf("extract: read manifest: %w", readErr)
	}
	var m entities.BundleManifest
	if parseErr := json.Unmarshal(data, &m); parseErr != nil {
		return nil, fmt.Errorf("extract: parse manifest: %w", parseErr)
	}
	return &m, nil
}

// readFileEntry materialises one payload tar member into a BundleFile,
// masking the on-disk mode before the narrowing cast so a malformed
// header with high bits set cannot trip gosec G115 nor inject any extra
// mode bits we did not put there ourselves on bundle.
func readFileEntry(tr *tar.Reader, hdr *tar.Header) (repositories.BundleFile, error) {
	data, err := io.ReadAll(tr)
	if err != nil {
		return repositories.BundleFile{}, fmt.Errorf("extract: read %s: %w", hdr.Name, err)
	}
	return repositories.BundleFile{
		RelativePath: filepath.ToSlash(hdr.Name),
		Content:      data,
		ModTime:      hdr.ModTime.Unix(),
		Mode:         uint32(hdr.Mode & bundlePermissionMask),
	}, nil
}

// writeMergedFile materialises one bundle file inside the local target
// directory, creating any missing intermediate directories with 0700
// permissions and stamping the final mtime so subsequent merges pick the
// correct winner.
func (s *TarAgeBundleService) writeMergedFile(dest string, f repositories.BundleFile) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return fmt.Errorf("merge: mkdir %s: %w", filepath.Dir(dest), err)
	}
	mode := os.FileMode(f.Mode).Perm()
	if mode == 0 {
		mode = 0o600
	}
	if err := os.WriteFile(dest, f.Content, mode); err != nil {
		return fmt.Errorf("merge: write %s: %w", dest, err)
	}
	mtime := time.Unix(f.ModTime, 0)
	if err := os.Chtimes(dest, mtime, mtime); err != nil {
		return fmt.Errorf("merge: chtimes %s: %w", dest, err)
	}
	return nil
}

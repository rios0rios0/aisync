package services

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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
	"time"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// bundleHashLength is the number of hex characters kept from sha256(name).
// Sixteen is enough for ~10^19 namespace entries — vastly more than any
// device will ever have project directories — while keeping the .age
// filename short enough to display.
const bundleHashLength = 16

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

// TarAgeBundleService implements [repositories.BundleService] using a
// gzip-compressed tar archive as the on-the-wire format and age as the
// outer encryption layer. The manifest is the very first member of the
// tarball so Extract can decide whether to refuse the archive (e.g. an
// unsupported schema version) before processing any payload bytes.
type TarAgeBundleService struct {
	encryption repositories.EncryptionService
	now        func() time.Time
}

// NewTarAgeBundleService builds a TarAgeBundleService that delegates the
// outer encryption layer to the provided encryption service.
func NewTarAgeBundleService(encryption repositories.EncryptionService) *TarAgeBundleService {
	return &TarAgeBundleService{
		encryption: encryption,
		now:        time.Now,
	}
}

// HashName implements [repositories.BundleService].
func (s *TarAgeBundleService) HashName(sourceDirName string) string {
	sum := sha256.Sum256([]byte(sourceDirName))
	return hex.EncodeToString(sum[:])[:bundleHashLength]
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

	ciphertext, err := s.encryption.Encrypt(tarball, recipients)
	if err != nil {
		return nil, nil, fmt.Errorf("bundle: age encrypt: %w", err)
	}
	return ciphertext, manifest, nil
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

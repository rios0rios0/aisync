//go:build unit

package services_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
	"github.com/rios0rios0/aisync/internal/infrastructure/services"
)

// passthroughEncryption is an identity encryption that returns plaintext
// unchanged. It lets the bundle round-trip tests exercise the full
// tar/gzip/manifest path without coupling them to age and without
// pulling a real X25519 key into the test fixtures.
type passthroughEncryption struct{}

func (passthroughEncryption) GenerateKey(string) (string, error)         { return "", nil }
func (passthroughEncryption) ImportKey(string, string) error             { return nil }
func (passthroughEncryption) ExportPublicKey(string) (string, error)     { return "", nil }
func (passthroughEncryption) Encrypt(b []byte, _ []string) ([]byte, error) { return b, nil }
func (passthroughEncryption) Decrypt(b []byte, _ string) ([]byte, error)   { return b, nil }

// writeFakeIdentity writes a synthetic age identity file containing one
// AGE-SECRET-KEY line with a deterministic but realistic-looking
// payload. The tests don't decrypt anything so the value just has to
// thread through the HKDF input and is otherwise opaque.
func writeFakeIdentity(t *testing.T, payload string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "key.txt")
	contents := "# created: 2026-01-01T00:00:00Z\n" +
		"# public key: age1publictest\n" +
		"AGE-SECRET-KEY-1" + payload + "\n"
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	return path
}

func TestTarAgeBundleService_HashName(t *testing.T) {
	t.Parallel()

	t.Run("should produce deterministic 16-hex-character hash for the same input and identity", func(t *testing.T) {
		// given
		service := services.NewTarAgeBundleService(passthroughEncryption{})
		identity := writeFakeIdentity(t, "DETERMINISTICKEYAAAA")
		name := "-home-user-Development-Acme-Project"

		// when
		first, err1 := service.HashName(name, identity)
		second, err2 := service.HashName(name, identity)

		// then
		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.Equal(t, first, second)
		assert.Len(t, first, 16)
	})

	t.Run("should produce different hashes for different inputs", func(t *testing.T) {
		// given
		service := services.NewTarAgeBundleService(passthroughEncryption{})
		identity := writeFakeIdentity(t, "DETERMINISTICKEYBBBB")

		// when
		a, errA := service.HashName("project-a", identity)
		b, errB := service.HashName("project-b", identity)

		// then — collisions in 16 hex chars are astronomically unlikely
		require.NoError(t, errA)
		require.NoError(t, errB)
		assert.NotEqual(t, a, b)
	})

	t.Run("should produce different hashes for the same input under different identities", func(t *testing.T) {
		// given — two devices with different age identities must NOT
		// collide on bundle filenames; otherwise an attacker who knows
		// one device's identity could derive another device's hashes.
		service := services.NewTarAgeBundleService(passthroughEncryption{})
		identity1 := writeFakeIdentity(t, "FIRSTIDENTITYPAYLOAD")
		identity2 := writeFakeIdentity(t, "SECONDIDENTITYPAYLOA")
		name := "-home-user-Development-shared-project"

		// when
		hash1, err1 := service.HashName(name, identity1)
		hash2, err2 := service.HashName(name, identity2)

		// then
		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, hash1, hash2,
			"different identity files MUST produce different hashes — that is what closes the SHA-256 oracle")
	})

	t.Run("should differ from the plain sha256 oracle for this fixture", func(t *testing.T) {
		// given — exercises the migration property for one concrete
		// fixture: with truncation to 16 hex chars (64 bits) a collision
		// between the HMAC value and the legacy `sha256(name)[:16]` is
		// theoretically possible (~1 in 2^64) but vanishingly unlikely.
		// Asserting they differ for this specific input is enough to
		// catch a bug where HMAC accidentally degenerates to a plain
		// SHA-256 (e.g. by passing an all-zero key) — the actual
		// security argument is that the HMAC is unforgeable without
		// the per-repo key, not that any two specific values differ.
		service := services.NewTarAgeBundleService(passthroughEncryption{})
		identity := writeFakeIdentity(t, "ORACLECHECKKEYPAYLOA")
		name := "-home-user-Development-some-project"

		// when
		hmacHash, err := service.HashName(name, identity)

		// then
		require.NoError(t, err)
		legacy := sha256.Sum256([]byte(name))
		legacyHex := hex.EncodeToString(legacy[:])[:16]
		assert.NotEqual(t, legacyHex, hmacHash,
			"HMAC value should differ from the legacy sha256 oracle for this fixture (collision is theoretically possible at ~1 in 2^64 but would indicate a key-derivation bug)")
	})

	t.Run("should fail when identityPath is empty", func(t *testing.T) {
		// given
		service := services.NewTarAgeBundleService(passthroughEncryption{})

		// when
		_, err := service.HashName("project-x", "")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "identity path")
	})

	t.Run("should fail when identity file has no AGE-SECRET-KEY line", func(t *testing.T) {
		// given
		service := services.NewTarAgeBundleService(passthroughEncryption{})
		path := filepath.Join(t.TempDir(), "broken.txt")
		require.NoError(t, os.WriteFile(path, []byte("# only a comment\n"), 0o600))

		// when
		_, err := service.HashName("project-x", path)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AGE-SECRET-KEY")
	})
}

func TestTarAgeBundleService_BundleAndExtract_RoundTrip(t *testing.T) {
	t.Parallel()

	// given
	service := services.NewTarAgeBundleService(passthroughEncryption{})
	src := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(src, "nested"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(src, "MEMORY.md"), []byte("# memory\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(src, "nested", "note.md"), []byte("note"), 0o600))

	// when
	ciphertext, manifest, err := service.Bundle(src, "-home-user-aisync", []string{"age1xyz"})
	require.NoError(t, err)
	require.NotNil(t, manifest)
	require.NotEmpty(t, ciphertext)

	gotManifest, files, err := service.Extract(ciphertext, "")
	require.NoError(t, err)

	// then — manifest survives the round-trip and every file is recovered
	assert.Equal(t, "-home-user-aisync", gotManifest.OriginalName)
	assert.Equal(t, 2, gotManifest.FileCount)
	assert.Equal(t, entities.CurrentBundleSchemaVersion, gotManifest.SchemaVer)

	paths := map[string][]byte{}
	for _, f := range files {
		paths[f.RelativePath] = f.Content
	}
	assert.Equal(t, []byte("# memory\n"), paths["MEMORY.md"])
	assert.Equal(t, []byte("note"), paths["nested/note.md"])
}

func TestTarAgeBundleService_Bundle_RejectsEmptyRecipients(t *testing.T) {
	t.Parallel()

	// given
	service := services.NewTarAgeBundleService(passthroughEncryption{})
	src := t.TempDir()

	// when
	_, _, err := service.Bundle(src, "any", nil)

	// then
	require.Error(t, err)
	assert.Contains(t, err.Error(), "recipient")
}

func TestTarAgeBundleService_MergeIntoLocal(t *testing.T) {
	t.Parallel()

	t.Run("should add files that are missing locally", func(t *testing.T) {
		// given
		service := services.NewTarAgeBundleService(passthroughEncryption{})
		target := t.TempDir()
		now := time.Now().Unix()
		files := []repositories.BundleFile{
			{RelativePath: "new.md", Content: []byte("hello"), ModTime: now, Mode: 0o600},
		}

		// when
		report, err := service.MergeIntoLocal(files, target, entities.BundleMergeMTime)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"new.md"}, report.Added)
		assert.Empty(t, report.Overwrote)
		got, _ := os.ReadFile(filepath.Join(target, "new.md"))
		assert.Equal(t, []byte("hello"), got)
	})

	t.Run("should overwrite local file when bundle copy is newer", func(t *testing.T) {
		// given
		service := services.NewTarAgeBundleService(passthroughEncryption{})
		target := t.TempDir()
		dest := filepath.Join(target, "MEMORY.md")
		require.NoError(t, os.WriteFile(dest, []byte("old"), 0o600))
		oldTime := time.Now().Add(-2 * time.Hour)
		require.NoError(t, os.Chtimes(dest, oldTime, oldTime))

		newer := time.Now().Unix()
		files := []repositories.BundleFile{
			{RelativePath: "MEMORY.md", Content: []byte("new"), ModTime: newer, Mode: 0o600},
		}

		// when
		report, err := service.MergeIntoLocal(files, target, entities.BundleMergeMTime)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"MEMORY.md"}, report.Overwrote)
		got, _ := os.ReadFile(dest)
		assert.Equal(t, []byte("new"), got)
	})

	t.Run("should preserve local file when local is newer", func(t *testing.T) {
		// given
		service := services.NewTarAgeBundleService(passthroughEncryption{})
		target := t.TempDir()
		dest := filepath.Join(target, "MEMORY.md")
		require.NoError(t, os.WriteFile(dest, []byte("local-newer"), 0o600))
		// Local mtime defaults to now — leave it as-is.

		older := time.Now().Add(-2 * time.Hour).Unix()
		files := []repositories.BundleFile{
			{RelativePath: "MEMORY.md", Content: []byte("bundle-older"), ModTime: older, Mode: 0o600},
		}

		// when
		report, err := service.MergeIntoLocal(files, target, entities.BundleMergeMTime)

		// then — the local edit is preserved because the bundle copy is older
		require.NoError(t, err)
		assert.Empty(t, report.Overwrote)
		assert.Equal(t, []string{"MEMORY.md"}, report.SkippedNew)
		got, _ := os.ReadFile(dest)
		assert.Equal(t, []byte("local-newer"), got)
	})

	t.Run("should overwrite unconditionally with replace strategy", func(t *testing.T) {
		// given
		service := services.NewTarAgeBundleService(passthroughEncryption{})
		target := t.TempDir()
		dest := filepath.Join(target, "MEMORY.md")
		require.NoError(t, os.WriteFile(dest, []byte("local"), 0o600))

		older := time.Now().Add(-2 * time.Hour).Unix()
		files := []repositories.BundleFile{
			{RelativePath: "MEMORY.md", Content: []byte("bundle"), ModTime: older, Mode: 0o600},
		}

		// when
		report, err := service.MergeIntoLocal(files, target, entities.BundleMergeReplace)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"MEMORY.md"}, report.Overwrote)
		got, _ := os.ReadFile(dest)
		assert.Equal(t, []byte("bundle"), got)
	})

	t.Run("should refuse path-traversal entries", func(t *testing.T) {
		// given
		service := services.NewTarAgeBundleService(passthroughEncryption{})
		target := t.TempDir()
		now := time.Now().Unix()
		files := []repositories.BundleFile{
			{RelativePath: "../escape.md", Content: []byte("pwn"), ModTime: now},
		}

		// when
		_, err := service.MergeIntoLocal(files, target, entities.BundleMergeMTime)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "traversal")
	})
}

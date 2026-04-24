//go:build unit

package services_test

import (
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

func TestTarAgeBundleService_HashName(t *testing.T) {
	t.Parallel()

	service := services.NewTarAgeBundleService(passthroughEncryption{})

	t.Run("should produce deterministic 16-hex-character hash for the same input", func(t *testing.T) {
		// given
		name := "-home-user-Development-Acme-Project"

		// when
		first := service.HashName(name)
		second := service.HashName(name)

		// then
		assert.Equal(t, first, second)
		assert.Len(t, first, 16)
	})

	t.Run("should produce different hashes for different inputs", func(t *testing.T) {
		// given/when
		a := service.HashName("project-a")
		b := service.HashName("project-b")

		// then — collisions in 16 hex chars are astronomically unlikely
		assert.NotEqual(t, a, b)
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

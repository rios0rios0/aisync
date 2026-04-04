//go:build unit

package entities_test

import (
	"testing"
	"github.com/rios0rios0/aisync/internal/domain/entities"

	"github.com/stretchr/testify/assert"
)

func TestNewManifest_CreatesWithCorrectFields(t *testing.T) {
	// given
	version := "1.2.3"
	device := "laptop"

	// when
	m := entities.NewManifest(version, device)

	// then
	assert.Equal(t, "aisync", m.ManagedBy)
	assert.Equal(t, version, m.Version)
	assert.Equal(t, device, m.Device)
	assert.NotNil(t, m.Files)
	assert.Len(t, m.Files, 0)
	assert.False(t, m.LastSync.IsZero(), "LastSync should be set to a non-zero time")
}

func TestNewManifest_FilesMapStartsEmpty(t *testing.T) {
	// given / when
	m := entities.NewManifest("0.0.1", "workstation")

	// then
	assert.Empty(t, m.Files)
}

func TestManifest_SetFile_AddsFileToManifest(t *testing.T) {
	// given
	m := entities.NewManifest("1.0.0", "dev")

	// when
	m.SetFile("rules/test.md", "guide", "shared", "abc123")

	// then
	assert.Len(t, m.Files, 1)
	f, ok := m.Files["rules/test.md"]
	assert.True(t, ok)
	assert.Equal(t, "guide", f.Source)
	assert.Equal(t, "shared", f.Namespace)
	assert.Equal(t, "abc123", f.Checksum)
}

func TestManifest_SetFile_OverwritesExistingFile(t *testing.T) {
	// given
	m := entities.NewManifest("1.0.0", "dev")
	m.SetFile("rules/test.md", "guide", "shared", "abc123")

	// when
	m.SetFile("rules/test.md", "guide-v2", "personal", "def456")

	// then
	assert.Len(t, m.Files, 1)
	f := m.Files["rules/test.md"]
	assert.Equal(t, "guide-v2", f.Source)
	assert.Equal(t, "personal", f.Namespace)
	assert.Equal(t, "def456", f.Checksum)
}

func TestManifest_SetFile_MultipleFiles(t *testing.T) {
	// given
	m := entities.NewManifest("1.0.0", "dev")

	// when
	m.SetFile("rules/a.md", "src1", "shared", "aaa")
	m.SetFile("rules/b.md", "src2", "personal", "bbb")
	m.SetFile("agents/c.md", "src1", "shared", "ccc")

	// then
	assert.Len(t, m.Files, 3)
}

func TestManifest_RemoveFile_DeletesExistingFile(t *testing.T) {
	// given
	m := entities.NewManifest("1.0.0", "dev")
	m.SetFile("rules/test.md", "guide", "shared", "abc123")
	m.SetFile("rules/other.md", "guide", "shared", "def456")

	// when
	m.RemoveFile("rules/test.md")

	// then
	assert.Len(t, m.Files, 1)
	_, ok := m.Files["rules/test.md"]
	assert.False(t, ok, "removed file should not exist in manifest")
	_, ok = m.Files["rules/other.md"]
	assert.True(t, ok, "other file should still exist")
}

func TestManifest_RemoveFile_NonExistentFileIsNoOp(t *testing.T) {
	// given
	m := entities.NewManifest("1.0.0", "dev")
	m.SetFile("rules/test.md", "guide", "shared", "abc123")

	// when
	m.RemoveFile("nonexistent.md")

	// then
	assert.Len(t, m.Files, 1)
}

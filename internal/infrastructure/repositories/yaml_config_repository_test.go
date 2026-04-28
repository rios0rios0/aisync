//go:build unit

package repositories_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	repositories "github.com/rios0rios0/aisync/internal/infrastructure/repositories"
)

func TestYAMLConfigRepository_SaveThenLoad(t *testing.T) {
	// given
	repo := repositories.NewYAMLConfigRepository()
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := &entities.Config{
		Sync: entities.SyncConfig{
			Remote:       "git@github.com:user/repo.git",
			Branch:       "main",
			AutoPush:     true,
			Debounce:     "5s",
			CommitPrefix: "aisync:",
		},
		Encryption: entities.EncryptionConfig{
			Identity:   "~/.config/aisync/identity.txt",
			Recipients: []string{"age1recipient1", "age1recipient2"},
		},
		Tools: map[string]entities.Tool{
			"claude": {Path: "~/.claude", Enabled: true},
			"cursor": {Path: "~/.cursor", Enabled: false},
		},
		Sources: []entities.Source{
			{
				Name:   "guide",
				Repo:   "rios0rios0/guide",
				Branch: "generated",
				Mappings: []entities.SourceMapping{
					{Source: "claude/rules", Target: "rules"},
				},
			},
		},
		Watch: entities.WatchConfig{
			PollingInterval: "10s",
			IgnoredPatterns: []string{".git", "*.tmp"},
		},
		HooksExclude: []entities.HooksExcludeEntry{
			{Event: "pre-commit", Matcher: "*.md", Command: "lint"},
		},
	}

	// when
	err := repo.Save(path, original)
	assert.NoError(t, err)
	loaded, err := repo.Load(path)

	// then
	assert.NoError(t, err)
	assert.Equal(t, original.Sync, loaded.Sync)
	assert.Equal(t, original.Encryption, loaded.Encryption)
	assert.Equal(t, original.Tools, loaded.Tools)
	assert.Equal(t, original.Sources, loaded.Sources)
	assert.Equal(t, original.Watch, loaded.Watch)
	assert.Equal(t, original.HooksExclude, loaded.HooksExclude)
}

func TestYAMLConfigRepository_Load_InvalidYAML(t *testing.T) {
	// given
	repo := repositories.NewYAMLConfigRepository()
	path := filepath.Join(t.TempDir(), "config.yaml")
	err := os.WriteFile(path, []byte(":\tinvalid:\n\t- [yaml broken"), 0600)
	assert.NoError(t, err)

	// when
	config, err := repo.Load(path)

	// then
	assert.Error(t, err)
	assert.Nil(t, config)
	assert.Contains(t, err.Error(), "failed to parse config file")
}

func TestYAMLConfigRepository_Load_MissingFile(t *testing.T) {
	// given
	repo := repositories.NewYAMLConfigRepository()
	path := filepath.Join(t.TempDir(), "nonexistent.yaml")

	// when
	config, err := repo.Load(path)

	// then
	assert.Error(t, err)
	assert.Nil(t, config)
	assert.Contains(t, err.Error(), "failed to read config file")
}

func TestYAMLConfigRepository_Exists_ExistingFile(t *testing.T) {
	// given
	repo := repositories.NewYAMLConfigRepository()
	path := filepath.Join(t.TempDir(), "config.yaml")
	err := os.WriteFile(path, []byte("sync:\n  branch: main\n"), 0600)
	assert.NoError(t, err)

	// when
	exists := repo.Exists(path)

	// then
	assert.True(t, exists)
}

func TestYAMLConfigRepository_Exists_MissingFile(t *testing.T) {
	// given
	repo := repositories.NewYAMLConfigRepository()
	path := filepath.Join(t.TempDir(), "nonexistent.yaml")

	// when
	exists := repo.Exists(path)

	// then
	assert.False(t, exists)
}

func TestYAMLConfigRepository_AllConfigFieldsSurviveRoundtrip(t *testing.T) {
	// given
	repo := repositories.NewYAMLConfigRepository()
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := &entities.Config{
		Sync: entities.SyncConfig{
			Remote:       "https://github.com/org/repo.git",
			Branch:       "custom",
			AutoPush:     false,
			Debounce:     "30s",
			CommitPrefix: "sync:",
		},
		Encryption: entities.EncryptionConfig{
			Identity:   "/path/to/identity",
			Recipients: []string{"recipient-a"},
		},
		Tools: map[string]entities.Tool{
			"copilot": {Path: "~/.github", Enabled: true},
		},
		Sources: []entities.Source{
			{
				Name:    "external",
				Repo:    "org/external",
				Branch:  "main",
				Ref:     "v1.0.0",
				Refresh: "24h",
				Mappings: []entities.SourceMapping{
					{Source: "src", Target: "dst"},
					{Source: "docs", Target: "notes"},
				},
			},
		},
		Watch: entities.WatchConfig{
			PollingInterval: "1m",
			IgnoredPatterns: []string{"node_modules", ".DS_Store"},
		},
		HooksExclude: []entities.HooksExcludeEntry{
			{Event: "post-merge", Matcher: "*.go", Command: "test"},
			{Event: "pre-push", Matcher: "*", Command: "lint"},
		},
	}

	// when
	err := repo.Save(path, original)
	assert.NoError(t, err)
	loaded, err := repo.Load(path)

	// then
	assert.NoError(t, err)
	assert.Equal(t, original.Sync.Remote, loaded.Sync.Remote)
	assert.Equal(t, original.Sync.Branch, loaded.Sync.Branch)
	assert.Equal(t, original.Sync.AutoPush, loaded.Sync.AutoPush)
	assert.Equal(t, original.Sync.Debounce, loaded.Sync.Debounce)
	assert.Equal(t, original.Sync.CommitPrefix, loaded.Sync.CommitPrefix)
	assert.Equal(t, original.Encryption.Identity, loaded.Encryption.Identity)
	assert.Equal(t, original.Encryption.Recipients, loaded.Encryption.Recipients)
	assert.Equal(t, original.Tools, loaded.Tools)
	assert.Equal(t, len(original.Sources), len(loaded.Sources))
	assert.Equal(t, original.Sources[0].Name, loaded.Sources[0].Name)
	assert.Equal(t, original.Sources[0].Ref, loaded.Sources[0].Ref)
	assert.Equal(t, original.Sources[0].Refresh, loaded.Sources[0].Refresh)
	assert.Equal(t, original.Sources[0].Mappings, loaded.Sources[0].Mappings)
	assert.Equal(t, original.Watch.PollingInterval, loaded.Watch.PollingInterval)
	assert.Equal(t, original.Watch.IgnoredPatterns, loaded.Watch.IgnoredPatterns)
	assert.Equal(t, len(original.HooksExclude), len(loaded.HooksExclude))
	assert.Equal(t, original.HooksExclude[0], loaded.HooksExclude[0])
	assert.Equal(t, original.HooksExclude[1], loaded.HooksExclude[1])
}

func TestYAMLConfigRepository_Save_ShouldCreateParentDirectories(t *testing.T) {
	// given
	repo := repositories.NewYAMLConfigRepository()
	nestedPath := filepath.Join(t.TempDir(), "nested", "deep", "config.yaml")
	config := &entities.Config{
		Sync: entities.SyncConfig{
			Remote: "https://github.com/test/repo.git",
			Branch: "main",
		},
		Tools: map[string]entities.Tool{
			"claude": {Path: "~/.claude", Enabled: true},
		},
	}

	// when
	// Save should fail because the function does not create parent directories
	// (the current implementation uses os.WriteFile which requires the parent to exist)
	err := repo.Save(nestedPath, config)

	// then
	// The current implementation does NOT create parent dirs, so this should fail.
	// This test documents the current behavior.
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write config file")
}

func TestYAMLConfigRepository_Save_ShouldEmitSingleQuotedStringValues(t *testing.T) {
	// given — a config containing a mix of string, boolean, and integer-like
	// fields. The YAML rule requires single-quoted string values, unquoted
	// booleans/numbers, and unquoted map keys.
	repo := repositories.NewYAMLConfigRepository()
	path := filepath.Join(t.TempDir(), "config.yaml")
	config := &entities.Config{
		Sync: entities.SyncConfig{
			Remote:       "git@github.com:user/repo.git",
			Branch:       "main",
			AutoPush:     true,
			Debounce:     "60s",
			CommitPrefix: "sync",
		},
		Encryption: entities.EncryptionConfig{
			Identity:   "~/.config/aisync/key.txt",
			Recipients: []string{"age1abcd"},
		},
		Tools: map[string]entities.Tool{
			"claude": {Path: "~/.claude", Enabled: true},
		},
		Watch: entities.WatchConfig{
			PollingInterval: "30s",
			IgnoredPatterns: []string{"*.tmp", "*.swp"},
		},
	}

	// when
	err := repo.Save(path, config)

	// then
	assert.NoError(t, err)
	raw, err := os.ReadFile(path)
	assert.NoError(t, err)
	yamlText := string(raw)

	// String values must be wrapped in single quotes.
	assert.Contains(t, yamlText, "remote: 'git@github.com:user/repo.git'",
		"string scalar `remote` must be single-quoted")
	assert.Contains(t, yamlText, "branch: 'main'",
		"string scalar `branch` must be single-quoted")
	assert.Contains(t, yamlText, "commit_prefix: 'sync'",
		"string scalar `commit_prefix` must be single-quoted")
	assert.Contains(t, yamlText, "identity: '~/.config/aisync/key.txt'",
		"string scalar `identity` must be single-quoted for explicit YAML string formatting")
	assert.Contains(t, yamlText, "- 'age1abcd'",
		"string sequence values must be single-quoted")
	assert.Contains(t, yamlText, "- '*.tmp'",
		"glob string sequence values must be single-quoted")

	// Booleans must remain unquoted so the YAML parser preserves their type.
	assert.Contains(t, yamlText, "auto_push: true",
		"boolean scalar must be emitted unquoted")
	assert.NotContains(t, yamlText, "auto_push: 'true'",
		"boolean scalar must NOT be quoted")
	assert.NotContains(t, yamlText, `auto_push: "true"`,
		"boolean scalar must NOT be double-quoted either")
	assert.Contains(t, yamlText, "enabled: true",
		"boolean scalar in nested map must be emitted unquoted")

	// Map keys must be unquoted.
	assert.Contains(t, yamlText, "sync:",
		"top-level map keys must be unquoted")
	assert.Contains(t, yamlText, "claude:",
		"nested map key must be unquoted (no `'claude':`)")
	assert.NotContains(t, yamlText, "'claude':",
		"nested map key must NOT be single-quoted")
}

func TestYAMLConfigRepository_Save_QuotedStringRoundtripsCleanly(t *testing.T) {
	// given — single-quoted output must round-trip back to the same Config
	// so the new style does not silently change observed values.
	repo := repositories.NewYAMLConfigRepository()
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := &entities.Config{
		Sync: entities.SyncConfig{
			Remote:       "git@github.com:user/repo.git",
			Branch:       "main",
			AutoPush:     true,
			Debounce:     "60s",
			CommitPrefix: "sync",
		},
		Encryption: entities.EncryptionConfig{
			Identity:   "~/.config/aisync/key.txt",
			Recipients: []string{"age1abcd", "age1efgh"},
		},
		Tools: map[string]entities.Tool{
			"claude": {Path: "~/.claude", Enabled: true},
			"cursor": {Path: "~/.cursor", Enabled: false},
		},
		Watch: entities.WatchConfig{
			PollingInterval: "30s",
			IgnoredPatterns: []string{"*.tmp"},
		},
	}

	// when
	err := repo.Save(path, original)
	assert.NoError(t, err)
	loaded, err := repo.Load(path)

	// then
	assert.NoError(t, err)
	assert.Equal(t, original.Sync, loaded.Sync)
	assert.Equal(t, original.Encryption, loaded.Encryption)
	assert.Equal(t, original.Tools, loaded.Tools)
	assert.Equal(t, original.Watch, loaded.Watch)
}

func TestYAMLConfigRepository_Load_PartialConfigWithMissingOptionalFields(t *testing.T) {
	// given
	repo := repositories.NewYAMLConfigRepository()
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := `
sync:
  remote: "https://github.com/test/repo.git"
  branch: "main"
tools:
  claude:
    path: "~/.claude"
    enabled: true
`
	err := os.WriteFile(path, []byte(content), 0600)
	assert.NoError(t, err)

	// when
	config, err := repo.Load(path)

	// then
	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, "https://github.com/test/repo.git", config.Sync.Remote)
	assert.Equal(t, "main", config.Sync.Branch)
	assert.False(t, config.Sync.AutoPush)
	assert.Empty(t, config.Sync.Debounce)
	assert.Empty(t, config.Sync.CommitPrefix)
	assert.Empty(t, config.Encryption.Identity)
	assert.Nil(t, config.Encryption.Recipients)
	assert.Nil(t, config.Sources)
	assert.Empty(t, config.Watch.PollingInterval)
	assert.Nil(t, config.HooksExclude)
	assert.True(t, config.Tools["claude"].Enabled)
}

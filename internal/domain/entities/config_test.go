//go:build unit

package entities_test

import (
	"testing"
	"github.com/rios0rios0/aisync/internal/domain/entities"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestConfig_MarshalUnmarshalYAML(t *testing.T) {
	// given
	original := entities.Config{
		Sync: entities.SyncConfig{
			Remote:       "origin",
			Branch:       "main",
			AutoPush:     true,
			Debounce:     "5s",
			CommitPrefix: "aisync:",
		},
		Encryption: entities.EncryptionConfig{
			Identity:   "age1abc",
			Recipients: []string{"age1recipient1", "age1recipient2"},
		},
		Tools: map[string]entities.Tool{
			"claude": {Path: "~/.claude", Enabled: true},
		},
		Sources: []entities.Source{
			{Name: "guide", Repo: "rios0rios0/guide", Branch: "main"},
		},
		Watch: entities.WatchConfig{
			PollingInterval: "2s",
			IgnoredPatterns: []string{"*.tmp"},
		},
		HooksExclude: []entities.HooksExcludeEntry{
			{Event: "PreSync", Matcher: "*.log", Command: "echo skip"},
		},
	}

	// when
	data, err := yaml.Marshal(&original)
	assert.NoError(t, err)

	var restored entities.Config
	err = yaml.Unmarshal(data, &restored)

	// then
	assert.NoError(t, err)
	assert.Equal(t, original.Sync.Remote, restored.Sync.Remote)
	assert.Equal(t, original.Sync.Branch, restored.Sync.Branch)
	assert.Equal(t, original.Sync.AutoPush, restored.Sync.AutoPush)
	assert.Equal(t, original.Sync.Debounce, restored.Sync.Debounce)
	assert.Equal(t, original.Sync.CommitPrefix, restored.Sync.CommitPrefix)
	assert.Equal(t, original.Encryption.Identity, restored.Encryption.Identity)
	assert.Equal(t, original.Encryption.Recipients, restored.Encryption.Recipients)
	assert.Equal(t, original.Tools["claude"].Path, restored.Tools["claude"].Path)
	assert.Equal(t, original.Tools["claude"].Enabled, restored.Tools["claude"].Enabled)
	assert.Len(t, restored.Sources, 1)
	assert.Equal(t, original.Sources[0].Name, restored.Sources[0].Name)
	assert.Equal(t, original.Watch.PollingInterval, restored.Watch.PollingInterval)
	assert.Equal(t, original.Watch.IgnoredPatterns, restored.Watch.IgnoredPatterns)
	assert.Len(t, restored.HooksExclude, 1)
	assert.Equal(t, original.HooksExclude[0].Event, restored.HooksExclude[0].Event)
	assert.Equal(t, original.HooksExclude[0].Matcher, restored.HooksExclude[0].Matcher)
	assert.Equal(t, original.HooksExclude[0].Command, restored.HooksExclude[0].Command)
}

func TestConfig_EmptyHooksExclude_OmittedInYAML(t *testing.T) {
	// given
	cfg := entities.Config{
		Sync: entities.SyncConfig{Remote: "origin"},
	}

	// when
	data, err := yaml.Marshal(&cfg)

	// then
	assert.NoError(t, err)
	assert.NotContains(t, string(data), "hooks_exclude")
}

func TestConfig_UnmarshalFromYAML(t *testing.T) {
	// given
	raw := `
sync:
  remote: origin
  branch: main
  auto_push: false
  debounce: "10s"
  commit_prefix: "sync:"
encryption:
  identity: age1key
  recipients:
    - age1r1
tools:
  cursor:
    path: ~/.cursor
    enabled: true
sources:
  - name: src1
    repo: owner/repo
    branch: develop
    refresh: "1h"
    mappings:
      - source: rules/
        target: rules/
watch:
  polling_interval: "3s"
  ignored_patterns:
    - "*.log"
    - "*.bak"
hooks_exclude:
  - event: PostSync
    matcher: "*.md"
    command: "echo done"
`

	// when
	var cfg entities.Config
	err := yaml.Unmarshal([]byte(raw), &cfg)

	// then
	assert.NoError(t, err)
	assert.Equal(t, "origin", cfg.Sync.Remote)
	assert.Equal(t, "main", cfg.Sync.Branch)
	assert.False(t, cfg.Sync.AutoPush)
	assert.Equal(t, "10s", cfg.Sync.Debounce)
	assert.Equal(t, "sync:", cfg.Sync.CommitPrefix)
	assert.Equal(t, "age1key", cfg.Encryption.Identity)
	assert.Equal(t, []string{"age1r1"}, cfg.Encryption.Recipients)
	assert.True(t, cfg.Tools["cursor"].Enabled)
	assert.Equal(t, "~/.cursor", cfg.Tools["cursor"].Path)
	assert.Len(t, cfg.Sources, 1)
	assert.Equal(t, "src1", cfg.Sources[0].Name)
	assert.Equal(t, "develop", cfg.Sources[0].Branch)
	assert.Equal(t, "1h", cfg.Sources[0].Refresh)
	assert.Len(t, cfg.Sources[0].Mappings, 1)
	assert.Equal(t, "rules/", cfg.Sources[0].Mappings[0].Source)
	assert.Equal(t, "rules/", cfg.Sources[0].Mappings[0].Target)
	assert.Equal(t, "3s", cfg.Watch.PollingInterval)
	assert.Equal(t, []string{"*.log", "*.bak"}, cfg.Watch.IgnoredPatterns)
	assert.Len(t, cfg.HooksExclude, 1)
	assert.Equal(t, "PostSync", cfg.HooksExclude[0].Event)
	assert.Equal(t, "*.md", cfg.HooksExclude[0].Matcher)
	assert.Equal(t, "echo done", cfg.HooksExclude[0].Command)
}

func TestHooksExcludeEntry_MarshalRoundTrip(t *testing.T) {
	// given
	entry := entities.HooksExcludeEntry{
		Event:   "PreSync",
		Matcher: "personal/**",
		Command: "skip-encrypt",
	}

	// when
	data, err := yaml.Marshal(&entry)
	assert.NoError(t, err)

	var restored entities.HooksExcludeEntry
	err = yaml.Unmarshal(data, &restored)

	// then
	assert.NoError(t, err)
	assert.Equal(t, entry, restored)
}

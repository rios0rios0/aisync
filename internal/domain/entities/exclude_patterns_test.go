package entities_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

func TestParseExcludePatterns(t *testing.T) {
	t.Parallel()

	t.Run("should skip comments and blank lines and trim each pattern", func(t *testing.T) {
		t.Parallel()
		// given
		content := []byte("# Termux keeps its own settings\n\n  claude/settings.json  \n\t*/AGENTS.md\n")

		// when
		patterns, invalid := entities.ParseExcludePatterns(content)

		// then
		assert.Empty(t, invalid)
		assert.Equal(t, []entities.ExcludePattern{
			{Tool: "claude", Pattern: "settings.json"},
			{Tool: "*", Pattern: "AGENTS.md"},
		}, patterns.Patterns)
		assert.Equal(t, 2, patterns.Len())
	})

	t.Run("should report lines that name no tool or no path, and keep the rest", func(t *testing.T) {
		t.Parallel()
		// given
		content := []byte("settings.json\n/CLAUDE.md\nclaude/\nopencode/AGENTS.md\n")

		// when
		patterns, invalid := entities.ParseExcludePatterns(content)

		// then
		assert.Equal(t, []string{"settings.json", "/CLAUDE.md", "claude/"}, invalid)
		require.Len(t, patterns.Patterns, 1)
		assert.Equal(t, entities.ExcludePattern{Tool: "opencode", Pattern: "AGENTS.md"}, patterns.Patterns[0])
	})

	t.Run("should yield no patterns from empty content", func(t *testing.T) {
		t.Parallel()
		// given
		content := []byte("")

		// when
		patterns, invalid := entities.ParseExcludePatterns(content)

		// then
		assert.Empty(t, invalid)
		assert.Equal(t, 0, patterns.Len())
	})
}

func TestExcludePatterns_Matches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		lines       string
		tool        string
		path        string
		shouldMatch bool
	}{
		{
			name:        "should match the exact file at the tool root",
			lines:       "claude/settings.json",
			tool:        "claude",
			path:        "settings.json",
			shouldMatch: true,
		},
		{
			name:        "should not match a file of the same name deeper in the tool directory",
			lines:       "claude/CLAUDE.md",
			tool:        "claude",
			path:        "skills/review/CLAUDE.md",
			shouldMatch: false,
		},
		{
			name:        "should not match the same path of another tool",
			lines:       "claude/settings.json",
			tool:        "cursor",
			path:        "settings.json",
			shouldMatch: false,
		},
		{
			name:        "should match every tool for the * tool",
			lines:       "*/settings.json",
			tool:        "cursor",
			path:        "settings.json",
			shouldMatch: true,
		},
		{
			name:        "should match across directories with **",
			lines:       "claude/projects/**/*.jsonl",
			tool:        "claude",
			path:        "projects/-home-user-repo/session.jsonl",
			shouldMatch: true,
		},
		{
			name:        "should match a whole subtree with a trailing slash",
			lines:       "claude/hooks/",
			tool:        "claude",
			path:        "hooks/wip-pin-session.py",
			shouldMatch: true,
		},
		{
			name:        "should match a directory named with a trailing slash at any depth, as the allowlist does",
			lines:       "claude/hooks/",
			tool:        "claude",
			path:        "skills/review/hooks/check.sh",
			shouldMatch: true,
		},
		{
			name:        "should match a single segment with *",
			lines:       "opencode/skills/*/SKILL.md",
			tool:        "opencode",
			path:        "skills/review/SKILL.md",
			shouldMatch: true,
		},
		{
			name:        "should normalize Windows separators in the path",
			lines:       "claude/hooks/**",
			tool:        "claude",
			path:        `hooks\pre-commit.sh`,
			shouldMatch: true,
		},
		{
			name:        "should not match when no pattern applies",
			lines:       "claude/settings.json",
			tool:        "claude",
			path:        "rules/go.md",
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// given
			patterns, invalid := entities.ParseExcludePatterns([]byte(tt.lines))
			require.Empty(t, invalid)

			// when
			matched := patterns.Matches(tt.tool, tt.path)

			// then
			assert.Equal(t, tt.shouldMatch, matched)
		})
	}

	t.Run("should exclude nothing through a nil receiver", func(t *testing.T) {
		t.Parallel()
		// given
		var patterns *entities.ExcludePatterns

		// when
		matched := patterns.Matches("claude", "settings.json")

		// then
		assert.False(t, matched)
		assert.Equal(t, 0, patterns.Len())
	})
}

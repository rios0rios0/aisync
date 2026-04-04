//go:build unit

package entities_test

import (
	"testing"
	"github.com/rios0rios0/aisync/internal/domain/entities"

	"github.com/stretchr/testify/assert"
)

func TestIsDenied(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "should deny .credentials.json",
			path:     ".claude/.credentials.json",
			expected: true,
		},
		{
			name:     "should deny .credentials.json with prefix",
			path:     "tools/claude/.claude/.credentials.json",
			expected: true,
		},
		{
			name:     "should deny .oauth file",
			path:     ".claude/.oauth",
			expected: true,
		},
		{
			name:     "should deny .oauth_token (trailing wildcard)",
			path:     ".claude/.oauth_token",
			expected: true,
		},
		{
			name:     "should deny .oauth-device (trailing wildcard)",
			path:     ".claude/.oauth-device",
			expected: true,
		},
		{
			name:     "should deny statsig directory entries",
			path:     ".claude/statsig/experiments.json",
			expected: true,
		},
		{
			name:     "should deny statsig directory itself",
			path:     ".claude/statsig/",
			expected: true,
		},
		{
			name:     "should deny todos directory",
			path:     ".claude/todos/my-todo.json",
			expected: true,
		},
		{
			name:     "should deny session wildcard pattern",
			path:     ".claude/projects/myproject/session",
			expected: true,
		},
		{
			name:     "should deny session.json (trailing wildcard)",
			path:     ".claude/projects/myproject/session.json",
			expected: true,
		},
		{
			name:     "should deny session-abc123 (trailing wildcard)",
			path:     ".claude/projects/myproject/session-abc123",
			expected: true,
		},
		{
			name:     "should deny plugins directory",
			path:     ".claude/plugins/myplugin.js",
			expected: true,
		},
		{
			name:     "should deny .claude.json",
			path:     ".claude/.claude.json",
			expected: true,
		},
		{
			name:     "should deny .DS_Store",
			path:     ".DS_Store",
			expected: true,
		},
		{
			name:     "should deny .DS_Store in subdirectory",
			path:     "some/dir/.DS_Store",
			expected: true,
		},
		{
			name:     "should deny Thumbs.db",
			path:     "Thumbs.db",
			expected: true,
		},
		{
			name:     "should deny .git directory",
			path:     ".git/config",
			expected: true,
		},
		{
			name:     "should deny .git directory itself",
			path:     ".git/",
			expected: true,
		},
		{
			name:     "should allow normal rule files",
			path:     "rules/test.md",
			expected: false,
		},
		{
			name:     "should allow normal agent files",
			path:     "agents/review.md",
			expected: false,
		},
		{
			name:     "should allow regular hidden files not in denylist",
			path:     ".aisyncignore",
			expected: false,
		},
		{
			name:     "should allow CLAUDE.md",
			path:     "CLAUDE.md",
			expected: false,
		},
		{
			name:     "should handle backslash paths (Windows)",
			path:     ".claude\\.credentials.json",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			path := tt.path

			// when
			result := entities.IsDenied(path)

			// then
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDenyList_ContainsExpectedEntries(t *testing.T) {
	// given / when / then
	assert.True(t, len(entities.DenyList) > 0, "entities.DenyList should not be empty")

	expectedEntries := []string{
		".claude/.credentials.json",
		".claude/.oauth*",
		".claude/statsig/",
		".claude/todos/",
		".claude/projects/*/session*",
		".claude/plugins/",
		".claude/.claude.json",
		".DS_Store",
		"Thumbs.db",
		".git/",
	}
	for _, entry := range expectedEntries {
		assert.Contains(t, entities.DenyList, entry, "entities.DenyList should contain %s", entry)
	}
}

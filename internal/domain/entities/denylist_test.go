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
			name:     "should deny everything under .claude/projects",
			path:     ".claude/projects/myproject/session",
			expected: true,
		},
		{
			name:     "should deny conversation JSONL under .claude/projects",
			path:     "/home/user/.claude/projects/myproject/abc123.jsonl",
			expected: true,
		},
		{
			name:     "should deny subagent transcripts under .claude/projects",
			path:     ".claude/projects/myproject/conv/subagents/agent.jsonl",
			expected: true,
		},
		{
			name:     "should deny .claude/sessions directory",
			path:     ".claude/sessions/26626.json",
			expected: true,
		},
		{
			name:     "should deny .claude/tasks directory",
			path:     ".claude/tasks/abc/1.json",
			expected: true,
		},
		{
			name:     "should deny .claude/history.jsonl exactly",
			path:     ".claude/history.jsonl",
			expected: true,
		},
		{
			name:     "should deny .claude/history.jsonl under absolute path",
			path:     "/home/user/.claude/history.jsonl",
			expected: true,
		},
		{
			name:     "should deny .claude/backups directory",
			path:     ".claude/backups/.claude.json.backup.1776200112530",
			expected: true,
		},
		{
			name:     "should deny .claude/shell-snapshots directory",
			path:     ".claude/shell-snapshots/snapshot-zsh-1776204037093.sh",
			expected: true,
		},
		{
			name:     "should deny .claude/session-env directory",
			path:     ".claude/session-env/abc.env",
			expected: true,
		},
		{
			name:     "should deny .claude/ide directory",
			path:     ".claude/ide/lock",
			expected: true,
		},
		{
			name:     "should deny .claude/file-history directory",
			path:     ".claude/file-history/abc/hash@v1",
			expected: true,
		},
		{
			name:     "should deny .claude/cache directory",
			path:     ".claude/cache/changelog.md",
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
			name:     "should deny .cursor/projects directory",
			path:     ".cursor/projects/abc/def.json",
			expected: true,
		},
		{
			name:     "should deny .cursor/chats store.db",
			path:     ".cursor/chats/adcb/ea54/store.db",
			expected: true,
		},
		{
			name:     "should deny .cursor/snapshots directory",
			path:     ".cursor/snapshots/2025-01-01.json",
			expected: true,
		},
		{
			name:     "should deny .cursor/ide_state.json",
			path:     ".cursor/ide_state.json",
			expected: true,
		},
		{
			name:     "should deny .cursor/cli-config.json",
			path:     ".cursor/cli-config.json",
			expected: true,
		},
		{
			name:     "should deny .cursor/unified_repo_list.json",
			path:     ".cursor/unified_repo_list.json",
			expected: true,
		},
		{
			name:     "should deny .cursor/mcp.json",
			path:     ".cursor/mcp.json",
			expected: true,
		},
		{
			name:     "should deny .cursor/blocklist",
			path:     ".cursor/blocklist",
			expected: true,
		},
		{
			name:     "should deny .codex/sessions directory",
			path:     ".codex/sessions/abc.jsonl",
			expected: true,
		},
		{
			name:     "should deny .codex/cache directory",
			path:     ".codex/cache/data.json",
			expected: true,
		},
		{
			name:     "should deny .gemini/sessions directory",
			path:     ".gemini/sessions/abc.json",
			expected: true,
		},
		{
			name:     "should deny .gemini/cache directory",
			path:     ".gemini/cache/data.json",
			expected: true,
		},
		{
			name:     "should deny .cline/tasks directory",
			path:     ".cline/tasks/abc.json",
			expected: true,
		},
		{
			name:     "should deny .continue/sessions directory",
			path:     ".continue/sessions/abc.json",
			expected: true,
		},
		{
			name:     "should deny .roo/cache directory",
			path:     ".roo/cache/abc.json",
			expected: true,
		},
		{
			name:     "should deny .aisync/state.json",
			path:     ".aisync/state.json",
			expected: true,
		},
		{
			name:     "should allow .claude/rules (not in deny-list)",
			path:     ".claude/rules/golang.md",
			expected: false,
		},
		{
			name:     "should allow .claude/agents (not in deny-list)",
			path:     ".claude/agents/reviewer.md",
			expected: false,
		},
		{
			name:     "should allow .claude/commands (not in deny-list)",
			path:     ".claude/commands/commit.md",
			expected: false,
		},
		{
			name:     "should allow .claude/memories (encrypted, not denied)",
			path:     ".claude/memories/user.md",
			expected: false,
		},
		{
			name:     "should allow .cursor/rules (not in deny-list)",
			path:     ".cursor/rules/ts.md",
			expected: false,
		},
		{
			name:     "should not deny .claude/historical.jsonl (different filename)",
			path:     ".claude/historical.jsonl",
			expected: false,
		},
		{
			name:     "should not deny .cursor/mcp.yaml (different extension)",
			path:     ".cursor/mcp.yaml",
			expected: false,
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
		// Claude — credentials and OAuth state.
		".claude/.credentials.json",
		".claude/.oauth*",
		".claude/.claude.json",
		// Claude — transcripts and per-session runtime state.
		".claude/projects/",
		".claude/sessions/",
		".claude/tasks/",
		".claude/history.jsonl",
		".claude/shell-snapshots/",
		".claude/session-env/",
		".claude/ide/",
		".claude/file-history/",
		// Claude — backups and caches.
		".claude/backups/",
		".claude/cache/",
		".claude/statsig/",
		".claude/todos/",
		".claude/plugins/",
		// Cursor — chat DBs, transcripts, IDE state, CLI config.
		".cursor/projects/",
		".cursor/chats/",
		".cursor/snapshots/",
		".cursor/ide_state.json",
		".cursor/cli-config.json",
		".cursor/unified_repo_list.json",
		".cursor/mcp.json",
		".cursor/blocklist",
		// Codex.
		".codex/sessions/",
		".codex/cache/",
		// Other AI assistants.
		".gemini/sessions/",
		".gemini/cache/",
		".cline/tasks/",
		".continue/sessions/",
		".roo/cache/",
		// aisync device state.
		".aisync/state.json",
		// Generic.
		".DS_Store",
		"Thumbs.db",
		".git/",
	}
	for _, entry := range expectedEntries {
		assert.Contains(t, entities.DenyList, entry, "entities.DenyList should contain %s", entry)
	}
}

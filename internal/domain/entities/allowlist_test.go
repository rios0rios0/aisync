//go:build unit

package entities_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

func TestIsSyncable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		toolName       string
		relPath        string
		extraAllowlist []string
		expected       bool
	}{
		// Claude — positive (compiled-in allowlist)
		{
			name:     "should allow claude rules",
			toolName: "claude",
			relPath:  "rules/golang.md",
			expected: true,
		},
		{
			name:     "should allow claude nested rules",
			toolName: "claude",
			relPath:  "rules/sub/deep/file.md",
			expected: true,
		},
		{
			name:     "should allow claude agents",
			toolName: "claude",
			relPath:  "agents/code-reviewer.md",
			expected: true,
		},
		{
			name:     "should allow claude commands",
			toolName: "claude",
			relPath:  "commands/commit.md",
			expected: true,
		},
		{
			name:     "should allow claude hooks",
			toolName: "claude",
			relPath:  "hooks/pre-commit.sh",
			expected: true,
		},
		{
			name:     "should allow claude skills",
			toolName: "claude",
			relPath:  "skills/update-config/SKILL.md",
			expected: true,
		},
		{
			name:     "should allow claude memories",
			toolName: "claude",
			relPath:  "memories/user-preferences.md",
			expected: true,
		},
		{
			name:     "should allow claude output-styles",
			toolName: "claude",
			relPath:  "output-styles/minimal.md",
			expected: true,
		},
		{
			name:     "should allow claude settings.json",
			toolName: "claude",
			relPath:  "settings.json",
			expected: true,
		},
		{
			name:     "should allow claude settings.local.json",
			toolName: "claude",
			relPath:  "settings.local.json",
			expected: true,
		},
		{
			name:     "should allow claude CLAUDE.md",
			toolName: "claude",
			relPath:  "CLAUDE.md",
			expected: true,
		},
		{
			name:     "should allow claude AGENTS.md",
			toolName: "claude",
			relPath:  "AGENTS.md",
			expected: true,
		},

		// Claude — negative (formerly denied, now simply not allowlisted)
		{
			name:     "should deny claude projects transcripts",
			toolName: "claude",
			relPath:  "projects/abc/conv.jsonl",
			expected: false,
		},
		{
			name:     "should deny claude history.jsonl",
			toolName: "claude",
			relPath:  "history.jsonl",
			expected: false,
		},
		{
			name:     "should deny claude paste-cache (the leak that triggered this redesign)",
			toolName: "claude",
			relPath:  "paste-cache/8a836168454e1a6b.txt",
			expected: false,
		},
		{
			name:     "should deny claude backups",
			toolName: "claude",
			relPath:  "backups/.claude.json.backup.1776200112530",
			expected: false,
		},
		{
			name:     "should deny claude sessions",
			toolName: "claude",
			relPath:  "sessions/26626.json",
			expected: false,
		},
		{
			name:     "should deny claude tasks",
			toolName: "claude",
			relPath:  "tasks/abc/1.json",
			expected: false,
		},
		{
			name:     "should deny claude shell-snapshots",
			toolName: "claude",
			relPath:  "shell-snapshots/snapshot-zsh-1776204037093.sh",
			expected: false,
		},
		{
			name:     "should deny claude ide state",
			toolName: "claude",
			relPath:  "ide/lock",
			expected: false,
		},
		{
			name:     "should deny claude file-history",
			toolName: "claude",
			relPath:  "file-history/2fafe150/hash@v1",
			expected: false,
		},
		{
			name:     "should deny claude todos",
			toolName: "claude",
			relPath:  "todos/my-todo.json",
			expected: false,
		},
		{
			name:     "should deny claude plugins",
			toolName: "claude",
			relPath:  "plugins/myplugin.js",
			expected: false,
		},
		{
			name:     "should deny claude cache",
			toolName: "claude",
			relPath:  "cache/changelog.md",
			expected: false,
		},
		{
			name:     "should deny claude statsig",
			toolName: "claude",
			relPath:  "statsig/experiments.json",
			expected: false,
		},
		{
			name:     "should deny claude .credentials.json",
			toolName: "claude",
			relPath:  ".credentials.json",
			expected: false,
		},
		{
			name:     "should deny claude .claude.json",
			toolName: "claude",
			relPath:  ".claude.json",
			expected: false,
		},
		{
			name:     "should deny claude .oauth_token",
			toolName: "claude",
			relPath:  ".oauth_token",
			expected: false,
		},
		{
			name:     "should deny claude session-env",
			toolName: "claude",
			relPath:  "session-env/abc.env",
			expected: false,
		},

		// Cursor — positive
		{
			name:     "should allow cursor rules",
			toolName: "cursor",
			relPath:  "rules/ts.md",
			expected: true,
		},
		{
			name:     "should allow cursor skills",
			toolName: "cursor",
			relPath:  "skills/shell/SKILL.md",
			expected: true,
		},
		{
			name:     "should allow cursor skills-cursor",
			toolName: "cursor",
			relPath:  "skills-cursor/migrate-to-skills/SKILL.md",
			expected: true,
		},
		{
			name:     "should allow cursor memories",
			toolName: "cursor",
			relPath:  "memories/notes.md",
			expected: true,
		},
		{
			name:     "should allow cursor settings.local.json",
			toolName: "cursor",
			relPath:  "settings.local.json",
			expected: true,
		},
		{
			name:     "should allow cursor .cursorignore",
			toolName: "cursor",
			relPath:  ".cursorignore",
			expected: true,
		},

		// Cursor — negative
		{
			name:     "should deny cursor projects transcripts",
			toolName: "cursor",
			relPath:  "projects/abc/def.json",
			expected: false,
		},
		{
			name:     "should deny cursor chats store.db",
			toolName: "cursor",
			relPath:  "chats/adcb/ea54/store.db",
			expected: false,
		},
		{
			name:     "should deny cursor snapshots",
			toolName: "cursor",
			relPath:  "snapshots/2025-01-01.json",
			expected: false,
		},
		{
			name:     "should deny cursor ide_state.json",
			toolName: "cursor",
			relPath:  "ide_state.json",
			expected: false,
		},
		{
			name:     "should deny cursor mcp.json (leaks workspace names)",
			toolName: "cursor",
			relPath:  "mcp.json",
			expected: false,
		},
		{
			name:     "should deny cursor cli-config.json",
			toolName: "cursor",
			relPath:  "cli-config.json",
			expected: false,
		},
		{
			name:     "should deny cursor blocklist",
			toolName: "cursor",
			relPath:  "blocklist",
			expected: false,
		},
		{
			name:     "should deny cursor unified_repo_list.json",
			toolName: "cursor",
			relPath:  "unified_repo_list.json",
			expected: false,
		},

		// Copilot — positive
		{
			name:     "should allow copilot copilot-instructions.md",
			toolName: "copilot",
			relPath:  "copilot-instructions.md",
			expected: true,
		},
		{
			name:     "should allow copilot instructions",
			toolName: "copilot",
			relPath:  "instructions/security.instructions.md",
			expected: true,
		},
		{
			name:     "should allow copilot prompts",
			toolName: "copilot",
			relPath:  "prompts/refactor.prompt.md",
			expected: true,
		},

		// Copilot — negative
		{
			name:     "should deny copilot arbitrary files",
			toolName: "copilot",
			relPath:  "workflows/ci.yml",
			expected: false,
		},

		// Codex — positive
		{
			name:     "should allow codex rules",
			toolName: "codex",
			relPath:  "rules/golang.md",
			expected: true,
		},
		{
			name:     "should allow codex default.rules",
			toolName: "codex",
			relPath:  "default.rules",
			expected: true,
		},
		{
			name:     "should allow codex instructions",
			toolName: "codex",
			relPath:  "instructions/foo.md",
			expected: true,
		},

		// Codex — negative
		{
			name:     "should deny codex sessions",
			toolName: "codex",
			relPath:  "sessions/abc.jsonl",
			expected: false,
		},
		{
			name:     "should deny codex cache",
			toolName: "codex",
			relPath:  "cache/data.json",
			expected: false,
		},

		// Unknown tool — falls back to DefaultAllowlist
		{
			name:     "should allow unknown tool rules (default allowlist)",
			toolName: "some_new_tool",
			relPath:  "rules/foo.md",
			expected: true,
		},
		{
			name:     "should allow unknown tool agents (default allowlist)",
			toolName: "some_new_tool",
			relPath:  "agents/bar.md",
			expected: true,
		},
		{
			name:     "should allow unknown tool memories (default allowlist)",
			toolName: "some_new_tool",
			relPath:  "memories/baz.md",
			expected: true,
		},
		{
			name:     "should deny unknown tool cache (not in default allowlist)",
			toolName: "some_new_tool",
			relPath:  "cache/data.json",
			expected: false,
		},
		{
			name:     "should deny unknown tool projects (not in default allowlist)",
			toolName: "some_new_tool",
			relPath:  "projects/abc.jsonl",
			expected: false,
		},
		{
			name:     "should deny unknown tool arbitrary file (not in default allowlist)",
			toolName: "some_new_tool",
			relPath:  "random.bin",
			expected: false,
		},

		// User extra_allowlist — additive
		{
			name:           "should allow claude path via extra_allowlist",
			toolName:       "claude",
			relPath:        "my-research/notes.md",
			extraAllowlist: []string{"my-research/**"},
			expected:       true,
		},
		{
			name:           "should allow cursor path via extra_allowlist",
			toolName:       "cursor",
			relPath:        "mcp.json",
			extraAllowlist: []string{"mcp.json"},
			expected:       true,
		},
		{
			name:           "should allow unknown tool path via extra_allowlist",
			toolName:       "some_new_tool",
			relPath:        "custom/config.toml",
			extraAllowlist: []string{"custom/**"},
			expected:       true,
		},
		{
			name:           "should not be tricked by non-matching extra_allowlist",
			toolName:       "claude",
			relPath:        "projects/abc.jsonl",
			extraAllowlist: []string{"other-stuff/**"},
			expected:       false,
		},

		// Edge cases
		{
			name:     "should handle empty relPath",
			toolName: "claude",
			relPath:  "",
			expected: false,
		},
		{
			name:     "should normalize backslash paths",
			toolName: "claude",
			relPath:  "rules\\golang.md",
			expected: true,
		},
		{
			name:     "should deny relPath that coincidentally looks like allowlisted file elsewhere",
			toolName: "claude",
			relPath:  "random/CLAUDE.md",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// given
			toolName := tt.toolName
			relPath := tt.relPath
			extra := tt.extraAllowlist

			// when
			result := entities.IsSyncable(toolName, relPath, extra)

			// then
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToolAllowlists_CoversTier1Tools(t *testing.T) {
	t.Parallel()

	t.Run("should have compiled-in entries for every Tier-1 enabled-by-default tool", func(t *testing.T) {
		// given — Tier-1 tools are those enabled: true in DefaultTools()
		tier1 := []string{}
		for name, tool := range entities.DefaultTools() {
			if tool.Enabled {
				tier1 = append(tier1, name)
			}
		}

		// when / then
		for _, name := range tier1 {
			_, ok := entities.ToolAllowlists[name]
			assert.True(t, ok, "Tier-1 tool %q must have a compiled-in allowlist entry", name)
		}
	})
}

func TestDefaultAllowlist_IsNotEmpty(t *testing.T) {
	t.Parallel()

	// given / when / then
	assert.NotEmpty(t, entities.DefaultAllowlist, "DefaultAllowlist must have at least one fallback pattern")
}

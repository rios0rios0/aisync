//go:build unit

package entities_test

import (
	"testing"
	"github.com/rios0rios0/aisync/internal/domain/entities"

	"github.com/stretchr/testify/assert"
)

func TestDefaultTools_ReturnsAtLeast31Tools(t *testing.T) {
	// given / when
	tools := entities.DefaultTools()

	// then
	assert.True(t, len(tools) >= 31, "expected at least 31 tools, got %d", len(tools))
}

func TestDefaultTools_ClaudeIsEnabledByDefault(t *testing.T) {
	// given / when
	tools := entities.DefaultTools()

	// then
	claude, ok := tools["claude"]
	assert.True(t, ok, "claude should exist in default tools")
	assert.True(t, claude.Enabled, "claude should be enabled by default")
	assert.Equal(t, "~/.claude", claude.Path)
}

func TestDefaultTools_CursorIsEnabledByDefault(t *testing.T) {
	// given / when
	tools := entities.DefaultTools()

	// then
	cursor, ok := tools["cursor"]
	assert.True(t, ok, "cursor should exist in default tools")
	assert.True(t, cursor.Enabled, "cursor should be enabled by default")
	assert.Equal(t, "~/.cursor", cursor.Path)
}

func TestDefaultTools_CopilotIsEnabledByDefault(t *testing.T) {
	// given / when
	tools := entities.DefaultTools()

	// then
	copilot, ok := tools["copilot"]
	assert.True(t, ok, "copilot should exist in default tools")
	assert.True(t, copilot.Enabled, "copilot should be enabled by default")
	assert.Equal(t, "~/.github", copilot.Path)
}

func TestDefaultTools_CodexIsEnabledByDefault(t *testing.T) {
	// given / when
	tools := entities.DefaultTools()

	// then
	codex, ok := tools["codex"]
	assert.True(t, ok, "codex should exist in default tools")
	assert.True(t, codex.Enabled, "codex should be enabled by default")
	assert.Equal(t, "~/.codex", codex.Path)
}

func TestDefaultTools_DisabledToolsByDefault(t *testing.T) {
	tests := []struct {
		name         string
		expectedPath string
	}{
		{name: "gemini", expectedPath: "~/.gemini"},
		{name: "windsurf", expectedPath: "~/.codeium/windsurf"},
		{name: "cline", expectedPath: "~/.cline"},
		{name: "roo", expectedPath: "~/.roo"},
		{name: "continue", expectedPath: "~/.continue"},
		{name: "trae", expectedPath: "~/.trae"},
		{name: "amazonq", expectedPath: "~/.amazonq"},
		{name: "kilo", expectedPath: "~/.config/kilo"},
		{name: "opencode", expectedPath: "~/.config/opencode"},
		{name: "kiro", expectedPath: "~/.kiro"},
		{name: "factory", expectedPath: "~/.factory"},
		{name: "augment", expectedPath: "~/.augment"},
		{name: "tabnine", expectedPath: "~/.tabnine"},
		{name: "qwen", expectedPath: "~/.qwen"},
		{name: "rovodev", expectedPath: "~/.rovodev"},
		{name: "deepagents", expectedPath: "~/.deepagents"},
		{name: "warp", expectedPath: "~/.warp"},
		{name: "goose", expectedPath: "~/.config/goose"},
		{name: "zed", expectedPath: "~/.config/zed"},
		{name: "junie", expectedPath: "~/.junie"},
		{name: "amp", expectedPath: "~/.amp"},
		{name: "replit", expectedPath: "~/.replit"},
		{name: "blackbox", expectedPath: "~/.blackbox"},
		{name: "openclaw", expectedPath: "~/.openclaw"},
		{name: "antigravity", expectedPath: "~/.antigravity"},
		{name: "copilot-cli", expectedPath: "~/.copilot"},
	}

	tools := entities.DefaultTools()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given / when
			tool, ok := tools[tt.name]

			// then
			assert.True(t, ok, "%s should exist in default tools", tt.name)
			assert.False(t, tool.Enabled, "%s should be disabled by default", tt.name)
			assert.Equal(t, tt.expectedPath, tool.Path)
		})
	}
}

func TestDefaultTools_AllToolsHaveNonEmptyPath(t *testing.T) {
	// given / when
	tools := entities.DefaultTools()

	// then
	for name, tool := range tools {
		assert.NotEmpty(t, tool.Path, "tool %s should have a non-empty path", name)
	}
}

func TestDefaultTools_ExactlyFourEnabled(t *testing.T) {
	// given / when
	tools := entities.DefaultTools()

	// then
	enabledCount := 0
	for _, tool := range tools {
		if tool.Enabled {
			enabledCount++
		}
	}
	assert.Equal(t, 4, enabledCount, "exactly 4 tools should be enabled by default")
}

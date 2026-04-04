//go:build unit

package commands_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/domain/commands"
	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/test/doubles"
)

func TestWatchCommand_Execute(t *testing.T) {
	t.Run("should return error when config load fails", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{LoadErr: assert.AnError}
		cmd := commands.NewWatchCommand(configRepo, nil, nil)

		// when
		err := cmd.Execute("/tmp/config.yaml", "/tmp/repo", false, 60*time.Second)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})

	t.Run("should return error when no enabled tool directories are found", func(t *testing.T) {
		// given
		config := &entities.Config{
			Tools: map[string]entities.Tool{
				"claude": {Path: "/nonexistent/path", Enabled: true},
			},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		cmd := commands.NewWatchCommand(configRepo, nil, nil)

		// when
		err := cmd.Execute("/tmp/config.yaml", "/tmp/repo", false, 60*time.Second)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no enabled AI tool directories found")
	})

	t.Run("should return error when all tools are disabled", func(t *testing.T) {
		// given
		config := &entities.Config{
			Tools: map[string]entities.Tool{
				"claude": {Path: "/tmp", Enabled: false},
				"cursor": {Path: "/tmp", Enabled: false},
			},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		cmd := commands.NewWatchCommand(configRepo, nil, nil)

		// when
		err := cmd.Execute("/tmp/config.yaml", "/tmp/repo", false, 60*time.Second)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no enabled AI tool directories found")
	})

	t.Run("should return error when watch service fails to start", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		toolDir := filepath.Join(tmpDir, "claude")
		require.NoError(t, os.MkdirAll(toolDir, 0755))

		config := &entities.Config{
			Tools: map[string]entities.Tool{
				"claude": {Path: toolDir, Enabled: true},
			},
			Watch: entities.WatchConfig{
				IgnoredPatterns: []string{"*.tmp"},
			},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		watchService := &doubles.MockWatchService{WatchErr: assert.AnError}
		cmd := commands.NewWatchCommand(configRepo, watchService, nil)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, false, 60*time.Second)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to start watcher")
	})

	t.Run("should load ignore patterns from both config and file", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		toolDir := filepath.Join(tmpDir, "claude")
		require.NoError(t, os.MkdirAll(toolDir, 0755))

		// Create .aisyncignore file
		require.NoError(t, os.WriteFile(
			filepath.Join(tmpDir, ".aisyncignore"),
			[]byte("*.log\n*.bak"),
			0644,
		))

		config := &entities.Config{
			Tools: map[string]entities.Tool{
				"claude": {Path: toolDir, Enabled: true},
			},
			Watch: entities.WatchConfig{
				IgnoredPatterns: []string{"*.tmp", "*.swp"},
			},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		watchService := &doubles.MockWatchService{WatchErr: assert.AnError} // fail quickly for test
		cmd := commands.NewWatchCommand(configRepo, watchService, nil)

		// when
		_ = cmd.Execute("/tmp/config.yaml", tmpDir, false, 60*time.Second)

		// then
		require.NotNil(t, watchService.IgnorePatterns)
		// Should have 4 patterns total: 2 from file + 2 from config
		assert.Len(t, watchService.IgnorePatterns.Patterns, 4)
	})
}

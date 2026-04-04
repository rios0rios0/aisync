//go:build unit

package commands_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/domain/commands"
	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
	"github.com/rios0rios0/aisync/test/doubles"
)

func TestMigrateCommand_Execute(t *testing.T) {
	t.Run("should migrate personal files from tool directory to sync repo", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		// Create a tool directory with a file
		claudeDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(claudeDir, 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(claudeDir, "CLAUDE.md"),
			[]byte("# My Personal Config"),
			0644,
		))

		config := &entities.Config{
			Sources: []entities.Source{},
			Tools: map[string]entities.Tool{
				"claude": {Path: claudeDir, Enabled: true},
			},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		manifestRepo := &doubles.MockManifestRepository{}
		sourceRepo := &doubles.MockSourceRepository{
			Result: nil, // no external source files
		}
		cmd := commands.NewMigrateCommand(configRepo, manifestRepo, sourceRepo)

		// when
		err := cmd.Execute("/tmp/config.yaml", repoPath, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, manifestRepo.SaveCalls)

		// Verify file was copied to personal namespace
		migratedPath := filepath.Join(repoPath, "personal", "claude", "CLAUDE.md")
		content, readErr := os.ReadFile(migratedPath)
		require.NoError(t, readErr)
		assert.Equal(t, "# My Personal Config", string(content))
	})

	t.Run("should classify file as shared when checksum matches source content", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		sharedContent := []byte("# Shared Rule Content")

		// Create a tool directory with a file matching the source
		claudeDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(filepath.Join(claudeDir, "rules"), 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(claudeDir, "rules", "test.md"),
			sharedContent,
			0644,
		))

		config := &entities.Config{
			Sources: []entities.Source{
				{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
			},
			Tools: map[string]entities.Tool{
				"claude": {Path: claudeDir, Enabled: true},
			},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		manifestRepo := &doubles.MockManifestRepository{}
		sourceRepo := &doubles.MockSourceRepository{
			Result: &repositories.FetchResult{
				Files: map[string][]byte{
					"shared/claude/rules/test.md": sharedContent,
				},
			},
		}
		cmd := commands.NewMigrateCommand(configRepo, manifestRepo, sourceRepo)

		// when
		err := cmd.Execute("/tmp/config.yaml", repoPath, false)

		// then
		require.NoError(t, err)

		// Verify file was placed in shared namespace
		sharedPath := filepath.Join(repoPath, "shared", "claude", "rules", "test.md")
		content, readErr := os.ReadFile(sharedPath)
		require.NoError(t, readErr)
		assert.Equal(t, string(sharedContent), string(content))
	})

	t.Run("should return error when config load fails", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			LoadErr: assert.AnError,
		}
		cmd := commands.NewMigrateCommand(configRepo, nil, nil)

		// when
		err := cmd.Execute("/tmp/config.yaml", "/tmp/repo", false)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})

	t.Run("should print no files message when tools have no files", func(t *testing.T) {
		// given
		config := &entities.Config{
			Sources: []entities.Source{},
			Tools:   map[string]entities.Tool{},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		manifestRepo := &doubles.MockManifestRepository{}
		sourceRepo := &doubles.MockSourceRepository{}
		cmd := commands.NewMigrateCommand(configRepo, manifestRepo, sourceRepo)

		// when
		err := cmd.Execute("/tmp/config.yaml", "/tmp/repo", false)

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, manifestRepo.SaveCalls)
	})

	t.Run("should skip disabled tools during migration", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		claudeDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(claudeDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "test.md"), []byte("content"), 0644))

		config := &entities.Config{
			Sources: []entities.Source{},
			Tools: map[string]entities.Tool{
				"claude": {Path: claudeDir, Enabled: false},
			},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		manifestRepo := &doubles.MockManifestRepository{}
		sourceRepo := &doubles.MockSourceRepository{}
		cmd := commands.NewMigrateCommand(configRepo, manifestRepo, sourceRepo)

		// when
		err := cmd.Execute("/tmp/config.yaml", repoPath, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, manifestRepo.SaveCalls)
	})

	t.Run("should handle mixed shared and personal files", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		sharedContent := []byte("# Shared Rule")
		personalContent := []byte("# My Personal Rule")

		claudeDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(filepath.Join(claudeDir, "rules"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "rules/shared.md"), sharedContent, 0644))
		require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "rules/personal.md"), personalContent, 0644))

		config := &entities.Config{
			Sources: []entities.Source{
				{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
			},
			Tools: map[string]entities.Tool{
				"claude": {Path: claudeDir, Enabled: true},
			},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		manifestRepo := &doubles.MockManifestRepository{}
		sourceRepo := &doubles.MockSourceRepository{
			Result: &repositories.FetchResult{
				Files: map[string][]byte{
					"shared/claude/rules/shared.md": sharedContent,
				},
			},
		}
		cmd := commands.NewMigrateCommand(configRepo, manifestRepo, sourceRepo)

		// when
		err := cmd.Execute("/tmp/config.yaml", repoPath, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, manifestRepo.SaveCalls)

		// Verify shared file was placed in shared namespace
		sharedPath := filepath.Join(repoPath, "shared", "claude", "rules", "shared.md")
		content, readErr := os.ReadFile(sharedPath)
		require.NoError(t, readErr)
		assert.Equal(t, string(sharedContent), string(content))

		// Verify personal file was placed in personal namespace
		personalPath := filepath.Join(repoPath, "personal", "claude", "rules", "personal.md")
		content, readErr = os.ReadFile(personalPath)
		require.NoError(t, readErr)
		assert.Equal(t, string(personalContent), string(content))
	})

	t.Run("should skip tool directory that does not exist", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		config := &entities.Config{
			Sources: []entities.Source{},
			Tools: map[string]entities.Tool{
				"claude": {Path: filepath.Join(tmpDir, "nonexistent"), Enabled: true},
			},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		manifestRepo := &doubles.MockManifestRepository{}
		sourceRepo := &doubles.MockSourceRepository{}
		cmd := commands.NewMigrateCommand(configRepo, manifestRepo, sourceRepo)

		// when
		err := cmd.Execute("/tmp/config.yaml", repoPath, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, manifestRepo.SaveCalls)
	})

	t.Run("should continue when source fetch fails during checksum build", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")
		require.NoError(t, os.MkdirAll(repoPath, 0755))

		claudeDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(claudeDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "test.md"), []byte("content"), 0644))

		config := &entities.Config{
			Sources: []entities.Source{
				{Name: "failing", Repo: "foo/bar", Branch: "main"},
			},
			Tools: map[string]entities.Tool{
				"claude": {Path: claudeDir, Enabled: true},
			},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		manifestRepo := &doubles.MockManifestRepository{}
		sourceRepo := &doubles.MockSourceRepository{
			FetchErr: assert.AnError,
		}
		cmd := commands.NewMigrateCommand(configRepo, manifestRepo, sourceRepo)

		// when
		err := cmd.Execute("/tmp/config.yaml", repoPath, false)

		// then
		require.NoError(t, err)
		// File should be classified as personal since source fetch failed
		personalPath := filepath.Join(repoPath, "personal", "claude", "test.md")
		content, readErr := os.ReadFile(personalPath)
		require.NoError(t, readErr)
		assert.Equal(t, "content", string(content))
	})
}

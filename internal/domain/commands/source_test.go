//go:build unit

package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/domain/commands"
	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
	"github.com/rios0rios0/aisync/test/doubles"
)

func TestSourceCommand_Add(t *testing.T) {
	t.Run("should add new source to config when name is unique", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Sources: []entities.Source{},
			},
		}
		cmd := commands.NewSourceCommand(configRepo, nil)
		source := entities.Source{
			Name:   "guide",
			Repo:   "rios0rios0/guide",
			Branch: "generated",
		}

		// when
		err := cmd.Add("/tmp/config.yaml", source)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, configRepo.SaveCalls)
		require.NotNil(t, configRepo.SavedConfig)
		assert.Len(t, configRepo.SavedConfig.Sources, 1)
		assert.Equal(t, "guide", configRepo.SavedConfig.Sources[0].Name)
		assert.Equal(t, "168h", configRepo.SavedConfig.Sources[0].Refresh)
	})

	t.Run("should return error when source name already exists", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Sources: []entities.Source{
					{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
				},
			},
		}
		cmd := commands.NewSourceCommand(configRepo, nil)
		source := entities.Source{
			Name:   "guide",
			Repo:   "rios0rios0/guide",
			Branch: "main",
		}

		// when
		err := cmd.Add("/tmp/config.yaml", source)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
		assert.Equal(t, 0, configRepo.SaveCalls)
	})
}

func TestSourceCommand_Remove(t *testing.T) {
	t.Run("should remove source from config when name matches", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Sources: []entities.Source{
					{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
					{Name: "other", Repo: "foo/bar", Branch: "main"},
				},
			},
		}
		cmd := commands.NewSourceCommand(configRepo, nil)

		// when
		err := cmd.Remove("/tmp/config.yaml", "guide")

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, configRepo.SaveCalls)
		require.NotNil(t, configRepo.SavedConfig)
		assert.Len(t, configRepo.SavedConfig.Sources, 1)
		assert.Equal(t, "other", configRepo.SavedConfig.Sources[0].Name)
	})

	t.Run("should return error when source name is not found", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Sources: []entities.Source{
					{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
				},
			},
		}
		cmd := commands.NewSourceCommand(configRepo, nil)

		// when
		err := cmd.Remove("/tmp/config.yaml", "nonexistent")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
		assert.Equal(t, 0, configRepo.SaveCalls)
	})
}

func TestSourceCommand_List(t *testing.T) {
	t.Run("should return no error when sources are configured", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Sources: []entities.Source{
					{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated", Mappings: []entities.SourceMapping{{Source: "a", Target: "b"}}},
				},
			},
		}
		cmd := commands.NewSourceCommand(configRepo, nil)

		// when
		err := cmd.List("/tmp/config.yaml")

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, configRepo.LoadCalls)
	})

	t.Run("should return no error when no sources are configured", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Sources: []entities.Source{},
			},
		}
		cmd := commands.NewSourceCommand(configRepo, nil)

		// when
		err := cmd.List("/tmp/config.yaml")

		// then
		require.NoError(t, err)
	})
}

func TestSourceCommand_Pin(t *testing.T) {
	t.Run("should update ref in config when source is found", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Sources: []entities.Source{
					{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
				},
			},
		}
		cmd := commands.NewSourceCommand(configRepo, nil)

		// when
		err := cmd.Pin("/tmp/config.yaml", "guide", "v1.0.0")

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, configRepo.SaveCalls)
		require.NotNil(t, configRepo.SavedConfig)
		assert.Equal(t, "v1.0.0", configRepo.SavedConfig.Sources[0].Ref)
	})

	t.Run("should return error when source is not found", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Sources: []entities.Source{},
			},
		}
		cmd := commands.NewSourceCommand(configRepo, nil)

		// when
		err := cmd.Pin("/tmp/config.yaml", "nonexistent", "v1.0.0")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestSourceCommand_Update(t *testing.T) {
	t.Run("should fetch and write files when source is found", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Sources: []entities.Source{
					{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
				},
			},
		}
		sourceRepo := &doubles.MockSourceRepository{
			Result: &repositories.FetchResult{
				Files: map[string][]byte{
					"shared/claude/rules/test.md": []byte("# Test Rule"),
				},
				ETag: "etag-123",
			},
		}
		cmd := commands.NewSourceCommand(configRepo, sourceRepo)

		// when
		err := cmd.Update("/tmp/config.yaml", tmpDir, "guide")

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, sourceRepo.FetchCalls)
	})

	t.Run("should return error when named source is not found", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Sources: []entities.Source{
					{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
				},
			},
		}
		sourceRepo := &doubles.MockSourceRepository{
			Result: nil,
		}
		cmd := commands.NewSourceCommand(configRepo, sourceRepo)

		// when
		err := cmd.Update("/tmp/config.yaml", "/tmp/repo", "nonexistent")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("should return error when source repository is nil", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Sources: []entities.Source{},
			},
		}
		cmd := commands.NewSourceCommand(configRepo, nil)

		// when
		err := cmd.Update("/tmp/config.yaml", "/tmp/repo", "")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "source repository not configured")
	})

	t.Run("should complete without error when two sources map to same target", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Sources: []entities.Source{
					{Name: "source-a", Repo: "org/repo-a", Branch: "main"},
					{Name: "source-b", Repo: "org/repo-b", Branch: "main"},
				},
			},
		}
		sourceRepo := &doubles.MockSourceRepository{
			ResultsBySource: map[string]*repositories.FetchResult{
				"source-a": {
					Files: map[string][]byte{
						"shared/claude/rules/overlap.md": []byte("# From A"),
					},
					ETag: "etag-a",
				},
				"source-b": {
					Files: map[string][]byte{
						"shared/claude/rules/overlap.md": []byte("# From B"),
					},
					ETag: "etag-b",
				},
			},
		}
		cmd := commands.NewSourceCommand(configRepo, sourceRepo)

		// when
		err := cmd.Update("/tmp/config.yaml", tmpDir, "")

		// then
		require.NoError(t, err)
		assert.Equal(t, 2, sourceRepo.FetchCalls)
	})
}

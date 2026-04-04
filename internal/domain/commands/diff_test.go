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

func TestDiffCommand_Execute(t *testing.T) {
	t.Run("should display changes when shared diff has results", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Sources: []entities.Source{
					{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
				},
				Tools: map[string]entities.Tool{},
			},
		}
		sourceRepo := &doubles.MockSourceRepository{
			Result: &repositories.FetchResult{
				Files: map[string][]byte{
					"shared/claude/rules/test.md": []byte("# Test"),
				},
			},
		}
		diffService := &doubles.MockDiffService{
			SharedDiff: []entities.FileChange{
				{
					Path:      "shared/claude/rules/test.md",
					Direction: entities.ChangeAdded,
					Source:    "guide",
				},
			},
		}
		formatter := &entities.PlainFormatter{}
		cmd := commands.NewDiffCommand(configRepo, sourceRepo, diffService, formatter)

		// when
		err := cmd.Execute("/tmp/config.yaml", "/tmp/repo", commands.DiffOptions{
			Summary: true,
		})

		// then
		require.NoError(t, err)
	})

	t.Run("should print no changes message when diff is empty", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Sources: []entities.Source{},
				Tools:   map[string]entities.Tool{},
			},
		}
		sourceRepo := &doubles.MockSourceRepository{}
		diffService := &doubles.MockDiffService{}
		formatter := &entities.PlainFormatter{}
		cmd := commands.NewDiffCommand(configRepo, sourceRepo, diffService, formatter)

		// when
		err := cmd.Execute("/tmp/config.yaml", "/tmp/repo", commands.DiffOptions{})

		// then
		require.NoError(t, err)
	})

	t.Run("should return error when config load fails", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			LoadErr: assert.AnError,
		}
		cmd := commands.NewDiffCommand(configRepo, nil, nil, nil)

		// when
		err := cmd.Execute("/tmp/config.yaml", "/tmp/repo", commands.DiffOptions{})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})

	t.Run("should compute local diff in reverse mode", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Sources: []entities.Source{},
				Tools:   map[string]entities.Tool{},
			},
		}
		diffService := &doubles.MockDiffService{
			LocalDiff: []entities.FileChange{
				{
					Path:      "rules/custom.md",
					Direction: entities.ChangeModified,
					Source:    "personal",
				},
			},
		}
		formatter := &entities.PlainFormatter{}
		cmd := commands.NewDiffCommand(configRepo, nil, diffService, formatter)

		// when
		err := cmd.Execute("/tmp/config.yaml", "/tmp/repo", commands.DiffOptions{
			Reverse: true,
			Summary: true,
		})

		// then
		require.NoError(t, err)
	})

	t.Run("should only show personal diff when personal filter is set", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Sources: []entities.Source{
					{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
				},
				Tools: map[string]entities.Tool{},
			},
		}
		sourceRepo := &doubles.MockSourceRepository{
			Result: &repositories.FetchResult{
				Files: map[string][]byte{"shared/claude/rules/test.md": []byte("# Test")},
			},
		}
		diffService := &doubles.MockDiffService{
			SharedDiff: []entities.FileChange{
				{Path: "shared/claude/rules/test.md", Direction: entities.ChangeAdded, Source: "guide"},
			},
			PersonalDiff: []entities.FileChange{
				{Path: "rules/custom.md", Direction: entities.ChangeModified, Source: "personal"},
			},
			LocalDiff: []entities.FileChange{
				{Path: "rules/local.md", Direction: entities.ChangeAdded, Source: "personal"},
			},
		}
		formatter := &entities.PlainFormatter{}
		cmd := commands.NewDiffCommand(configRepo, sourceRepo, diffService, formatter)

		// when
		err := cmd.Execute("/tmp/config.yaml", "/tmp/repo", commands.DiffOptions{
			Personal: true,
			Summary:  true,
		})

		// then
		require.NoError(t, err)
		// With Personal=true, shared diff should not be computed (sourceRepo not called)
		assert.Equal(t, 0, sourceRepo.FetchCalls)
	})

	t.Run("should only show shared diff when shared filter is set", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Sources: []entities.Source{
					{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
				},
				Tools: map[string]entities.Tool{},
			},
		}
		sourceRepo := &doubles.MockSourceRepository{
			Result: &repositories.FetchResult{
				Files: map[string][]byte{"shared/claude/rules/test.md": []byte("# Test")},
			},
		}
		diffService := &doubles.MockDiffService{
			SharedDiff: []entities.FileChange{
				{Path: "shared/claude/rules/test.md", Direction: entities.ChangeAdded, Source: "guide"},
			},
		}
		formatter := &entities.PlainFormatter{}
		cmd := commands.NewDiffCommand(configRepo, sourceRepo, diffService, formatter)

		// when
		err := cmd.Execute("/tmp/config.yaml", "/tmp/repo", commands.DiffOptions{
			Shared:  true,
			Summary: true,
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, sourceRepo.FetchCalls)
	})

	t.Run("should filter by source when source filter is provided", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Sources: []entities.Source{
					{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
					{Name: "other", Repo: "foo/bar", Branch: "main"},
				},
				Tools: map[string]entities.Tool{},
			},
		}
		sourceRepo := &doubles.MockSourceRepository{
			Result: &repositories.FetchResult{
				Files: map[string][]byte{"shared/claude/rules/test.md": []byte("# Test")},
			},
		}
		diffService := &doubles.MockDiffService{
			SharedDiff: []entities.FileChange{
				{Path: "shared/claude/rules/test.md", Direction: entities.ChangeAdded, Source: "guide"},
			},
		}
		formatter := &entities.PlainFormatter{}
		cmd := commands.NewDiffCommand(configRepo, sourceRepo, diffService, formatter)

		// when
		err := cmd.Execute("/tmp/config.yaml", "/tmp/repo", commands.DiffOptions{
			SourceFilter: "guide",
			Summary:      true,
		})

		// then
		require.NoError(t, err)
		// Only one source should be fetched
		assert.Equal(t, 1, sourceRepo.FetchCalls)
		require.Len(t, sourceRepo.FetchedSources, 1)
		assert.Equal(t, "guide", sourceRepo.FetchedSources[0].Name)
	})

	t.Run("should show detailed output when summary is false", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Sources: []entities.Source{
					{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
				},
				Tools: map[string]entities.Tool{},
			},
		}
		sourceRepo := &doubles.MockSourceRepository{
			Result: &repositories.FetchResult{
				Files: map[string][]byte{"shared/claude/rules/test.md": []byte("# Test")},
			},
		}
		diffService := &doubles.MockDiffService{
			SharedDiff: []entities.FileChange{
				{
					Path:       "shared/claude/rules/test.md",
					Direction:  entities.ChangeAdded,
					Source:     "guide",
					RemoteSize: 100,
				},
			},
			PersonalDiff: []entities.FileChange{
				{
					Path:       "rules/personal.md",
					Direction:  entities.ChangeModified,
					Source:     "personal",
					LocalSize:  50,
					RemoteSize: 75,
				},
			},
		}
		formatter := &entities.PlainFormatter{}
		cmd := commands.NewDiffCommand(configRepo, sourceRepo, diffService, formatter)

		// when
		err := cmd.Execute("/tmp/config.yaml", "/tmp/repo", commands.DiffOptions{
			Summary: false, // detailed output
		})

		// then
		require.NoError(t, err)
	})

	t.Run("should show reverse mode with detailed output", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				Sources: []entities.Source{},
				Tools:   map[string]entities.Tool{},
			},
		}
		diffService := &doubles.MockDiffService{
			LocalDiff: []entities.FileChange{
				{
					Path:      "rules/custom.md",
					Direction: entities.ChangeRemoved,
					Source:    "personal",
				},
			},
		}
		formatter := &entities.PlainFormatter{}
		cmd := commands.NewDiffCommand(configRepo, nil, diffService, formatter)

		// when
		err := cmd.Execute("/tmp/config.yaml", "/tmp/repo", commands.DiffOptions{
			Reverse: true,
			Summary: false,
		})

		// then
		require.NoError(t, err)
	})
}


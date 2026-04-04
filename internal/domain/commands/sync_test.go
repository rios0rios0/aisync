//go:build unit

package commands_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/domain/commands"
	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
	"github.com/rios0rios0/aisync/test/doubles"
)

// newTestPullCommand creates a PullCommand wired with test doubles for sync tests.
func newTestPullCommand(
	configRepo *doubles.MockConfigRepository,
	stateRepo *doubles.MockStateRepository,
	sourceRepo *doubles.MockSourceRepository,
	gitRepo *doubles.MockGitRepository,
	applyService *doubles.MockApplyService,
) *commands.PullCommand {
	return commands.NewPullCommand(
		configRepo,
		stateRepo,
		sourceRepo,
		&doubles.MockManifestRepository{},
		gitRepo,
		&doubles.MockEncryptionService{},
		&doubles.MockConflictDetector{},
		&doubles.MockMerger{},
		&doubles.MockMerger{},
		&doubles.MockMerger{},
		applyService,
	)
}

// newTestPushCommand creates a PushCommand wired with test doubles for sync tests.
func newTestPushCommand(
	configRepo *doubles.MockConfigRepository,
	stateRepo *doubles.MockStateRepository,
	gitRepo *doubles.MockGitRepository,
) *commands.PushCommand {
	return commands.NewPushCommand(
		configRepo,
		stateRepo,
		gitRepo,
		&doubles.MockEncryptionService{},
		&doubles.MockManifestRepository{},
		&doubles.MockSecretScanner{},
	)
}

func TestSyncCommand_Execute(t *testing.T) {
	t.Run("should call pull then push when not dry-run", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		config := &entities.Config{
			Sources: []entities.Source{
				{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
			},
			Tools: map[string]entities.Tool{},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{
			State:     entities.NewState("test-device"),
			ExistsVal: true,
		}
		sourceRepo := &doubles.MockSourceRepository{
			Result: &repositories.FetchResult{
				Files: map[string][]byte{},
				ETag:  "etag-1",
			},
		}
		gitRepo := &doubles.MockGitRepository{
			HasRemoteVal: false,
			IsCleanVal:   true,
		}
		applyService := &doubles.MockApplyService{}

		pullCmd := newTestPullCommand(configRepo, stateRepo, sourceRepo, gitRepo, applyService)
		pushCmd := newTestPushCommand(configRepo, stateRepo, gitRepo)
		syncCmd := commands.NewSyncCommand(pullCmd, pushCmd)

		// when
		err := syncCmd.Execute("/tmp/config.yaml", tmpDir, "", false)

		// then
		require.NoError(t, err)
		// Pull phase loads config and fetches sources
		assert.GreaterOrEqual(t, configRepo.LoadCalls, 2) // pull + push
		assert.Equal(t, 1, sourceRepo.FetchCalls)
	})

	t.Run("should skip push phase when dry-run is true", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		config := &entities.Config{
			Sources: []entities.Source{
				{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
			},
			Tools: map[string]entities.Tool{},
		}
		configRepo := &doubles.MockConfigRepository{Config: config}
		stateRepo := &doubles.MockStateRepository{
			State:     entities.NewState("test-device"),
			ExistsVal: true,
		}
		sourceRepo := &doubles.MockSourceRepository{
			Result: &repositories.FetchResult{
				Files: map[string][]byte{},
				ETag:  "etag-1",
			},
		}
		gitRepo := &doubles.MockGitRepository{HasRemoteVal: false}
		applyService := &doubles.MockApplyService{}

		pullCmd := newTestPullCommand(configRepo, stateRepo, sourceRepo, gitRepo, applyService)
		pushCmd := newTestPushCommand(configRepo, stateRepo, gitRepo)
		syncCmd := commands.NewSyncCommand(pullCmd, pushCmd)

		// when
		err := syncCmd.Execute("/tmp/config.yaml", tmpDir, "", true)

		// then
		require.NoError(t, err)
		// Push should not open the git repo (only pull does)
		assert.Equal(t, 0, gitRepo.CommitAllCalls)
		assert.Equal(t, 0, gitRepo.PushCalls)
	})

	t.Run("should return error when pull phase fails", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{
			LoadErr: fmt.Errorf("config not found"),
		}
		stateRepo := &doubles.MockStateRepository{}
		gitRepo := &doubles.MockGitRepository{}
		sourceRepo := &doubles.MockSourceRepository{}
		applyService := &doubles.MockApplyService{}

		pullCmd := newTestPullCommand(configRepo, stateRepo, sourceRepo, gitRepo, applyService)
		pushCmd := newTestPushCommand(configRepo, stateRepo, gitRepo)
		syncCmd := commands.NewSyncCommand(pullCmd, pushCmd)

		// when
		err := syncCmd.Execute("/tmp/config.yaml", "/tmp/repo", "", false)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "pull phase failed")
	})
}

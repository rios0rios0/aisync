//go:build unit

package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/domain/commands"
	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/test/doubles"
)

func TestDeviceCommand_List(t *testing.T) {
	t.Run("should return no error when devices are registered", func(t *testing.T) {
		// given
		stateRepo := &doubles.MockStateRepository{
			State: &entities.State{
				Devices: []entities.Device{
					{ID: "abc-123", Name: "laptop", Platform: "amd64", OS: "linux"},
					{ID: "def-456", Name: "desktop", Platform: "arm64", OS: "darwin"},
				},
			},
		}
		cmd := commands.NewDeviceCommand(stateRepo)

		// when
		err := cmd.List("/tmp/repo")

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, stateRepo.LoadCalls)
	})

	t.Run("should return no error when no devices are registered", func(t *testing.T) {
		// given
		stateRepo := &doubles.MockStateRepository{
			State: &entities.State{
				Devices: []entities.Device{},
			},
		}
		cmd := commands.NewDeviceCommand(stateRepo)

		// when
		err := cmd.List("/tmp/repo")

		// then
		require.NoError(t, err)
	})
}

func TestDeviceCommand_Rename(t *testing.T) {
	t.Run("should rename device when name matches", func(t *testing.T) {
		// given
		stateRepo := &doubles.MockStateRepository{
			State: &entities.State{
				Devices: []entities.Device{
					{ID: "abc-123", Name: "laptop", Platform: "amd64", OS: "linux"},
				},
			},
		}
		cmd := commands.NewDeviceCommand(stateRepo)

		// when
		err := cmd.Rename("/tmp/repo", "laptop", "workstation")

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, stateRepo.SaveCalls)
		require.NotNil(t, stateRepo.SavedState)
		assert.Equal(t, "workstation", stateRepo.SavedState.Devices[0].Name)
	})

	t.Run("should return error when device is not found", func(t *testing.T) {
		// given
		stateRepo := &doubles.MockStateRepository{
			State: &entities.State{
				Devices: []entities.Device{
					{ID: "abc-123", Name: "laptop", Platform: "amd64", OS: "linux"},
				},
			},
		}
		cmd := commands.NewDeviceCommand(stateRepo)

		// when
		err := cmd.Rename("/tmp/repo", "nonexistent", "new-name")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
		assert.Equal(t, 0, stateRepo.SaveCalls)
	})
}

func TestDeviceCommand_Remove(t *testing.T) {
	t.Run("should remove device when name matches", func(t *testing.T) {
		// given
		stateRepo := &doubles.MockStateRepository{
			State: &entities.State{
				Devices: []entities.Device{
					{ID: "abc-123", Name: "laptop", Platform: "amd64", OS: "linux"},
					{ID: "def-456", Name: "desktop", Platform: "arm64", OS: "darwin"},
				},
			},
		}
		cmd := commands.NewDeviceCommand(stateRepo)

		// when
		err := cmd.Remove("/tmp/repo", "laptop")

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, stateRepo.SaveCalls)
		require.NotNil(t, stateRepo.SavedState)
		assert.Len(t, stateRepo.SavedState.Devices, 1)
		assert.Equal(t, "desktop", stateRepo.SavedState.Devices[0].Name)
	})

	t.Run("should return error when device is not found", func(t *testing.T) {
		// given
		stateRepo := &doubles.MockStateRepository{
			State: &entities.State{
				Devices: []entities.Device{
					{ID: "abc-123", Name: "laptop", Platform: "amd64", OS: "linux"},
				},
			},
		}
		cmd := commands.NewDeviceCommand(stateRepo)

		// when
		err := cmd.Remove("/tmp/repo", "nonexistent")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
		assert.Equal(t, 0, stateRepo.SaveCalls)
	})
}

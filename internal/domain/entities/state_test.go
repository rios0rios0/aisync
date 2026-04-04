//go:build unit

package entities_test

import (
	"regexp"
	"runtime"
	"testing"
	"github.com/rios0rios0/aisync/internal/domain/entities"

	"github.com/stretchr/testify/assert"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewState_GeneratesValidUUID(t *testing.T) {
	// given
	deviceName := "test-laptop"

	// when
	state := entities.NewState(deviceName)

	// then
	assert.Len(t, state.Devices, 1)
	device := state.Devices[0]
	assert.Regexp(t, uuidPattern, device.ID, "device ID should be a valid v4 UUID")
}

func TestNewState_SetsPlatformAndOS(t *testing.T) {
	// given
	deviceName := "my-workstation"

	// when
	state := entities.NewState(deviceName)

	// then
	device := state.Devices[0]
	assert.Equal(t, runtime.GOARCH, device.Platform)
	assert.Equal(t, runtime.GOOS, device.OS)
}

func TestNewState_SetsDeviceName(t *testing.T) {
	// given
	deviceName := "prod-server"

	// when
	state := entities.NewState(deviceName)

	// then
	assert.Equal(t, deviceName, state.Devices[0].Name)
}

func TestNewState_SetsLastSync(t *testing.T) {
	// given / when
	state := entities.NewState("device")

	// then
	assert.False(t, state.Devices[0].LastSync.IsZero(), "LastSync should be set")
}

func TestNewState_InitializesSourceETags(t *testing.T) {
	// given / when
	state := entities.NewState("device")

	// then
	assert.NotNil(t, state.SourceETags)
	assert.Empty(t, state.SourceETags)
}

func TestNewState_LastPullAndPushAreZero(t *testing.T) {
	// given / when
	state := entities.NewState("device")

	// then
	assert.True(t, state.LastPull.IsZero(), "LastPull should be zero-value")
	assert.True(t, state.LastPush.IsZero(), "LastPush should be zero-value")
}

func TestNewState_GeneratesUniqueUUIDs(t *testing.T) {
	// given / when
	state1 := entities.NewState("device1")
	state2 := entities.NewState("device2")

	// then
	assert.NotEqual(t, state1.Devices[0].ID, state2.Devices[0].ID,
		"two separate NewState calls should generate different UUIDs")
}

func TestState_FindDevice_ExistingDevice(t *testing.T) {
	// given
	state := entities.NewState("laptop")

	// when
	device := state.FindDevice("laptop")

	// then
	assert.NotNil(t, device)
	assert.Equal(t, "laptop", device.Name)
}

func TestState_FindDevice_NonExistentDevice(t *testing.T) {
	// given
	state := entities.NewState("laptop")

	// when
	device := state.FindDevice("desktop")

	// then
	assert.Nil(t, device)
}

func TestState_FindDevice_MultipleDevices(t *testing.T) {
	// given
	state := entities.NewState("laptop")
	state.Devices = append(state.Devices, entities.Device{
		ID:   "second-id",
		Name: "desktop",
	})

	// when
	found := state.FindDevice("desktop")

	// then
	assert.NotNil(t, found)
	assert.Equal(t, "desktop", found.Name)
	assert.Equal(t, "second-id", found.ID)
}

func TestState_FindDevice_EmptyName(t *testing.T) {
	// given
	state := entities.NewState("laptop")

	// when
	device := state.FindDevice("")

	// then
	assert.Nil(t, device)
}

func TestState_SetETag_StoresValue(t *testing.T) {
	// given
	state := entities.NewState("device")

	// when
	state.SetETag("guide-source", "etag-abc123")

	// then
	assert.Equal(t, "etag-abc123", state.SourceETags["guide-source"])
}

func TestState_SetETag_OverwritesExisting(t *testing.T) {
	// given
	state := entities.NewState("device")
	state.SetETag("source1", "old-etag")

	// when
	state.SetETag("source1", "new-etag")

	// then
	assert.Equal(t, "new-etag", state.SourceETags["source1"])
}

func TestState_SetETag_InitializesNilMap(t *testing.T) {
	// given
	state := &entities.State{SourceETags: nil}

	// when
	state.SetETag("source", "etag-value")

	// then
	assert.NotNil(t, state.SourceETags)
	assert.Equal(t, "etag-value", state.SourceETags["source"])
}

func TestState_GetETag_ReturnsStoredValue(t *testing.T) {
	// given
	state := entities.NewState("device")
	state.SetETag("guide", "etag-xyz")

	// when
	result := state.GetETag("guide")

	// then
	assert.Equal(t, "etag-xyz", result)
}

func TestState_GetETag_ReturnsEmptyForMissing(t *testing.T) {
	// given
	state := entities.NewState("device")

	// when
	result := state.GetETag("nonexistent")

	// then
	assert.Equal(t, "", result)
}

func TestState_GetETag_ReturnsEmptyForNilMap(t *testing.T) {
	// given
	state := &entities.State{SourceETags: nil}

	// when
	result := state.GetETag("source")

	// then
	assert.Equal(t, "", result)
}

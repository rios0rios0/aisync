package entities

import (
	"crypto/rand"
	"fmt"
	"runtime"
	"time"
)

// State represents the sync state stored in .aisync/state.json.
type State struct {
	Devices     []Device          `json:"devices"`
	SourceETags map[string]string `json:"source_etags"`
	LastPull    time.Time         `json:"last_pull"`
	LastPush    time.Time         `json:"last_push"`
}

// Device identifies a machine that participates in the sync.
type Device struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Platform string    `json:"platform"`
	OS       string    `json:"os"`
	LastSync time.Time `json:"last_sync"`
}

// NewState creates a new State with a single device entry for the given name.
// It generates a v4 UUID using crypto/rand and populates platform information
// from the Go runtime.
func NewState(deviceName string) *State {
	return &State{
		Devices: []Device{
			{
				ID:       generateUUID(),
				Name:     deviceName,
				Platform: runtime.GOARCH,
				OS:       runtime.GOOS,
				LastSync: time.Now(),
			},
		},
		SourceETags: make(map[string]string),
		LastPull:    time.Time{},
		LastPush:    time.Time{},
	}
}

// FindDevice returns the device with the given name, or nil if not found.
func (s *State) FindDevice(name string) *Device {
	for i := range s.Devices {
		if s.Devices[i].Name == name {
			return &s.Devices[i]
		}
	}
	return nil
}

// SetETag stores the ETag for a named source.
func (s *State) SetETag(sourceName, etag string) {
	if s.SourceETags == nil {
		s.SourceETags = make(map[string]string)
	}
	s.SourceETags[sourceName] = etag
}

// GetETag returns the cached ETag for a named source, or an empty string if none.
func (s *State) GetETag(sourceName string) string {
	if s.SourceETags == nil {
		return ""
	}
	return s.SourceETags[sourceName]
}

// generateUUID produces a v4 UUID string using crypto/rand.
func generateUUID() string {
	var uuid [16]byte
	_, _ = rand.Read(uuid[:])

	// Set version 4 (bits 12-15 of time_hi_and_version)
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	// Set variant to RFC 4122 (bits 6-7 of clock_seq_hi_and_reserved)
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}

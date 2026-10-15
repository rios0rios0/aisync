package entities_test

import (
	"testing"
	"time"

	"github.com/rios0rios0/aisync/internal/domain/entities"

	"github.com/stretchr/testify/assert"
)

func TestFileChange_SizeDelta(t *testing.T) {
	tests := []struct {
		name       string
		localSize  int64
		remoteSize int64
		expected   int64
	}{
		{
			name:       "should return positive delta when remote is larger",
			localSize:  100,
			remoteSize: 250,
			expected:   150,
		},
		{
			name:       "should return negative delta when local is larger",
			localSize:  500,
			remoteSize: 200,
			expected:   -300,
		},
		{
			name:       "should return zero when sizes are equal",
			localSize:  100,
			remoteSize: 100,
			expected:   0,
		},
		{
			name:       "should handle zero sizes",
			localSize:  0,
			remoteSize: 0,
			expected:   0,
		},
		{
			name:       "should handle remote being zero",
			localSize:  100,
			remoteSize: 0,
			expected:   -100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			change := &entities.FileChange{
				LocalSize:  tt.localSize,
				RemoteSize: tt.remoteSize,
			}

			// when
			result := change.SizeDelta()

			// then
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFileChange_IsRemoteNewer(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name            string
		localTimestamp  time.Time
		remoteTimestamp time.Time
		expected        bool
	}{
		{
			name:            "should return true when remote is newer",
			localTimestamp:  now.Add(-time.Hour),
			remoteTimestamp: now,
			expected:        true,
		},
		{
			name:            "should return false when local is newer",
			localTimestamp:  now,
			remoteTimestamp: now.Add(-time.Hour),
			expected:        false,
		},
		{
			name:            "should return false when timestamps are equal",
			localTimestamp:  now,
			remoteTimestamp: now,
			expected:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			change := &entities.FileChange{
				LocalTimestamp:  tt.localTimestamp,
				RemoteTimestamp: tt.remoteTimestamp,
			}

			// when
			result := change.IsRemoteNewer()

			// then
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFileChange_HasClockSkew(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		change   entities.FileChange
		expected bool
	}{
		{
			name: "should return true when timestamps within 1s but checksums differ",
			change: entities.FileChange{
				LocalTimestamp:  now,
				RemoteTimestamp: now.Add(500 * time.Millisecond),
				LocalChecksum:   "aaa",
				RemoteChecksum:  "bbb",
			},
			expected: true,
		},
		{
			name: "should return false when timestamps differ by more than 1s",
			change: entities.FileChange{
				LocalTimestamp:  now,
				RemoteTimestamp: now.Add(2 * time.Second),
				LocalChecksum:   "aaa",
				RemoteChecksum:  "bbb",
			},
			expected: false,
		},
		{
			name: "should return false when checksums are the same",
			change: entities.FileChange{
				LocalTimestamp:  now,
				RemoteTimestamp: now.Add(500 * time.Millisecond),
				LocalChecksum:   "same",
				RemoteChecksum:  "same",
			},
			expected: false,
		},
		{
			name: "should return false when local checksum is empty",
			change: entities.FileChange{
				LocalTimestamp:  now,
				RemoteTimestamp: now.Add(500 * time.Millisecond),
				LocalChecksum:   "",
				RemoteChecksum:  "bbb",
			},
			expected: false,
		},
		{
			name: "should return false when remote checksum is empty",
			change: entities.FileChange{
				LocalTimestamp:  now,
				RemoteTimestamp: now.Add(500 * time.Millisecond),
				LocalChecksum:   "aaa",
				RemoteChecksum:  "",
			},
			expected: false,
		},
		{
			name: "should detect skew when remote is slightly before local",
			change: entities.FileChange{
				LocalTimestamp:  now,
				RemoteTimestamp: now.Add(-800 * time.Millisecond),
				LocalChecksum:   "aaa",
				RemoteChecksum:  "bbb",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			change := tt.change

			// when
			result := change.HasClockSkew()

			// then
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestChangeDirection_Constants(t *testing.T) {
	// given / when / then
	assert.Equal(t, entities.ChangeAdded, entities.ChangeDirection("+"))
	assert.Equal(t, entities.ChangeModified, entities.ChangeDirection("~"))
	assert.Equal(t, entities.ChangeRemoved, entities.ChangeDirection("-"))
}

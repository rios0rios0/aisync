//go:build unit

package entities_test

import (
	"testing"
	"github.com/rios0rios0/aisync/internal/domain/entities"
	"time"

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
		localTimestamp   time.Time
		remoteTimestamp  time.Time
		expected        bool
	}{
		{
			name:           "should return true when remote is newer",
			localTimestamp:  now.Add(-time.Hour),
			remoteTimestamp: now,
			expected:       true,
		},
		{
			name:           "should return false when local is newer",
			localTimestamp:  now,
			remoteTimestamp: now.Add(-time.Hour),
			expected:       false,
		},
		{
			name:           "should return false when timestamps are equal",
			localTimestamp:  now,
			remoteTimestamp: now,
			expected:       false,
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
		name           string
		change entities.FileChange
		expected       bool
	}{
		{
			name: "should return true when timestamps within 1s but checksums differ",
			change: entities.FileChange{
				LocalTimestamp:  now,
				RemoteTimestamp: now.Add(500 * time.Millisecond),
				LocalChecksum:  "aaa",
				RemoteChecksum: "bbb",
			},
			expected: true,
		},
		{
			name: "should return false when timestamps differ by more than 1s",
			change: entities.FileChange{
				LocalTimestamp:  now,
				RemoteTimestamp: now.Add(2 * time.Second),
				LocalChecksum:  "aaa",
				RemoteChecksum: "bbb",
			},
			expected: false,
		},
		{
			name: "should return false when checksums are the same",
			change: entities.FileChange{
				LocalTimestamp:  now,
				RemoteTimestamp: now.Add(500 * time.Millisecond),
				LocalChecksum:  "same",
				RemoteChecksum: "same",
			},
			expected: false,
		},
		{
			name: "should return false when local checksum is empty",
			change: entities.FileChange{
				LocalTimestamp:  now,
				RemoteTimestamp: now.Add(500 * time.Millisecond),
				LocalChecksum:  "",
				RemoteChecksum: "bbb",
			},
			expected: false,
		},
		{
			name: "should return false when remote checksum is empty",
			change: entities.FileChange{
				LocalTimestamp:  now,
				RemoteTimestamp: now.Add(500 * time.Millisecond),
				LocalChecksum:  "aaa",
				RemoteChecksum: "",
			},
			expected: false,
		},
		{
			name: "should detect skew when remote is slightly before local",
			change: entities.FileChange{
				LocalTimestamp:  now,
				RemoteTimestamp: now.Add(-800 * time.Millisecond),
				LocalChecksum:  "aaa",
				RemoteChecksum: "bbb",
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

func TestDiffResult_HasChanges(t *testing.T) {
	tests := []struct {
		name     string
		diff entities.DiffResult
		expected bool
	}{
		{
			name:     "should return false when all slices are empty",
			diff:     entities.DiffResult{},
			expected: false,
		},
		{
			name: "should return true when SharedChanges is non-empty",
			diff: entities.DiffResult{
				SharedChanges: []entities.FileChange{{Path: "a.md"}},
			},
			expected: true,
		},
		{
			name: "should return true when PersonalChanges is non-empty",
			diff: entities.DiffResult{
				PersonalChanges: []entities.FileChange{{Path: "b.md"}},
			},
			expected: true,
		},
		{
			name: "should return true when LocalUncommitted is non-empty",
			diff: entities.DiffResult{
				LocalUncommitted: []entities.FileChange{{Path: "c.md"}},
			},
			expected: true,
		},
		{
			name: "should return true when all slices have entries",
			diff: entities.DiffResult{
				SharedChanges:    []entities.FileChange{{Path: "a.md"}},
				PersonalChanges:  []entities.FileChange{{Path: "b.md"}},
				LocalUncommitted: []entities.FileChange{{Path: "c.md"}},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			diff := tt.diff

			// when
			result := diff.HasChanges()

			// then
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDiffResult_TotalCount(t *testing.T) {
	tests := []struct {
		name     string
		diff entities.DiffResult
		expected int
	}{
		{
			name:     "should return 0 for empty diff",
			diff:     entities.DiffResult{},
			expected: 0,
		},
		{
			name: "should count all changes across categories",
			diff: entities.DiffResult{
				SharedChanges:    []entities.FileChange{{Path: "a"}, {Path: "b"}},
				PersonalChanges:  []entities.FileChange{{Path: "c"}},
				LocalUncommitted: []entities.FileChange{{Path: "d"}, {Path: "e"}, {Path: "f"}},
			},
			expected: 6,
		},
		{
			name: "should count only shared changes when others are empty",
			diff: entities.DiffResult{
				SharedChanges: []entities.FileChange{{Path: "a"}, {Path: "b"}, {Path: "c"}},
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			diff := tt.diff

			// when
			result := diff.TotalCount()

			// then
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestChangeDirection_Constants(t *testing.T) {
	// given / when / then
	assert.Equal(t, entities.ChangeDirection("+"), entities.ChangeAdded)
	assert.Equal(t, entities.ChangeDirection("~"), entities.ChangeModified)
	assert.Equal(t, entities.ChangeDirection("-"), entities.ChangeRemoved)
}

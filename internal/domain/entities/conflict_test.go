//go:build unit

package entities_test

import (
	"testing"
	"github.com/rios0rios0/aisync/internal/domain/entities"

	"github.com/stretchr/testify/assert"
)

func TestConflict_ConflictFileName(t *testing.T) {
	tests := []struct {
		name     string
		conflict entities.Conflict
		expected string
	}{
		{
			name: "should return correct format for simple path",
			conflict: entities.Conflict{
				Path:         "rules/test.md",
				RemoteDevice: "laptop",
			},
			expected: "rules/test.md.conflict.laptop",
		},
		{
			name: "should include remote device name in suffix",
			conflict: entities.Conflict{
				Path:         "agents/review.md",
				RemoteDevice: "desktop-work",
			},
			expected: "agents/review.md.conflict.desktop-work",
		},
		{
			name: "should handle file with multiple dots",
			conflict: entities.Conflict{
				Path:         "config/settings.prod.yaml",
				RemoteDevice: "server-01",
			},
			expected: "config/settings.prod.yaml.conflict.server-01",
		},
		{
			name: "should handle root-level file",
			conflict: entities.Conflict{
				Path:         "CLAUDE.md",
				RemoteDevice: "mobile",
			},
			expected: "CLAUDE.md.conflict.mobile",
		},
		{
			name: "should handle deeply nested path",
			conflict: entities.Conflict{
				Path:         "a/b/c/d/file.txt",
				RemoteDevice: "remote",
			},
			expected: "a/b/c/d/file.txt.conflict.remote",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			c := tt.conflict

			// when
			result := c.ConflictFileName()

			// then
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConflict_StructFields(t *testing.T) {
	// given
	c := entities.Conflict{
		Path:          "rules/test.md",
		LocalDevice:   "my-laptop",
		RemoteDevice:  "work-desktop",
		LocalContent:  []byte("local content"),
		RemoteContent: []byte("remote content"),
	}

	// when / then
	assert.Equal(t, "rules/test.md", c.Path)
	assert.Equal(t, "my-laptop", c.LocalDevice)
	assert.Equal(t, "work-desktop", c.RemoteDevice)
	assert.Equal(t, []byte("local content"), c.LocalContent)
	assert.Equal(t, []byte("remote content"), c.RemoteContent)
}

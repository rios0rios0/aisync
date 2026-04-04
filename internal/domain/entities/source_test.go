//go:build unit

package entities_test

import (
	"testing"
	"github.com/rios0rios0/aisync/internal/domain/entities"

	"github.com/stretchr/testify/assert"
)

func TestSource_TarballURL(t *testing.T) {
	tests := []struct {
		name     string
		source   entities.Source
		expected string
	}{
		{
			name: "should return branch URL when Ref is empty",
			source: entities.Source{
				Repo:   "rios0rios0/guide",
				Branch: "main",
				Ref:    "",
			},
			expected: "https://github.com/rios0rios0/guide/archive/refs/heads/main.tar.gz",
		},
		{
			name: "should return branch URL with custom branch name",
			source: entities.Source{
				Repo:   "owner/repo",
				Branch: "develop",
				Ref:    "",
			},
			expected: "https://github.com/owner/repo/archive/refs/heads/develop.tar.gz",
		},
		{
			name: "should return tag URL when Ref is a tag",
			source: entities.Source{
				Repo:   "rios0rios0/guide",
				Branch: "main",
				Ref:    "v1.0.0",
			},
			expected: "https://github.com/rios0rios0/guide/archive/refs/tags/v1.0.0.tar.gz",
		},
		{
			name: "should return tag URL for short non-SHA Ref",
			source: entities.Source{
				Repo:   "owner/repo",
				Branch: "main",
				Ref:    "release-2024",
			},
			expected: "https://github.com/owner/repo/archive/refs/tags/release-2024.tar.gz",
		},
		{
			name: "should return SHA URL when Ref is 40 hex characters",
			source: entities.Source{
				Repo:   "rios0rios0/guide",
				Branch: "main",
				Ref:    "abc123def456789012345678901234567890abcd",
			},
			expected: "https://github.com/rios0rios0/guide/archive/abc123def456789012345678901234567890abcd.tar.gz",
		},
		{
			name: "should return SHA URL when Ref is longer than 40 hex characters",
			source: entities.Source{
				Repo:   "owner/repo",
				Branch: "main",
				Ref:    "abc123def456789012345678901234567890abcdef01",
			},
			expected: "https://github.com/owner/repo/archive/abc123def456789012345678901234567890abcdef01.tar.gz",
		},
		{
			name: "should return SHA URL for uppercase hex SHA",
			source: entities.Source{
				Repo:   "owner/repo",
				Branch: "main",
				Ref:    "ABC123DEF456789012345678901234567890ABCD",
			},
			expected: "https://github.com/owner/repo/archive/ABC123DEF456789012345678901234567890ABCD.tar.gz",
		},
		{
			name: "should treat 39-char hex as tag not SHA",
			source: entities.Source{
				Repo:   "owner/repo",
				Branch: "main",
				Ref:    "abc123def456789012345678901234567890abc",
			},
			expected: "https://github.com/owner/repo/archive/refs/tags/abc123def456789012345678901234567890abc.tar.gz",
		},
		{
			name: "should treat non-hex 40-char string as tag",
			source: entities.Source{
				Repo:   "owner/repo",
				Branch: "main",
				Ref:    "ghijklmnopqrstuvwxyz12345678901234567890",
			},
			expected: "https://github.com/owner/repo/archive/refs/tags/ghijklmnopqrstuvwxyz12345678901234567890.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			src := tt.source

			// when
			result := src.TarballURL()

			// then
			assert.Equal(t, tt.expected, result)
		})
	}
}

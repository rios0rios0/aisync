//go:build unit

package entities_test

import (
	"testing"
	"github.com/rios0rios0/aisync/internal/domain/entities"

	"github.com/stretchr/testify/assert"
)

func TestParseEncryptPatterns_SkipsCommentsAndBlanks(t *testing.T) {
	// given
	content := []byte(`# This is a comment
personal/*/memories/**

# Another comment
secrets/*.key

`)

	// when
	patterns := entities.ParseEncryptPatterns(content)

	// then
	assert.Len(t, patterns.Patterns, 2)
	assert.Equal(t, "personal/*/memories/**", patterns.Patterns[0])
	assert.Equal(t, "secrets/*.key", patterns.Patterns[1])
}

func TestParseEncryptPatterns_EmptyContent(t *testing.T) {
	// given
	content := []byte("")

	// when
	patterns := entities.ParseEncryptPatterns(content)

	// then
	assert.Empty(t, patterns.Patterns)
}

func TestParseEncryptPatterns_OnlyComments(t *testing.T) {
	// given
	content := []byte(`# comment 1
# comment 2
# comment 3`)

	// when
	patterns := entities.ParseEncryptPatterns(content)

	// then
	assert.Empty(t, patterns.Patterns)
}

func TestParseEncryptPatterns_OnlyBlankLines(t *testing.T) {
	// given
	content := []byte("\n\n\n")

	// when
	patterns := entities.ParseEncryptPatterns(content)

	// then
	assert.Empty(t, patterns.Patterns)
}

func TestParseEncryptPatterns_TrimsWhitespace(t *testing.T) {
	// given
	content := []byte("  personal/*.md  \n\t secrets/*.key \t\n")

	// when
	patterns := entities.ParseEncryptPatterns(content)

	// then
	assert.Len(t, patterns.Patterns, 2)
	assert.Equal(t, "personal/*.md", patterns.Patterns[0])
	assert.Equal(t, "secrets/*.key", patterns.Patterns[1])
}

func TestEncryptPatterns_Matches(t *testing.T) {
	tests := []struct {
		name         string
		patterns     []string
		path         string
		shouldMatch  bool
	}{
		{
			name:        "should match exact glob pattern",
			patterns:    []string{"secrets/*.key"},
			path:        "secrets/master.key",
			shouldMatch: true,
		},
		{
			name:        "should not match path outside pattern",
			patterns:    []string{"secrets/*.key"},
			path:        "public/readme.md",
			shouldMatch: false,
		},
		{
			name:        "should match double-star pattern via filename fallback",
			patterns:    []string{"personal/**/memories.md"},
			path:        "personal/device1/memories.md",
			shouldMatch: true,
		},
		{
			name:        "should match double-star pattern via filename only",
			patterns:    []string{"**/secret.yaml"},
			path:        "deep/nested/path/secret.yaml",
			shouldMatch: true,
		},
		{
			name:        "should not match when filename does not match double-star",
			patterns:    []string{"**/secret.yaml"},
			path:        "deep/nested/path/config.yaml",
			shouldMatch: false,
		},
		{
			name:        "should match with multiple patterns if any matches",
			patterns:    []string{"*.tmp", "*.log", "secrets/*"},
			path:        "app.log",
			shouldMatch: true,
		},
		{
			name:        "should not match if no patterns match",
			patterns:    []string{"*.tmp", "*.log"},
			path:        "readme.md",
			shouldMatch: false,
		},
		{
			name:        "should return false for empty patterns list",
			patterns:    []string{},
			path:        "anything.txt",
			shouldMatch: false,
		},
		{
			name:        "should match simple wildcard in filename",
			patterns:    []string{"*.age"},
			path:        "document.age",
			shouldMatch: true,
		},
		{
			name:        "should match trailing-slash directory pattern at any depth",
			patterns:    []string{"plans/"},
			path:        "plans/my-plan.md",
			shouldMatch: true,
		},
		{
			name:        "should match trailing-slash directory pattern under nested path",
			patterns:    []string{"plans/"},
			path:        "claude/plans/nested/file.md",
			shouldMatch: true,
		},
		{
			name:        "should not match trailing-slash directory against adjacent name",
			patterns:    []string{"plans/"},
			path:        "planning.md",
			shouldMatch: false,
		},
		{
			name:        "should match multi-segment trailing-slash directory pattern",
			patterns:    []string{"personal/claude/memories/"},
			path:        "personal/claude/memories/user.md",
			shouldMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			p := &entities.EncryptPatterns{Patterns: tt.patterns}

			// when
			result := p.Matches(tt.path)

			// then
			assert.Equal(t, tt.shouldMatch, result)
		})
	}
}

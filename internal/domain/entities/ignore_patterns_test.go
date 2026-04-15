//go:build unit

package entities_test

import (
	"testing"
	"github.com/rios0rios0/aisync/internal/domain/entities"

	"github.com/stretchr/testify/assert"
)

func TestParseIgnorePatterns_SkipsCommentsAndBlanks(t *testing.T) {
	// given
	content := []byte(`# Temporary files
*.tmp

# Swap files
*.swp

`)

	// when
	patterns := entities.ParseIgnorePatterns(content)

	// then
	assert.Len(t, patterns.Patterns, 2)
	assert.Equal(t, "*.tmp", patterns.Patterns[0])
	assert.Equal(t, "*.swp", patterns.Patterns[1])
}

func TestParseIgnorePatterns_EmptyContent(t *testing.T) {
	// given
	content := []byte("")

	// when
	patterns := entities.ParseIgnorePatterns(content)

	// then
	assert.Empty(t, patterns.Patterns)
}

func TestParseIgnorePatterns_OnlyComments(t *testing.T) {
	// given
	content := []byte("# just comments\n# nothing else\n")

	// when
	patterns := entities.ParseIgnorePatterns(content)

	// then
	assert.Empty(t, patterns.Patterns)
}

func TestIgnorePatterns_Matches(t *testing.T) {
	tests := []struct {
		name        string
		patterns    []string
		path        string
		shouldMatch bool
	}{
		{
			name:        "should match *.tmp pattern",
			patterns:    []string{"*.tmp"},
			path:        "data.tmp",
			shouldMatch: true,
		},
		{
			name:        "should match *.swp pattern",
			patterns:    []string{"*.swp"},
			path:        "file.swp",
			shouldMatch: true,
		},
		{
			name:        "should not match unrelated file",
			patterns:    []string{"*.tmp", "*.swp"},
			path:        "rules/test.md",
			shouldMatch: false,
		},
		{
			name:        "should match double-star pattern via filename fallback",
			patterns:    []string{"**/*.bak"},
			path:        "deep/nested/file.bak",
			shouldMatch: true,
		},
		{
			name:        "should not match when no patterns defined",
			patterns:    []string{},
			path:        "anything.txt",
			shouldMatch: false,
		},
		{
			name:        "should match wildcard in directory name",
			patterns:    []string{"temp/*"},
			path:        "temp/file.txt",
			shouldMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			p := &entities.IgnorePatterns{Patterns: tt.patterns}

			// when
			result := p.Matches(tt.path)

			// then
			assert.Equal(t, tt.shouldMatch, result)
		})
	}
}

// Note: the old TestIgnorePatterns_IsIgnored_* tests were removed along with
// the IsIgnored() method and the compiled deny-list. Under the allowlist
// model in allowlist.go, "what must never sync" is expressed as the inverse
// of ToolAllowlists rather than a secondary deny-list on IgnorePatterns.
// Coverage for that invariant lives in allowlist_test.go under the negative
// cases for each tool. IgnorePatterns is now a pure subtractive filter on
// the already-allowlisted set, and its behavior is exercised by Matches
// directly.

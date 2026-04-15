package entities

// IgnorePatterns holds glob patterns that determine which files should be
// excluded from syncing. Patterns are loaded from .aisyncignore and act as
// a subtractive filter applied AFTER the compiled-in allowlist. A file
// must be in the allowlist AND not match any ignore pattern to be synced.
type IgnorePatterns struct {
	Patterns []string
}

// ParseIgnorePatterns reads lines from the given content, skipping comments
// (lines starting with #) and blank lines. Each remaining line is treated as
// a glob pattern.
func ParseIgnorePatterns(content []byte) *IgnorePatterns {
	return &IgnorePatterns{
		Patterns: parsePatternLines(content),
	}
}

// Matches returns true if the given path matches any of the ignore patterns.
// For patterns without "/", it falls back to matching against just the
// filename component so "*.tmp" matches "some/dir/file.tmp".
func (p *IgnorePatterns) Matches(path string) bool {
	return matchesAnyPattern(path, p.Patterns)
}

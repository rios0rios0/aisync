package entities

// IgnorePatterns holds glob patterns that determine which files should be
// excluded from syncing. Patterns are loaded from .aisyncignore.
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
// For patterns containing "**", it also attempts to match against just the
// filename component of the path.
func (p *IgnorePatterns) Matches(path string) bool {
	return matchesAnyPattern(path, p.Patterns)
}

// IsIgnored returns true if the given path matches either the user-defined
// ignore patterns or the compiled-in DenyList.
func (p *IgnorePatterns) IsIgnored(path string) bool {
	if IsDenied(path) {
		return true
	}
	return p.Matches(path)
}

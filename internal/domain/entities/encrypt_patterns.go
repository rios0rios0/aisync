package entities

import (
	"path/filepath"
	"strings"
)

// EncryptPatterns holds glob patterns that determine which files should be
// encrypted before syncing. Patterns are loaded from .aisyncencrypt.
type EncryptPatterns struct {
	Patterns []string
}

// ParseEncryptPatterns reads lines from the given content, skipping comments
// (lines starting with #) and blank lines. Each remaining line is treated as
// a glob pattern.
func ParseEncryptPatterns(content []byte) *EncryptPatterns {
	return &EncryptPatterns{
		Patterns: parsePatternLines(content),
	}
}

// Matches returns true if the given relative path matches any of the encrypt
// patterns. For patterns containing "**", it also attempts to match against
// just the filename component of the path.
func (p *EncryptPatterns) Matches(relativePath string) bool {
	return matchesAnyPattern(relativePath, p.Patterns)
}

// parsePatternLines is a shared helper that splits content into lines,
// trims whitespace, and filters out comments and blank lines.
func parsePatternLines(content []byte) []string {
	var patterns []string
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

// matchesAnyPattern checks a path against a list of glob patterns.
// For patterns containing "**", it also tries matching only the filename.
func matchesAnyPattern(path string, patterns []string) bool {
	normalized := filepath.ToSlash(path)
	base := filepath.Base(normalized)

	for _, pattern := range patterns {
		pattern = filepath.ToSlash(pattern)

		if matched, _ := filepath.Match(pattern, normalized); matched {
			return true
		}

		// Handle "**" by also matching against just the filename component.
		if strings.Contains(pattern, "**") {
			// Replace "**/" or "**" with nothing to get the trailing pattern.
			simplePattern := strings.ReplaceAll(pattern, "**/", "")
			if matched, _ := filepath.Match(simplePattern, base); matched {
				return true
			}
		}
	}
	return false
}

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
	for line := range strings.SplitSeq(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

// matchesAnyPattern checks a path against a list of glob patterns. The matcher
// supports four pattern styles, in order of precedence:
//
//  1. Directory patterns ending with "/" (e.g. "plans/"). These match any path
//     containing the directory name as a contiguous segment, at any depth,
//     using the same engine as the compiled deny-list.
//  2. Full-path globs via [filepath.Match] (e.g. "personal/*/settings.local.json").
//  3. Basename globs for patterns without "/" (e.g. "*.tmp" matches
//     "some/dir/file.tmp").
//  4. Basename fallback for patterns containing "**" (e.g. "**/secret.yaml").
func matchesAnyPattern(path string, patterns []string) bool {
	normalized := filepath.ToSlash(path)
	base := filepath.Base(normalized)

	for _, pattern := range patterns {
		pattern = filepath.ToSlash(pattern)

		// Directory pattern: "plans/" matches any path with "plans" as a
		// contiguous segment sequence, at any depth. Mirrors the deny-list
		// matcher so .aisyncignore and the compiled deny-list behave the
		// same way when given "foo/"-style entries.
		if before, ok := strings.CutSuffix(pattern, "/"); ok {
			if matchesDirectoryPattern(normalized, before) {
				return true
			}
			continue
		}

		if matched, _ := filepath.Match(pattern, normalized); matched {
			return true
		}

		// For simple patterns without directory separators, also match
		// against the basename so "*.tmp" matches "some/dir/file.tmp".
		if !strings.Contains(pattern, "/") {
			if matched, _ := filepath.Match(pattern, base); matched {
				return true
			}
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

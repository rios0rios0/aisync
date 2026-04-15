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
// supports three pattern styles, in order of precedence:
//
//  1. Directory patterns ending with "/" (e.g. "plans/"). These match any path
//     containing the directory name as a contiguous segment, at any depth,
//     using the same engine as the compiled deny-list.
//  2. Segment-based globs with gitwildmatch-style "**" support (e.g.
//     "personal/*/settings.local.json" or "personal/**/memories/**"). A bare
//     "**" segment matches zero or more path components, so these patterns
//     behave the same way as .gitattributes — which is what users expect when
//     they encrypt secrets with recursive wildcards.
//  3. Basename globs for patterns without "/" (e.g. "*.tmp" matches
//     "some/dir/file.tmp"), via [filepath.Match] against the filename.
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

		if matchesRecursiveGlob(normalized, pattern) {
			return true
		}

		// For simple patterns without directory separators, also match
		// against the basename so "*.tmp" matches "some/dir/file.tmp".
		if !strings.Contains(pattern, "/") {
			if matched, _ := filepath.Match(pattern, base); matched {
				return true
			}
		}
	}
	return false
}

// matchesRecursiveGlob matches a slash-separated path against a pattern that
// may contain "**" (matches zero or more path segments) or "*" (matches
// within a single segment via [filepath.Match]). Both inputs are split on
// "/" and compared segment-by-segment, so the matcher agrees with
// .gitattributes/gitwildmatch semantics and correctly handles patterns like
// "personal/**/memories/**" across arbitrary nesting depths.
func matchesRecursiveGlob(path, pattern string) bool {
	return matchSegments(splitPathSegments(path), splitPathSegments(pattern))
}

// matchSegments is the recursive core of [matchesRecursiveGlob]. It walks the
// pattern and path in lockstep, consuming "**" greedily (collapsing runs to
// avoid exponential backtracking) and deferring per-segment comparisons to
// [filepath.Match] so single-star globs and character classes still work.
func matchSegments(path, pattern []string) bool {
	switch {
	case len(pattern) == 0:
		return len(path) == 0
	case pattern[0] == "**":
		for len(pattern) > 0 && pattern[0] == "**" {
			pattern = pattern[1:]
		}
		if len(pattern) == 0 {
			return true
		}
		for i := 0; i <= len(path); i++ {
			if matchSegments(path[i:], pattern) {
				return true
			}
		}
		return false
	case len(path) == 0:
		return false
	default:
		matched, _ := filepath.Match(pattern[0], path[0])
		if !matched {
			return false
		}
		return matchSegments(path[1:], pattern[1:])
	}
}

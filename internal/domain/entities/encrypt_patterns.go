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
// (lines starting with #) and blank lines. Each remaining line yields a glob
// pattern from its first whitespace-separated token, so .gitattributes-style
// rows like "personal/*/foo    encrypt" use the path as the pattern and
// silently ignore trailing action keywords. Without this tokenization the
// whole line ("personal/*/foo    encrypt") would be treated as a literal
// glob and never match any real path.
func ParseEncryptPatterns(content []byte) *EncryptPatterns {
	lines := parsePatternLines(content)
	patterns := make([]string, 0, len(lines))
	for _, line := range lines {
		if token := strings.Fields(line); len(token) > 0 {
			patterns = append(patterns, token[0])
		}
	}
	return &EncryptPatterns{Patterns: patterns}
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

// matchesAnchoredPattern is a stricter variant of [matchesAnyPattern] that
// does NOT fall back to basename matching for patterns without "/". It still
// honors trailing-slash directory patterns and gitwildmatch-style "**"
// globs, but a bare filename pattern like "CLAUDE.md" matches only when the
// path IS that filename at the tool root — never a file coincidentally
// named CLAUDE.md inside a runtime cache dir. Used by the allowlist layer
// in allowlist.go, where lenient basename matching would be a security
// regression (a vendor could ship a "new-runtime-thing/CLAUDE.md" and have
// it slip through).
func matchesAnchoredPattern(path string, patterns []string) bool {
	normalized := filepath.ToSlash(path)

	for _, pattern := range patterns {
		pattern = filepath.ToSlash(pattern)

		// Directory pattern ("plans/") — still matches any depth by design,
		// so users can write `my-research/` in extra_allowlist to cover a
		// whole subtree. This is the same semantics the deny-list used.
		if before, ok := strings.CutSuffix(pattern, "/"); ok {
			if matchesDirectoryPattern(normalized, before) {
				return true
			}
			continue
		}

		// Segment-based recursive glob. No basename fallback.
		if matchesRecursiveGlob(normalized, pattern) {
			return true
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

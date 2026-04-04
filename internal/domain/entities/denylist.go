package entities

import "strings"

// DenyList contains patterns for files that must never be synced. These are
// compiled into the binary and cannot be overridden by the user.
var DenyList = []string{
	".claude/.credentials.json",
	".claude/.oauth*",
	".claude/statsig/",
	".claude/todos/",
	".claude/projects/*/session*",
	".claude/plugins/",
	".claude/.claude.json",
	".DS_Store",
	"Thumbs.db",
	".git/",
}

// IsDenied checks if a path matches any entry in the compiled-in deny-list.
func IsDenied(path string) bool {
	normalized := strings.ReplaceAll(path, "\\", "/")
	for _, pattern := range DenyList {
		if matchesDenyPattern(normalized, pattern) {
			return true
		}
	}
	return false
}

// matchesDenyPattern checks if a normalized path matches a deny-list pattern.
// Patterns ending with "/" match any path under that prefix. Patterns with "*"
// match any single path segment.
func matchesDenyPattern(path, pattern string) bool {
	if strings.HasSuffix(pattern, "/") {
		return strings.Contains(path, pattern) || strings.Contains(path, strings.TrimSuffix(pattern, "/"))
	}
	if strings.Contains(pattern, "*") {
		// Split pattern by "*" and check that all segments appear in order.
		segments := strings.Split(pattern, "*")
		remaining := path
		for _, seg := range segments {
			if seg == "" {
				continue
			}
			idx := strings.Index(remaining, seg)
			if idx < 0 {
				return false
			}
			remaining = remaining[idx+len(seg):]
		}
		return true
	}
	return strings.HasSuffix(path, pattern) || strings.Contains(path, pattern)
}

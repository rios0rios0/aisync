package entities

import "strings"

// DenyList contains patterns for files that must never be synced. These are
// compiled into the binary and cannot be overridden by the user.
var DenyList = []string{
	".claude/.credentials.json",
	".claude/.oauth",
	".claude/statsig/",
	".claude/todos/",
	".claude/projects/*/session",
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
		prefix := pattern[:strings.Index(pattern, "*")]
		suffix := pattern[strings.Index(pattern, "*")+1:]
		return strings.Contains(path, prefix) && strings.Contains(path, suffix)
	}
	return strings.HasSuffix(path, pattern) || strings.Contains(path, pattern)
}

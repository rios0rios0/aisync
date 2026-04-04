package entities

import (
	"path/filepath"
	"strings"
)

// DenyList contains patterns for files that must never be synced. These are
// compiled into the binary and cannot be overridden by the user.
var DenyList = []string{ //nolint:gochecknoglobals // compiled-in deny patterns that cannot be overridden
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
// Patterns ending with "/" match directory segments (e.g., ".git/" matches
// paths containing a ".git" segment but not ".gitignore"). Patterns with "*"
// use [filepath.Match] against each path segment.
func matchesDenyPattern(path, pattern string) bool {
	if before, ok := strings.CutSuffix(pattern, "/"); ok {
		return matchesDirectoryPattern(path, before)
	}
	if strings.Contains(pattern, "*") {
		return matchesWildcardPattern(path, pattern)
	}
	return strings.HasSuffix(path, pattern) || path == pattern
}

// matchesDirectoryPattern checks if any segment (or contiguous sequence of
// segments) in the path matches the directory name exactly. This prevents
// ".git/" from matching ".gitignore".
func matchesDirectoryPattern(path, dirName string) bool {
	segments := splitPathSegments(path)
	dirSegments := splitPathSegments(dirName)

	if len(dirSegments) == 0 {
		return false
	}

	for i := 0; i <= len(segments)-len(dirSegments); i++ {
		match := true
		for j, ds := range dirSegments {
			if segments[i+j] != ds {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// matchesWildcardPattern checks if a path matches a wildcard deny pattern
// using [filepath.Match] against each path segment.
func matchesWildcardPattern(path, pattern string) bool {
	pathSegments := splitPathSegments(path)
	patternSegments := splitPathSegments(pattern)

	if len(patternSegments) == 0 {
		return false
	}

	for i := 0; i <= len(pathSegments)-len(patternSegments); i++ {
		match := true
		for j, ps := range patternSegments {
			if matched, _ := filepath.Match(ps, pathSegments[i+j]); !matched {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// splitPathSegments splits a forward-slash-separated path into its individual
// directory/file components.
func splitPathSegments(path string) []string {
	var segments []string
	for s := range strings.SplitSeq(path, "/") {
		if s != "" {
			segments = append(segments, s)
		}
	}
	return segments
}

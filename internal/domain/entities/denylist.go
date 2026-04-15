package entities

import (
	"path/filepath"
	"strings"
)

// DenyList contains patterns for files that must never be synced. These are
// compiled into the binary and cannot be overridden by the user. The list is
// grouped by tool so it is easy to extend as new AI assistants are adopted.
//
// Matching semantics (see [matchesDenyPattern]):
//   - A pattern ending in "/" matches any contiguous sequence of path segments,
//     so ".claude/projects/" blocks every file under any ".claude/projects/"
//     directory regardless of depth.
//   - A pattern containing "*" uses [filepath.Match] against path segments.
//   - A bare pattern matches the full path or any suffix.
var DenyList = []string{ //nolint:gochecknoglobals // compiled-in deny patterns that cannot be overridden
	// Claude — credentials and OAuth state.
	".claude/.credentials.json",
	".claude/.oauth*",
	".claude/.claude.json",
	// Claude — conversation transcripts and per-session runtime state.
	// These files contain verbatim user/assistant messages plus tool calls,
	// which routinely include private source code, internal hostnames,
	// customer data, and NDA-protected context.
	".claude/projects/",
	".claude/sessions/",
	".claude/tasks/",
	".claude/history.jsonl",
	".claude/shell-snapshots/",
	".claude/session-env/",
	".claude/ide/",
	".claude/file-history/", // per-session snapshots of every file Claude touched — includes raw source of private code

	// Claude — backups and caches.
	// Backups of ".claude.json" contain "oauthAccount" blocks; cache
	// directories hold resolved prompts and tool outputs.
	".claude/backups/",
	".claude/cache/",
	".claude/statsig/",
	".claude/todos/",
	".claude/plugins/",

	// Cursor — chat databases, conversation transcripts, IDE state, and
	// CLI configuration that leaks workspace / MCP server names.
	".cursor/projects/",
	".cursor/chats/",
	".cursor/snapshots/",
	".cursor/ide_state.json",
	".cursor/cli-config.json",
	".cursor/unified_repo_list.json",
	".cursor/mcp.json",
	".cursor/blocklist",

	// Codex — session transcripts and cached data.
	".codex/sessions/",
	".codex/cache/",

	// Other AI assistants — runtime and cache directories that commonly
	// carry conversation state. The deny-list covers these proactively so
	// enabling any of them via config does not silently leak data.
	".gemini/sessions/",
	".gemini/cache/",
	".cline/tasks/",
	".continue/sessions/",
	".roo/cache/",

	// aisync — per-device state file, never shared across devices.
	".aisync/state.json",

	// Generic junk and VCS metadata.
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

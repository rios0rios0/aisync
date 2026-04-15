package entities

import "strings"

// ToolAllowlists defines, per AI tool, the tool-relative glob patterns that
// aisync is willing to sync. Anything not matching one of a tool's patterns
// is NOT synced — period. This is the single source of truth that replaces
// the old compiled-in deny-list.
//
// The design principle is "fail-safe by default": when a vendor adds a new
// runtime/cache/transcript subdirectory under ~/.claude, ~/.cursor, etc.,
// aisync ignores it automatically. Previously this class of change caused
// silent plaintext leaks until a maintainer noticed and filed a deny-list
// patch. With the allowlist, the failure mode is "loud skip of something
// unusual" rather than "silent commit of conversation data".
//
// Patterns use the gitwildmatch-style language from encrypt_patterns.go:
// "**" crosses path separators, trailing "/" marks a directory subtree, and
// bare names match only the exact segment (NOT any file coincidentally
// sharing that name deeper in the tree). The allowlist uses the stricter
// [matchesAnchoredPattern] variant rather than [matchesAnyPattern] because
// a basename fallback here would re-open the exact leak vector this design
// exists to close: a vendor could drop `<runtime-cache>/CLAUDE.md` and have
// it slip through.
//
// Tool keys correspond to the entries in [DefaultTools]. Users can extend a
// tool's allowlist via tools.<name>.extra_allowlist in config.yaml without
// patching this file.
var ToolAllowlists = map[string][]string{ //nolint:gochecknoglobals // compiled-in per-tool allowlist; extend here when adding Tier-1 tool coverage
	// Claude Code (~/.claude/)
	"claude": {
		"rules/**",
		"agents/**",
		"commands/**",
		"hooks/**",
		"hooks.json",
		"skills/**",
		"memories/**",
		"output-styles/**",
		"settings.json",
		"settings.local.json",
		"CLAUDE.md",
		"AGENTS.md",
	},
	// Cursor (~/.cursor/)
	"cursor": {
		"rules/**",
		"skills/**",
		"skills-cursor/**",
		"memories/**",
		"settings.json",
		"settings.local.json",
		".gitignore",
		".cursorignore",
		".cursorindexingignore",
	},
	// GitHub Copilot (~/.github/)
	"copilot": {
		"copilot-instructions.md",
		"instructions/**",
		"prompts/**",
	},
	// OpenAI Codex CLI (~/.codex/)
	"codex": {
		"rules/**",
		"instructions/**",
		"memories/**",
		"default.rules",
	},
}

// DefaultAllowlist is the fallback set of patterns used when a tool is
// enabled in config.yaml but has no compiled-in entry in [ToolAllowlists].
// It captures conventions common across AI assistants so new tools that
// follow the standard layout work out of the box, while still denying
// runtime/cache/transcript content by omission.
var DefaultAllowlist = []string{ //nolint:gochecknoglobals // compiled-in fallback for unknown tools
	"rules/**",
	"agents/**",
	"commands/**",
	"skills/**",
	"instructions/**",
	"memories/**",
	"settings.json",
	"settings.local.json",
}

// IsSyncable reports whether a file under a tool's home directory is allowed
// to be synced. It returns true if the tool-relative path matches any of:
//
//  1. The compiled-in [ToolAllowlists] entry for toolName, if one exists.
//  2. [DefaultAllowlist] if toolName is not in [ToolAllowlists] (fallback
//     for brand-new AI assistants following common conventions).
//  3. extraAllowlist supplied by the caller (populated from
//     tools.<name>.extra_allowlist in config.yaml), matched in addition to
//     the compiled-in list regardless of whether the tool is known.
//
// It returns false otherwise. False is the safe default — unknown paths are
// never synced until someone explicitly opts in.
//
// relPath must be tool-relative (e.g. "rules/golang.md", not
// "/home/alice/.claude/rules/golang.md"). The caller is responsible for
// computing the relative path via [filepath.Rel] against the tool's home
// directory before invoking this function.
func IsSyncable(toolName, relPath string, extraAllowlist []string) bool {
	normalized := strings.ReplaceAll(relPath, "\\", "/")

	if patterns, ok := ToolAllowlists[toolName]; ok {
		if matchesAnchoredPattern(normalized, patterns) {
			return true
		}
	} else if matchesAnchoredPattern(normalized, DefaultAllowlist) {
		return true
	}

	if len(extraAllowlist) > 0 && matchesAnchoredPattern(normalized, extraAllowlist) {
		return true
	}

	return false
}

package entities

import "strings"

// excludeAnyTool is the tool name that makes an exclusion apply to every tool.
const excludeAnyTool = "*"

// ExcludePatterns holds the synced paths this device opts out of. They come
// from a file in aisync's config directory, outside the sync repo, so unlike
// .aisyncignore and config.yaml -- which are committed and shared by every
// device -- they are never committed or synced: one device can keep its own
// copy of a file that every other device goes on sharing.
//
// Each pattern is "<tool>/<path pattern>". The tool is a tool name or "*", and
// the path pattern is matched against the tool-relative path with the
// allowlist's anchored semantics (see [matchesAnchoredPattern]): "**" crosses
// separators, a trailing "/" names a directory at any depth, and a bare name is
// that exact path at the tool root, never a file of the same name deeper in the
// tree.
type ExcludePatterns struct {
	Patterns []ExcludePattern
}

// ExcludePattern is one "<tool>/<path pattern>" line of the exclude file.
type ExcludePattern struct {
	Tool    string
	Pattern string
}

// ParseExcludePatterns reads one pattern per line, skipping blank lines and
// lines starting with "#". A line that names no tool, or no path after it, is
// returned in invalid so the caller can report it, and is otherwise ignored.
func ParseExcludePatterns(content []byte) (*ExcludePatterns, []string) {
	patterns := &ExcludePatterns{}
	var invalid []string
	for _, line := range parsePatternLines(content) {
		tool, pattern, found := strings.Cut(line, "/")
		tool, pattern = strings.TrimSpace(tool), strings.TrimSpace(pattern)
		if !found || tool == "" || pattern == "" {
			invalid = append(invalid, line)
			continue
		}
		patterns.Patterns = append(patterns.Patterns, ExcludePattern{Tool: tool, Pattern: pattern})
	}
	return patterns, invalid
}

// Len returns the number of active patterns.
func (p *ExcludePatterns) Len() int {
	if p == nil {
		return 0
	}
	return len(p.Patterns)
}

// Matches reports whether relPath, relative to the directory of tool, is
// excluded on this device. A nil receiver excludes nothing.
func (p *ExcludePatterns) Matches(tool, relPath string) bool {
	if p == nil {
		return false
	}
	normalized := strings.ReplaceAll(relPath, "\\", "/")
	for _, exclude := range p.Patterns {
		if exclude.Tool != excludeAnyTool && exclude.Tool != tool {
			continue
		}
		if matchesAnchoredPattern(normalized, []string{exclude.Pattern}) {
			return true
		}
	}
	return false
}

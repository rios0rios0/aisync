package entities

import "strings"

// matchesDirectoryPattern checks whether any contiguous sequence of path
// segments matches the dirName (which itself may be a multi-segment path).
// This is how patterns like "plans/" match any file under any "plans"
// directory regardless of depth, and how "personal/claude/memories/"
// matches only under exactly that tree. It is shared by the allowlist
// (allowlist.go), the encrypt-pattern matcher, and the ignore-pattern
// matcher so every filter layer agrees on trailing-slash semantics.
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

// splitPathSegments splits a forward-slash-separated path into its
// individual directory/file components, dropping empty segments so that
// trailing slashes and leading slashes do not introduce phantom matches.
func splitPathSegments(path string) []string {
	var segments []string
	for s := range strings.SplitSeq(path, "/") {
		if s != "" {
			segments = append(segments, s)
		}
	}
	return segments
}

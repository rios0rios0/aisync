package services

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// unifiedContextLines is the number of unchanged lines shown around each
// change, matching `diff -u` and `git diff` defaults.
const unifiedContextLines = 3

// UnifiedDiffer renders the `diff -u` format on top of go-diff's line-mode
// Myers diff, so conflict prompts and `aisync diff` show the same hunks a
// developer would get from git.
type UnifiedDiffer struct{}

// NewUnifiedDiffer creates a new UnifiedDiffer.
func NewUnifiedDiffer() *UnifiedDiffer {
	return &UnifiedDiffer{}
}

// diffLine is one line of a line-level diff: ' ' for context, '-' for a line
// only in the old version, '+' for a line only in the new version.
type diffLine struct {
	kind byte
	text string
}

// hunk is a contiguous group of changes plus surrounding context, with the
// 1-based line ranges it covers in each version.
type hunk struct {
	fromStart, fromCount int
	toStart, toCount     int
	lines                []diffLine
}

// Unified returns the unified diff turning from into to, or "" when the two
// contents are identical.
func (d *UnifiedDiffer) Unified(fromLabel, toLabel string, from, to []byte) string {
	if bytes.Equal(from, to) {
		return ""
	}

	hunks := groupHunks(lineDiff(string(from), string(to)), unifiedContextLines)
	if len(hunks) == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "--- %s\n+++ %s\n", fromLabel, toLabel)
	for _, h := range hunks {
		fmt.Fprintf(&b, "@@ -%s +%s @@\n", hunkRange(h.fromStart, h.fromCount), hunkRange(h.toStart, h.toCount))
		for _, line := range h.lines {
			b.WriteByte(line.kind)
			b.WriteString(line.text)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// lineDiff computes a line-level diff by mapping each distinct line to a
// rune, diffing the rune strings, and expanding the result back to lines.
func lineDiff(from, to string) []diffLine {
	dmp := diffmatchpatch.New()
	fromRunes, toRunes, lines := dmp.DiffLinesToRunes(from, to)
	diffs := dmp.DiffCharsToLines(dmp.DiffMainRunes(fromRunes, toRunes, false), lines)

	var result []diffLine
	for _, diff := range diffs {
		kind := byte(' ')
		switch diff.Type {
		case diffmatchpatch.DiffDelete:
			kind = '-'
		case diffmatchpatch.DiffInsert:
			kind = '+'
		case diffmatchpatch.DiffEqual:
		}
		for _, text := range splitDiffLines(diff.Text) {
			result = append(result, diffLine{kind: kind, text: text})
		}
	}
	return result
}

// splitDiffLines splits text into lines without their trailing newline. A final
// line without a newline is kept; the empty string after a final newline is
// not a line.
func splitDiffLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// groupHunks groups changed lines into hunks, keeping up to context
// unchanged lines on each side and merging changes separated by no more than
// twice the context, like GNU diff.
func groupHunks(lines []diffLine, context int) []hunk {
	fromLine := make([]int, len(lines))
	toLine := make([]int, len(lines))
	nextFrom, nextTo := 1, 1
	for i, line := range lines {
		fromLine[i], toLine[i] = nextFrom, nextTo
		if line.kind != '+' {
			nextFrom++
		}
		if line.kind != '-' {
			nextTo++
		}
	}

	var hunks []hunk
	for i := 0; i < len(lines); {
		if lines[i].kind == ' ' {
			i++
			continue
		}
		start := max(0, i-context)
		end := changeRunEnd(lines, i, context)
		stop := min(len(lines), end+context)

		h := hunk{fromStart: fromLine[start], toStart: toLine[start], lines: lines[start:stop]}
		for _, line := range h.lines {
			if line.kind != '+' {
				h.fromCount++
			}
			if line.kind != '-' {
				h.toCount++
			}
		}
		hunks = append(hunks, h)
		i = stop
	}
	return hunks
}

// changeRunEnd returns the index just past the last change that belongs to
// the hunk starting at the change at index i.
func changeRunEnd(lines []diffLine, i, context int) int {
	end := i
	for {
		for end < len(lines) && lines[end].kind != ' ' {
			end++
		}
		next := end
		for next < len(lines) && lines[next].kind == ' ' {
			next++
		}
		if next == len(lines) || next-end > 2*context {
			return end
		}
		end = next
	}
}

// hunkRange formats one side of a hunk header. An empty side points at the
// line before the hunk, as `diff -u` does for pure additions and deletions.
func hunkRange(start, count int) string {
	if count == 0 {
		return fmt.Sprintf("%d,0", start-1)
	}
	return fmt.Sprintf("%d,%d", start, count)
}

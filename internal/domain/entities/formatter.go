package entities

import "strings"

// Formatter provides styled output rendering. The domain defines the interface;
// infrastructure provides the colored implementation. Commands that need styled
// output accept this interface and fall back to plain text when nil.
type Formatter interface {
	StatusTag(passed bool) string
	DiffSymbol(direction string) string
	Bold(text string) string
	Subtle(text string) string
	FilePath(text string) string
	Success(text string) string
	Warning(text string) string
	Error(text string) string
	// DiffLine colors one line of a unified diff by its prefix: additions,
	// removals, hunk headers, and file headers.
	DiffLine(line string) string
}

// PlainFormatter provides no styling — used when no formatter is injected.
type PlainFormatter struct{}

func (f *PlainFormatter) StatusTag(passed bool) string {
	if passed {
		return "[PASS]"
	}
	return "[FAIL]"
}
func (f *PlainFormatter) DiffSymbol(direction string) string { return direction }
func (f *PlainFormatter) DiffLine(line string) string        { return line }
func (f *PlainFormatter) Bold(text string) string            { return text }
func (f *PlainFormatter) Subtle(text string) string          { return text }
func (f *PlainFormatter) FilePath(text string) string        { return text }
func (f *PlainFormatter) Success(text string) string         { return text }
func (f *PlainFormatter) Warning(text string) string         { return text }
func (f *PlainFormatter) Error(text string) string           { return text }

// ColorizeDiff colors every line of a unified diff with the formatter.
func ColorizeDiff(diff string, formatter Formatter) string {
	var b strings.Builder
	for _, line := range strings.SplitAfter(diff, "\n") {
		if line == "" {
			continue // the empty remainder after a final line break is not a line
		}
		text, hasNewline := strings.CutSuffix(line, "\n")
		b.WriteString(formatter.DiffLine(text))
		if hasNewline {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

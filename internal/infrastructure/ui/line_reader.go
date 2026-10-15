package ui

import (
	"bufio"
	"io"
)

// lineReader hands the lines of one input to every non-interactive prompt in
// turn. A scanner reads ahead, so a scanner per prompt would swallow the
// answers a script piped in for the prompts after it.
type lineReader struct {
	scanner *bufio.Scanner
}

// newLineReader creates a lineReader over input.
func newLineReader(input io.Reader) *lineReader {
	return &lineReader{scanner: bufio.NewScanner(input)}
}

// next returns the next line, without its line ending, and whether one was
// read before the end of the input.
func (r *lineReader) next() (string, bool) {
	if !r.scanner.Scan() {
		return "", false
	}
	return r.scanner.Text(), true
}

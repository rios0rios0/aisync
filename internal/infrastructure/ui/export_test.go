package ui

import "io"

// NewHuhPromptServiceReading creates a HuhPromptService that never uses the
// interactive forms and reads its answers from input instead of stdin.
func NewHuhPromptServiceReading(input io.Reader) *HuhPromptService {
	return &HuhPromptService{lines: newLineReader(input), interactive: func() bool { return false }}
}

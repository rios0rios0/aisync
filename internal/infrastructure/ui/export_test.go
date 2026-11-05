package ui

import (
	"io"

	tea "charm.land/bubbletea/v2"
)

// NewHuhPromptServiceReading creates a HuhPromptService that never uses the
// interactive forms and reads its answers from input instead of stdin.
func NewHuhPromptServiceReading(input io.Reader) *HuhPromptService {
	return &HuhPromptService{lines: newLineReader(input), interactive: func() bool { return false }}
}

// NewDiffModel creates the Bubble Tea model that pages content, the one the
// diff viewer and the conflict prompt run.
func NewDiffModel(content string) tea.Model {
	return newDiffModel(content)
}

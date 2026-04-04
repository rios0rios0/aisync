package services

import (
	"bytes"
	"fmt"
)

const defaultSeparator = "<!-- aisync: personal content below -->"

// SectionMerger merges free-form markdown files (e.g., CLAUDE.md, AGENTS.md)
// by concatenating shared sources and preserving personal content below a separator.
type SectionMerger struct {
	separator string
}

// NewSectionMerger creates a new SectionMerger with the default separator.
func NewSectionMerger() *SectionMerger {
	return &SectionMerger{separator: defaultSeparator}
}

// Merge concatenates all shared sources with newlines, then appends personal
// content below the separator. If the shared content already contains the
// separator from a previous sync, only the shared section (above the separator)
// is replaced while the personal section (below) is kept intact.
func (m *SectionMerger) Merge(sharedSources [][]byte, personal []byte) ([]byte, error) {
	var shared bytes.Buffer
	for i, source := range sharedSources {
		if i > 0 {
			shared.WriteByte('\n')
		}
		shared.Write(source)
	}

	sharedContent := shared.Bytes()

	if len(personal) == 0 {
		return sharedContent, nil
	}

	personalContent := extractPersonalContent(personal, m.separator)

	var result bytes.Buffer
	result.Write(bytes.TrimRight(sharedContent, "\n"))
	result.WriteByte('\n')
	result.WriteByte('\n')
	result.WriteString(m.separator)
	result.WriteByte('\n')
	result.WriteByte('\n')
	result.Write(bytes.TrimRight(personalContent, "\n"))
	result.WriteByte('\n')

	return result.Bytes(), nil
}

// extractPersonalContent returns the personal section from existing content.
// If the separator exists in the content, only the part below the separator is
// returned. Otherwise, the entire content is treated as personal.
func extractPersonalContent(content []byte, separator string) []byte {
	sepBytes := []byte(separator)
	idx := bytes.Index(content, sepBytes)
	if idx == -1 {
		return content
	}

	after := content[idx+len(sepBytes):]
	trimmed := bytes.TrimLeft(after, "\n")
	if len(trimmed) == 0 {
		return nil
	}

	return trimmed
}

// String returns a human-readable description of this merger for logging.
func (m *SectionMerger) String() string {
	return fmt.Sprintf("SectionMerger{separator=%q}", m.separator)
}

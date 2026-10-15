package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// LipglossFormatter provides colored terminal output using lipgloss.
// It implements entities.Formatter.
type LipglossFormatter struct{}

// NewLipglossFormatter creates a new LipglossFormatter.
func NewLipglossFormatter() *LipglossFormatter {
	return &LipglossFormatter{}
}

//nolint:gochecknoglobals // lipgloss styles are compile-time constants
var (
	passStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2"))
	failStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1"))
	addedStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	modifiedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	removedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	unchangedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	boldStyle      = lipgloss.NewStyle().Bold(true)
	subtleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	fileStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	successStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	warnStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	errorStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1"))
)

func (f *LipglossFormatter) StatusTag(passed bool) string {
	if passed {
		return passStyle.Render("[PASS]")
	}
	return failStyle.Render("[FAIL]")
}

func (f *LipglossFormatter) DiffSymbol(direction string) string {
	switch direction {
	case "+":
		return addedStyle.Render("+")
	case "~":
		return modifiedStyle.Render("~")
	case "-":
		return removedStyle.Render("-")
	case "=":
		return unchangedStyle.Render("=")
	default:
		return direction
	}
}

func (f *LipglossFormatter) Bold(text string) string     { return boldStyle.Render(text) }
func (f *LipglossFormatter) Subtle(text string) string   { return subtleStyle.Render(text) }
func (f *LipglossFormatter) FilePath(text string) string { return fileStyle.Render(text) }
func (f *LipglossFormatter) Success(text string) string  { return successStyle.Render(text) }
func (f *LipglossFormatter) Warning(text string) string  { return warnStyle.Render(text) }
func (f *LipglossFormatter) Error(text string) string    { return errorStyle.Render(text) }

// DiffLine colors one unified-diff line: file headers bold, hunk headers
// cyan, additions green, removals red, context unchanged.
func (f *LipglossFormatter) DiffLine(line string) string {
	switch {
	case strings.HasPrefix(line, "+++ "), strings.HasPrefix(line, "--- "):
		return boldStyle.Render(line)
	case strings.HasPrefix(line, "@@"):
		return fileStyle.Render(line)
	case strings.HasPrefix(line, "+"):
		return addedStyle.Render(line)
	case strings.HasPrefix(line, "-"):
		return removedStyle.Render(line)
	default:
		return line
	}
}

package entities

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
func (f *PlainFormatter) Bold(text string) string            { return text }
func (f *PlainFormatter) Subtle(text string) string          { return text }
func (f *PlainFormatter) FilePath(text string) string        { return text }
func (f *PlainFormatter) Success(text string) string         { return text }
func (f *PlainFormatter) Warning(text string) string         { return text }
func (f *PlainFormatter) Error(text string) string           { return text }

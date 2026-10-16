package entities_test

import (
	"testing"

	"github.com/rios0rios0/aisync/internal/domain/entities"

	"github.com/stretchr/testify/assert"
)

func TestPlainFormatter_StatusTag_Passed(t *testing.T) {
	t.Parallel()

	// given
	f := &entities.PlainFormatter{}

	// when
	result := f.StatusTag(true)

	// then
	assert.Equal(t, "[PASS]", result)
}

func TestPlainFormatter_StatusTag_Failed(t *testing.T) {
	t.Parallel()

	// given
	f := &entities.PlainFormatter{}

	// when
	result := f.StatusTag(false)

	// then
	assert.Equal(t, "[FAIL]", result)
}

func TestPlainFormatter_DiffSymbol_ReturnsUnstyled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		direction string
	}{
		{name: "added", direction: "+"},
		{name: "modified", direction: "~"},
		{name: "removed", direction: "-"},
		{name: "custom", direction: "?"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// given
			f := &entities.PlainFormatter{}

			// when
			result := f.DiffSymbol(tt.direction)

			// then
			assert.Equal(t, tt.direction, result)
		})
	}
}

func TestPlainFormatter_Bold_ReturnsUnstyledText(t *testing.T) {
	t.Parallel()

	// given
	f := &entities.PlainFormatter{}

	// when
	result := f.Bold("important text")

	// then
	assert.Equal(t, "important text", result)
}

func TestPlainFormatter_Subtle_ReturnsUnstyledText(t *testing.T) {
	t.Parallel()

	// given
	f := &entities.PlainFormatter{}

	// when
	result := f.Subtle("dimmed text")

	// then
	assert.Equal(t, "dimmed text", result)
}

func TestPlainFormatter_FilePath_ReturnsUnstyledText(t *testing.T) {
	t.Parallel()

	// given
	f := &entities.PlainFormatter{}

	// when
	result := f.FilePath("/home/user/.claude/rules/test.md")

	// then
	assert.Equal(t, "/home/user/.claude/rules/test.md", result)
}

func TestPlainFormatter_Success_ReturnsUnstyledText(t *testing.T) {
	t.Parallel()

	// given
	f := &entities.PlainFormatter{}

	// when
	result := f.Success("operation completed")

	// then
	assert.Equal(t, "operation completed", result)
}

func TestPlainFormatter_Warning_ReturnsUnstyledText(t *testing.T) {
	t.Parallel()

	// given
	f := &entities.PlainFormatter{}

	// when
	result := f.Warning("something is off")

	// then
	assert.Equal(t, "something is off", result)
}

func TestPlainFormatter_Error_ReturnsUnstyledText(t *testing.T) {
	t.Parallel()

	// given
	f := &entities.PlainFormatter{}

	// when
	result := f.Error("something failed")

	// then
	assert.Equal(t, "something failed", result)
}

func TestPlainFormatter_ImplementsFormatterInterface(t *testing.T) {
	t.Parallel()

	// given / when
	var f entities.Formatter = &entities.PlainFormatter{}

	// then
	assert.NotNil(t, f)
}

func TestPlainFormatter_EmptyStringInputs(t *testing.T) {
	t.Parallel()

	// given
	f := &entities.PlainFormatter{}

	// when / then
	assert.Empty(t, f.Bold(""))
	assert.Empty(t, f.Subtle(""))
	assert.Empty(t, f.FilePath(""))
	assert.Empty(t, f.Success(""))
	assert.Empty(t, f.Warning(""))
	assert.Empty(t, f.Error(""))
	assert.Empty(t, f.DiffSymbol(""))
}

// bracketFormatter marks every diff line it colors, so tests can see which
// lines ColorizeDiff passed through the formatter.
type bracketFormatter struct {
	entities.PlainFormatter
}

func (f *bracketFormatter) DiffLine(line string) string { return "[" + line + "]" }

func TestColorizeDiff(t *testing.T) {
	t.Parallel()

	t.Run("should color every line and keep the line breaks", func(t *testing.T) {
		t.Parallel()
		// given
		diff := "--- repo: a.md\n+++ local: a.md\n@@ -1 +1 @@\n-old\n+new\n"

		// when
		result := entities.ColorizeDiff(diff, &bracketFormatter{})

		// then
		assert.Equal(t, "[--- repo: a.md]\n[+++ local: a.md]\n[@@ -1 +1 @@]\n[-old]\n[+new]\n", result)
	})

	t.Run("should color a final line that has no line break", func(t *testing.T) {
		t.Parallel()
		// given
		diff := "+only"

		// when
		result := entities.ColorizeDiff(diff, &bracketFormatter{})

		// then
		assert.Equal(t, "[+only]", result)
	})

	t.Run("should leave lines untouched with the plain formatter", func(t *testing.T) {
		t.Parallel()
		// given
		diff := "-a\n+b\n"

		// when
		result := entities.ColorizeDiff(diff, &entities.PlainFormatter{})

		// then
		assert.Equal(t, diff, result)
	})
}

package ui_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	ui "github.com/rios0rios0/aisync/internal/infrastructure/ui"
)

func TestLipglossFormatter_StatusTag_ShouldReturnPassWhenTrue(t *testing.T) {
	t.Parallel()

	// given
	f := ui.NewLipglossFormatter()

	// when
	result := f.StatusTag(true)

	// then
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "PASS")
}

func TestLipglossFormatter_StatusTag_ShouldReturnFailWhenFalse(t *testing.T) {
	t.Parallel()

	// given
	f := ui.NewLipglossFormatter()

	// when
	result := f.StatusTag(false)

	// then
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "FAIL")
}

func TestLipglossFormatter_StatusTag_ShouldReturnDifferentStringsForTrueAndFalse(t *testing.T) {
	t.Parallel()

	// given
	f := ui.NewLipglossFormatter()

	// when
	passResult := f.StatusTag(true)
	failResult := f.StatusTag(false)

	// then
	assert.NotEqual(t, passResult, failResult)
}

func TestLipglossFormatter_DiffSymbol_ShouldReturnStyledPlusForAdded(t *testing.T) {
	t.Parallel()

	// given
	f := ui.NewLipglossFormatter()

	// when
	result := f.DiffSymbol("+")

	// then
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "+")
}

func TestLipglossFormatter_DiffSymbol_ShouldReturnStyledTildeForModified(t *testing.T) {
	t.Parallel()

	// given
	f := ui.NewLipglossFormatter()

	// when
	result := f.DiffSymbol("~")

	// then
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "~")
}

func TestLipglossFormatter_DiffSymbol_ShouldReturnStyledMinusForRemoved(t *testing.T) {
	t.Parallel()

	// given
	f := ui.NewLipglossFormatter()

	// when
	result := f.DiffSymbol("-")

	// then
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "-")
}

func TestLipglossFormatter_DiffSymbol_ShouldReturnStyledEqualsForUnchanged(t *testing.T) {
	t.Parallel()

	// given
	f := ui.NewLipglossFormatter()

	// when
	result := f.DiffSymbol("=")

	// then
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "=")
}

func TestLipglossFormatter_DiffSymbol_ShouldReturnUnknownDirectionAsIs(t *testing.T) {
	t.Parallel()

	// given
	f := ui.NewLipglossFormatter()

	// when
	result := f.DiffSymbol("?")

	// then
	assert.Equal(t, "?", result)
}

func TestLipglossFormatter_Bold_ShouldReturnNonEmptyString(t *testing.T) {
	t.Parallel()

	// given
	f := ui.NewLipglossFormatter()

	// when
	result := f.Bold("test text")

	// then
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "test text")
}

func TestLipglossFormatter_Subtle_ShouldReturnNonEmptyString(t *testing.T) {
	t.Parallel()

	// given
	f := ui.NewLipglossFormatter()

	// when
	result := f.Subtle("subtle text")

	// then
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "subtle text")
}

func TestLipglossFormatter_FilePath_ShouldReturnNonEmptyString(t *testing.T) {
	t.Parallel()

	// given
	f := ui.NewLipglossFormatter()

	// when
	result := f.FilePath("/path/to/file.md")

	// then
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "/path/to/file.md")
}

func TestLipglossFormatter_Success_ShouldReturnNonEmptyString(t *testing.T) {
	t.Parallel()

	// given
	f := ui.NewLipglossFormatter()

	// when
	result := f.Success("operation succeeded")

	// then
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "operation succeeded")
}

func TestLipglossFormatter_Warning_ShouldReturnNonEmptyString(t *testing.T) {
	t.Parallel()

	// given
	f := ui.NewLipglossFormatter()

	// when
	result := f.Warning("something might be wrong")

	// then
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "something might be wrong")
}

func TestLipglossFormatter_Error_ShouldReturnNonEmptyString(t *testing.T) {
	t.Parallel()

	// given
	f := ui.NewLipglossFormatter()

	// when
	result := f.Error("critical failure")

	// then
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "critical failure")
}

func TestLipglossFormatter_DiffLine(t *testing.T) {
	t.Parallel()

	t.Run("should keep the text of every kind of unified diff line", func(t *testing.T) {
		t.Parallel()
		// given
		f := ui.NewLipglossFormatter()
		lines := []string{
			"--- repo: claude/rules/go.md",
			"+++ local: claude/rules/go.md",
			"@@ -1,2 +1,2 @@",
			"+added",
			"-removed",
			" context",
		}

		for _, line := range lines {
			// when
			result := f.DiffLine(line)

			// then
			assert.Contains(t, result, line)
		}
	})

	t.Run("should return a context line unchanged", func(t *testing.T) {
		t.Parallel()
		// given
		f := ui.NewLipglossFormatter()

		// when
		result := f.DiffLine(" unchanged context")

		// then
		assert.Equal(t, " unchanged context", result)
	})
}

//go:build unit

package entities_test

import (
	"testing"
	"github.com/rios0rios0/aisync/internal/domain/entities"

	"github.com/stretchr/testify/assert"
)

func TestPlainFormatter_StatusTag_Passed(t *testing.T) {
	// given
	f := &entities.PlainFormatter{}

	// when
	result := f.StatusTag(true)

	// then
	assert.Equal(t, "[PASS]", result)
}

func TestPlainFormatter_StatusTag_Failed(t *testing.T) {
	// given
	f := &entities.PlainFormatter{}

	// when
	result := f.StatusTag(false)

	// then
	assert.Equal(t, "[FAIL]", result)
}

func TestPlainFormatter_DiffSymbol_ReturnsUnstyled(t *testing.T) {
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
	// given
	f := &entities.PlainFormatter{}

	// when
	result := f.Bold("important text")

	// then
	assert.Equal(t, "important text", result)
}

func TestPlainFormatter_Subtle_ReturnsUnstyledText(t *testing.T) {
	// given
	f := &entities.PlainFormatter{}

	// when
	result := f.Subtle("dimmed text")

	// then
	assert.Equal(t, "dimmed text", result)
}

func TestPlainFormatter_FilePath_ReturnsUnstyledText(t *testing.T) {
	// given
	f := &entities.PlainFormatter{}

	// when
	result := f.FilePath("/home/user/.claude/rules/test.md")

	// then
	assert.Equal(t, "/home/user/.claude/rules/test.md", result)
}

func TestPlainFormatter_Success_ReturnsUnstyledText(t *testing.T) {
	// given
	f := &entities.PlainFormatter{}

	// when
	result := f.Success("operation completed")

	// then
	assert.Equal(t, "operation completed", result)
}

func TestPlainFormatter_Warning_ReturnsUnstyledText(t *testing.T) {
	// given
	f := &entities.PlainFormatter{}

	// when
	result := f.Warning("something is off")

	// then
	assert.Equal(t, "something is off", result)
}

func TestPlainFormatter_Error_ReturnsUnstyledText(t *testing.T) {
	// given
	f := &entities.PlainFormatter{}

	// when
	result := f.Error("something failed")

	// then
	assert.Equal(t, "something failed", result)
}

func TestPlainFormatter_ImplementsFormatterInterface(t *testing.T) {
	// given / when
	var f entities.Formatter = &entities.PlainFormatter{}

	// then
	assert.NotNil(t, f)
}

func TestPlainFormatter_EmptyStringInputs(t *testing.T) {
	// given
	f := &entities.PlainFormatter{}

	// when / then
	assert.Equal(t, "", f.Bold(""))
	assert.Equal(t, "", f.Subtle(""))
	assert.Equal(t, "", f.FilePath(""))
	assert.Equal(t, "", f.Success(""))
	assert.Equal(t, "", f.Warning(""))
	assert.Equal(t, "", f.Error(""))
	assert.Equal(t, "", f.DiffSymbol(""))
}

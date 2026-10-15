package ui_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/infrastructure/ui"
)

func TestHuhPromptService_PromptSyncConflict_WithoutTerminal(t *testing.T) {
	t.Parallel()

	detail := entities.ConflictDetail{Tool: "claude", Path: "rules/go.md", Diff: "-old\n+new\n"}

	t.Run("should read each piped answer for the next conflict in turn", func(t *testing.T) {
		t.Parallel()
		// given
		prompt := ui.NewHuhPromptServiceReading(strings.NewReader("d\nl\nL\nr\n"))

		// when
		first := prompt.PromptSyncConflict(detail)
		second := prompt.PromptSyncConflict(detail)
		third := prompt.PromptSyncConflict(detail)

		// then
		assert.Equal(t, entities.ChoiceKeepLocal, first, "the diff request does not consume the answer")
		assert.Equal(t, entities.ChoiceKeepLocalAll, second)
		assert.Equal(t, entities.ChoiceKeepRemote, third)
	})

	t.Run("should skip once the input ends", func(t *testing.T) {
		t.Parallel()
		// given
		prompt := ui.NewHuhPromptServiceReading(strings.NewReader("r\n"))

		// when
		first := prompt.PromptSyncConflict(detail)
		second := prompt.PromptSyncConflict(detail)

		// then
		assert.Equal(t, entities.ChoiceKeepRemote, first)
		assert.Equal(t, entities.ChoiceSkip, second)
	})

	t.Run("should share the input with the other prompts", func(t *testing.T) {
		t.Parallel()
		// given
		prompt := ui.NewHuhPromptServiceReading(strings.NewReader("y\nq\n"))

		// when
		confirmed := prompt.PromptConfirmation("Delete it?")
		choice := prompt.PromptSyncConflict(detail)

		// then
		assert.True(t, confirmed)
		assert.Equal(t, entities.ChoiceQuit, choice)
	})
}

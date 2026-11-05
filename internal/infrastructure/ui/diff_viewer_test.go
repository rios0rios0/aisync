package ui_test

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/infrastructure/ui"
)

// pagedContent numbers ten lines, more than any terminal the tests size the
// pager to, so a test can tell which part of the content the pager shows.
func pagedContent() string {
	const lineCount = 10
	var b strings.Builder
	for i := 1; i <= lineCount; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	return b.String()
}

// shownLines splits a frame into its lines, without the spaces the viewport
// pads them with to fill its width.
func shownLines(view tea.View) []string {
	lines := strings.Split(view.Content, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return lines
}

func TestDiffModel_View_DrawsTheAlternateScreen(t *testing.T) {
	t.Parallel()

	t.Run("should show a placeholder when the terminal size has not arrived yet", func(t *testing.T) {
		t.Parallel()
		// given
		model := ui.NewDiffModel(pagedContent())

		// when
		view := model.View()

		// then
		assert.True(t, view.AltScreen)
		assert.Equal(t, "Loading...", view.Content)
	})

	t.Run("should fill the terminal with the content above the key help when the size arrives", func(t *testing.T) {
		t.Parallel()
		// given
		model, _ := ui.NewDiffModel(pagedContent()).Update(tea.WindowSizeMsg{Width: 20, Height: 4})

		// when
		view := model.View()

		// then
		assert.True(t, view.AltScreen)
		lines := shownLines(view)
		require.Len(t, lines, 4)
		assert.Equal(t, []string{"line 1", "line 2", "line 3"}, lines[:3])
		assert.Contains(t, lines[3], "q quit")
	})
}

func TestDiffModel_Update_HandlesKeysAndResizes(t *testing.T) {
	t.Parallel()

	quitKeys := map[string]tea.KeyPressMsg{
		"q":      {Code: 'q', Text: "q"},
		"esc":    {Code: tea.KeyEscape},
		"ctrl+c": {Code: 'c', Mod: tea.ModCtrl},
	}
	for name, key := range quitKeys {
		t.Run("should quit when "+name+" is pressed", func(t *testing.T) {
			t.Parallel()
			// given
			model := ui.NewDiffModel(pagedContent())

			// when
			_, cmd := model.Update(key)

			// then
			require.NotNil(t, cmd)
			assert.Equal(t, tea.QuitMsg{}, cmd())
		})
	}

	t.Run("should scroll down one line when the down arrow is pressed", func(t *testing.T) {
		t.Parallel()
		// given
		model, _ := ui.NewDiffModel(pagedContent()).Update(tea.WindowSizeMsg{Width: 20, Height: 4})

		// when
		model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})

		// then
		assert.Nil(t, cmd)
		assert.Equal(t, []string{"line 2", "line 3", "line 4"}, shownLines(model.View())[:3])
	})

	t.Run("should resize the page when the terminal is resized", func(t *testing.T) {
		t.Parallel()
		// given
		model, _ := ui.NewDiffModel(pagedContent()).Update(tea.WindowSizeMsg{Width: 20, Height: 4})

		// when
		model, _ = model.Update(tea.WindowSizeMsg{Width: 20, Height: 6})

		// then
		lines := shownLines(model.View())
		require.Len(t, lines, 6)
		assert.Equal(t, "line 5", lines[4])
		assert.Contains(t, lines[5], "q quit")
	})
}

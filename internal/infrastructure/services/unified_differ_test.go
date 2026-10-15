package services_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rios0rios0/aisync/internal/infrastructure/services"
)

func TestUnifiedDiffer_Unified(t *testing.T) {
	t.Parallel()

	t.Run("should return an empty string when the contents are identical", func(t *testing.T) {
		t.Parallel()
		// given
		differ := services.NewUnifiedDiffer()
		content := []byte("a\nb\n")

		// when
		diff := differ.Unified("repo", "local", content, content)

		// then
		assert.Empty(t, diff)
	})

	t.Run("should render a changed line with surrounding context", func(t *testing.T) {
		t.Parallel()
		// given
		differ := services.NewUnifiedDiffer()
		from := []byte("one\ntwo\nthree\nfour\nfive\n")
		to := []byte("one\ntwo\nTHREE\nfour\nfive\n")

		// when
		diff := differ.Unified("repo: rules/go.md", "local: rules/go.md", from, to)

		// then
		expected := "--- repo: rules/go.md\n+++ local: rules/go.md\n" +
			"@@ -1,5 +1,5 @@\n one\n two\n-three\n+THREE\n four\n five\n"
		assert.Equal(t, expected, diff)
	})

	t.Run("should describe a new file as a pure addition", func(t *testing.T) {
		t.Parallel()
		// given
		differ := services.NewUnifiedDiffer()

		// when
		diff := differ.Unified("repo", "local", nil, []byte("x\ny\n"))

		// then
		assert.Equal(t, "--- repo\n+++ local\n@@ -0,0 +1,2 @@\n+x\n+y\n", diff)
	})

	t.Run("should describe a deleted file as a pure removal", func(t *testing.T) {
		t.Parallel()
		// given
		differ := services.NewUnifiedDiffer()

		// when
		diff := differ.Unified("repo", "local", []byte("x\ny\n"), nil)

		// then
		assert.Equal(t, "--- repo\n+++ local\n@@ -1,2 +0,0 @@\n-x\n-y\n", diff)
	})

	t.Run("should split distant changes into separate hunks", func(t *testing.T) {
		t.Parallel()
		// given
		differ := services.NewUnifiedDiffer()
		lines := make([]string, 20)
		for i := range lines {
			lines[i] = fmt.Sprintf("line %02d", i+1)
		}
		from := []byte(strings.Join(lines, "\n") + "\n")
		lines[1], lines[18] = "first change", "second change"
		to := []byte(strings.Join(lines, "\n") + "\n")

		// when
		diff := differ.Unified("repo", "local", from, to)

		// then
		assert.Equal(t, 2, strings.Count(diff, "\n@@ "))
		assert.Contains(t, diff, "@@ -1,5 +1,5 @@\n")
		assert.Contains(t, diff, "@@ -16,5 +16,5 @@\n")
	})

	t.Run("should merge changes separated by little context into one hunk", func(t *testing.T) {
		t.Parallel()
		// given
		differ := services.NewUnifiedDiffer()
		from := []byte("a\nb\nc\nd\ne\nf\ng\n")
		to := []byte("A\nb\nc\nd\ne\nf\nG\n")

		// when
		diff := differ.Unified("repo", "local", from, to)

		// then
		assert.Equal(t, 1, strings.Count(diff, "@@ -"))
		assert.Contains(t, diff, "@@ -1,7 +1,7 @@\n-a\n+A\n")
		assert.True(t, strings.HasSuffix(diff, "-g\n+G\n"))
	})

	t.Run("should keep a final line that has no trailing newline", func(t *testing.T) {
		t.Parallel()
		// given
		differ := services.NewUnifiedDiffer()

		// when
		diff := differ.Unified("repo", "local", []byte("same\nold"), []byte("same\nnew"))

		// then
		assert.Contains(t, diff, "-old\n+new\n")
	})
}

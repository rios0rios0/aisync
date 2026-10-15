package entities_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

func TestTranscriptSpec_Classify(t *testing.T) {
	t.Parallel()

	spec := entities.TranscriptSpec{
		Dir:   "projects",
		Logs:  []string{"*/*.jsonl"},
		Files: []string{"*/memory/**"},
	}

	t.Run("should classify a project's main transcript as a log", func(t *testing.T) {
		t.Parallel()
		// given
		rel := "-home-user-Development-api/0e1f.jsonl"

		// when
		kind := spec.Classify(rel)

		// then
		assert.Equal(t, entities.TranscriptKindLog, kind)
	})

	t.Run("should classify a project memory as a file", func(t *testing.T) {
		t.Parallel()
		// given
		rel := "-home-user-Development-api/memory/MEMORY.md"

		// when
		kind := spec.Classify(rel)

		// then
		assert.Equal(t, entities.TranscriptKindFile, kind)
	})

	t.Run("should leave subagent logs and tool output out of scope", func(t *testing.T) {
		t.Parallel()
		// given
		paths := []string{
			"-home-user-Development-api/0e1f/subagents/agent-1.jsonl",
			"-home-user-Development-api/0e1f/tool-results/output.txt",
			"loose.jsonl",
		}

		for _, rel := range paths {
			// when
			kind := spec.Classify(rel)

			// then
			assert.Equal(t, entities.TranscriptKindNone, kind, rel)
		}
	})
}

func TestTranscriptSpec_MaxAgeDuration(t *testing.T) {
	t.Parallel()

	t.Run("should parse whole days", func(t *testing.T) {
		t.Parallel()
		// given
		spec := entities.TranscriptSpec{MaxAge: "14d"}

		// when
		duration, err := spec.MaxAgeDuration()

		// then
		require.NoError(t, err)
		assert.Equal(t, 14*24*time.Hour, duration)
	})

	t.Run("should parse a Go duration", func(t *testing.T) {
		t.Parallel()
		// given
		spec := entities.TranscriptSpec{MaxAge: "36h"}

		// when
		duration, err := spec.MaxAgeDuration()

		// then
		require.NoError(t, err)
		assert.Equal(t, 36*time.Hour, duration)
	})

	t.Run("should never expire when empty", func(t *testing.T) {
		t.Parallel()
		// given
		spec := entities.TranscriptSpec{}

		// when
		duration, err := spec.MaxAgeDuration()

		// then
		require.NoError(t, err)
		assert.Zero(t, duration)
	})

	t.Run("should return an error for an unreadable value", func(t *testing.T) {
		t.Parallel()
		// given
		values := []string{"two weeks", "-3d", "1.5d"}

		for _, value := range values {
			// when
			_, err := entities.TranscriptSpec{MaxAge: value}.MaxAgeDuration()

			// then
			assert.Error(t, err, value)
		}
	})
}

func TestSyncConfig_EffectiveTranscriptsBranch(t *testing.T) {
	t.Parallel()

	t.Run("should default to the transcripts branch", func(t *testing.T) {
		t.Parallel()
		// given
		config := entities.SyncConfig{}

		// when
		branch := config.EffectiveTranscriptsBranch()

		// then
		assert.Equal(t, entities.DefaultTranscriptsBranch, branch)
	})

	t.Run("should use the configured branch", func(t *testing.T) {
		t.Parallel()
		// given
		config := entities.SyncConfig{TranscriptsBranch: "conversations"}

		// when
		branch := config.EffectiveTranscriptsBranch()

		// then
		assert.Equal(t, "conversations", branch)
	})
}

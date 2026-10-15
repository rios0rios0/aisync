package commands_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/domain/commands"
	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/test/doubles"
)

// transcriptFixture is a Claude-like tool directory with a projects/ folder,
// an in-memory snapshot branch, and the doubles a TranscriptSyncer needs.
type transcriptFixture struct {
	t        *testing.T
	toolDir  string
	repoPath string
	config   *entities.Config
	service  *doubles.MockTranscriptService
	base     *doubles.MockTranscriptBaseRepository
	apply    *doubles.MockApplyService
	prompt   *doubles.MockPromptService
}

func newTranscriptFixture(t *testing.T) *transcriptFixture {
	t.Helper()
	root := t.TempDir()
	f := &transcriptFixture{
		t:        t,
		toolDir:  filepath.Join(root, "claude-home"),
		repoPath: filepath.Join(root, "repo"),
		service:  &doubles.MockTranscriptService{},
		base:     &doubles.MockTranscriptBaseRepository{Base: map[string]string{}},
		apply:    &doubles.MockApplyService{WriteOnApply: true},
		prompt:   &doubles.MockPromptService{},
	}
	require.NoError(t, os.MkdirAll(filepath.Join(f.toolDir, "projects"), 0o700))
	f.config = &entities.Config{
		Tools: map[string]entities.Tool{"claude": {
			Path:    f.toolDir,
			Enabled: true,
			Transcripts: &entities.TranscriptSpec{
				Dir:    "projects",
				Logs:   []string{"*/*.jsonl"},
				Files:  []string{"*/memory/**"},
				MaxAge: "14d",
			},
		}},
		Encryption: entities.EncryptionConfig{
			Identity:   filepath.Join(root, "key.txt"),
			Recipients: []string{"age1recipient"},
		},
	}
	return f
}

// writeConversation writes rel under projects/ and sets its modification time.
func (f *transcriptFixture) writeConversation(rel, content string, modTime time.Time) {
	f.t.Helper()
	full := filepath.Join(f.toolDir, "projects", rel)
	writeFile(f.t, full, content)
	require.NoError(f.t, os.Chtimes(full, modTime, modTime))
}

func (f *transcriptFixture) readConversation(rel string) (string, bool) {
	content, err := os.ReadFile(filepath.Join(f.toolDir, "projects", rel))
	return string(content), err == nil
}

func (f *transcriptFixture) sync(opts commands.PersonalSyncOptions) (*commands.PersonalSyncReport, error) {
	syncer := commands.NewTranscriptSyncer(f.service, f.base, f.apply, f.prompt, &doubles.MockTextDiffer{})
	return syncer.Sync(f.config, f.repoPath, opts)
}

func TestTranscriptSyncer_Sync(t *testing.T) {
	t.Parallel()

	both := commands.PersonalSyncOptions{Direction: entities.SyncBoth}
	recently := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	key := "claude/-proj/session.jsonl"

	t.Run("should publish a new conversation and record it as synced", func(t *testing.T) {
		t.Parallel()
		// given
		f := newTranscriptFixture(t)
		f.writeConversation("-proj/session.jsonl", "{\"a\":1}\n", recently)

		// when
		report, err := f.sync(both)

		// then
		require.NoError(t, err)
		require.Contains(t, f.service.Files, key)
		assert.Equal(t, []byte("{\"a\":1}\n"), f.service.Blobs[f.service.Files[key].Stored])
		assert.True(t, recently.Equal(f.service.Files[key].ModTime))
		assert.Equal(t, 1, report.Pushed)
		assert.Equal(t, checksumOf("{\"a\":1}\n"), f.base.Base[key])
	})

	t.Run("should bring a conversation from another device with its original timestamp", func(t *testing.T) {
		t.Parallel()
		// given
		f := newTranscriptFixture(t)
		f.service.Seed(key, []byte("{\"from\":\"desktop\"}\n"), recently, "desktop")

		// when
		report, err := f.sync(both)

		// then
		require.NoError(t, err)
		content, ok := f.readConversation("-proj/session.jsonl")
		require.True(t, ok)
		assert.JSONEq(t, "{\"from\":\"desktop\"}\n", content)
		info, statErr := os.Stat(filepath.Join(f.toolDir, "projects", "-proj", "session.jsonl"))
		require.NoError(t, statErr)
		assert.True(t, recently.Equal(info.ModTime()), "a pulled conversation keeps the age it had on the other device")
		assert.Equal(t, 1, report.Pulled)
		assert.Equal(t, 0, f.service.PublishCalls, "nothing changed in the snapshot, so nothing is published")
	})

	t.Run("should write a pulled memory file byte for byte, keeping its CRLF line endings", func(t *testing.T) {
		t.Parallel()
		// given
		f := newTranscriptFixture(t)
		memoryKey := "claude/-proj/memory/MEMORY.md"
		f.service.Seed(memoryKey, []byte("- [Rule](rule.md)\r\n"), recently, "desktop")

		// when
		report, err := f.sync(both)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, report.Pulled)
		assert.Equal(t, 1, f.apply.StageVerbatimCalls)
		content, _ := f.readConversation("-proj/memory/MEMORY.md")
		assert.Equal(t, "- [Rule](rule.md)\r\n", content)
	})

	t.Run("should not publish when only pulling", func(t *testing.T) {
		t.Parallel()
		// given
		f := newTranscriptFixture(t)
		f.writeConversation("-proj/session.jsonl", "local\n", recently)

		// when
		report, err := f.sync(commands.PersonalSyncOptions{Direction: entities.SyncIncoming})

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, f.service.PublishCalls)
		assert.Equal(t, 1, report.PendingOutgoing)
	})

	t.Run(
		"should neither publish nor restore conversations older than max_age, and drop them from the snapshot",
		func(t *testing.T) {
			t.Parallel()
			// given
			f := newTranscriptFixture(t)
			old := time.Now().Add(-20 * 24 * time.Hour)
			f.writeConversation("-old/local.jsonl", "old here\n", old)
			f.service.Seed("claude/-old/remote.jsonl", []byte("old there\n"), old, "desktop")
			f.base.Base["claude/-old/remote.jsonl"] = checksumOf("old there\n")
			f.writeConversation("-new/fresh.jsonl", "fresh\n", recently)

			// when
			_, err := f.sync(both)

			// then
			require.NoError(t, err)
			assert.NotContains(t, f.service.Files, "claude/-old/local.jsonl")
			assert.NotContains(t, f.service.Files, "claude/-old/remote.jsonl", "an expired log leaves the snapshot")
			assert.Contains(t, f.service.Files, "claude/-new/fresh.jsonl")
			_, restored := f.readConversation("-old/remote.jsonl")
			assert.False(t, restored)
			local, kept := f.readConversation("-old/local.jsonl")
			assert.True(t, kept, "expiry never deletes a local conversation")
			assert.Equal(t, "old here\n", local)
			assert.NotContains(t, f.base.Base, "claude/-old/remote.jsonl")
		},
	)

	t.Run("should keep a log alive when another device continued it recently", func(t *testing.T) {
		t.Parallel()
		// given
		f := newTranscriptFixture(t)
		old := time.Now().Add(-20 * 24 * time.Hour)
		f.writeConversation("-proj/session.jsonl", "line1\n", old)
		f.base.Base[key] = checksumOf("line1\n")
		f.service.Seed(key, []byte("line1\nline2\n"), recently, "desktop")

		// when
		report, err := f.sync(both)

		// then
		require.NoError(t, err)
		content, _ := f.readConversation("-proj/session.jsonl")
		assert.Equal(t, "line1\nline2\n", content)
		assert.Equal(t, 1, report.Pulled)
	})

	t.Run("should take the longer copy when one device extended the other's conversation", func(t *testing.T) {
		t.Parallel()
		// given
		f := newTranscriptFixture(t)
		f.base.Base[key] = checksumOf("line1\n")
		f.writeConversation("-proj/session.jsonl", "line1\nline2\n", recently)
		f.service.Seed(key, []byte("line1\nline2\nline3\n"), recently.Add(-time.Hour), "desktop")

		// when
		report, err := f.sync(both)

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, f.prompt.ConflictCalls, "conversation logs never prompt")
		content, _ := f.readConversation("-proj/session.jsonl")
		assert.Equal(t, "line1\nline2\nline3\n", content)
		assert.Equal(t, 1, report.Pulled)
	})

	t.Run("should publish this device's copy when it extends the snapshot's", func(t *testing.T) {
		t.Parallel()
		// given
		f := newTranscriptFixture(t)
		f.base.Base[key] = checksumOf("line1\n")
		f.writeConversation("-proj/session.jsonl", "line1\nline2\nline3\n", recently)
		f.service.Seed(key, []byte("line1\nline2\n"), recently, "desktop")

		// when
		report, err := f.sync(both)

		// then
		require.NoError(t, err)
		assert.Equal(t, []byte("line1\nline2\nline3\n"), f.service.Blobs[f.service.Files[key].Stored])
		assert.Equal(t, 1, report.Pushed)
	})

	t.Run("should keep the newer copy when a conversation diverged between devices", func(t *testing.T) {
		t.Parallel()
		// given
		f := newTranscriptFixture(t)
		f.base.Base[key] = checksumOf("line1\n")
		f.writeConversation("-proj/session.jsonl", "line1\nmine\n", recently.Add(-time.Hour))
		f.service.Seed(key, []byte("line1\ntheirs\n"), recently, "desktop")

		// when
		_, err := f.sync(both)

		// then
		require.NoError(t, err)
		content, _ := f.readConversation("-proj/session.jsonl")
		assert.Equal(t, "line1\ntheirs\n", content)
	})

	t.Run("should review a memory file changed on both devices like any personal file", func(t *testing.T) {
		t.Parallel()
		// given
		f := newTranscriptFixture(t)
		memory := "claude/-proj/memory/MEMORY.md"
		f.base.Base[memory] = checksumOf("- old index\n")
		f.writeConversation("-proj/memory/MEMORY.md", "- mine\n", recently)
		f.service.Seed(memory, []byte("- theirs\n"), recently, "desktop")
		f.prompt.ConflictChoices = []entities.ConflictChoice{entities.ChoiceKeepLocal}

		// when
		report, err := f.sync(both)

		// then
		require.NoError(t, err)
		require.Len(t, f.prompt.ConflictDetails, 1)
		detail := f.prompt.ConflictDetails[0]
		assert.Equal(t, "projects/-proj/memory/MEMORY.md", detail.Path)
		assert.Equal(t, "from desktop", detail.RemoteSummary)
		assert.Contains(t, detail.Diff, "+- mine")
		assert.Equal(t, []byte("- mine\n"), f.service.Blobs[f.service.Files[memory].Stored])
		assert.Equal(t, 1, report.Pushed)
	})

	t.Run("should remove a conversation from the snapshot when it was deleted here", func(t *testing.T) {
		t.Parallel()
		// given
		f := newTranscriptFixture(t)
		f.service.Seed(key, []byte("gone here\n"), recently, "laptop")
		f.base.Base[key] = checksumOf("gone here\n")

		// when
		report, err := f.sync(both)

		// then
		require.NoError(t, err)
		assert.NotContains(t, f.service.Files, key)
		assert.Equal(t, 1, report.DeletedRepo)
		assert.NotContains(t, f.base.Base, key)
	})

	t.Run("should merge again instead of overwriting when another device published first", func(t *testing.T) {
		t.Parallel()
		// given
		f := newTranscriptFixture(t)
		f.writeConversation("-proj/session.jsonl", "mine\n", recently)
		f.service.MoveBeforeNextPublish = true

		// when
		_, err := f.sync(both)

		// then
		require.NoError(t, err)
		assert.Equal(t, 2, f.service.FetchCalls, "the second attempt fetches the newer snapshot")
		assert.Equal(t, 2, f.service.PublishCalls)
		assert.Contains(t, f.service.Files, key)
	})

	t.Run("should keep the snapshot files of tools not synced on this device", func(t *testing.T) {
		t.Parallel()
		// given
		f := newTranscriptFixture(t)
		f.service.Seed("codex/2026/10/rollout.jsonl", []byte("codex\n"), recently, "desktop")
		f.writeConversation("-proj/session.jsonl", "mine\n", recently)

		// when
		_, err := f.sync(both)

		// then
		require.NoError(t, err)
		assert.Contains(t, f.service.Files, "codex/2026/10/rollout.jsonl")
		assert.Contains(t, f.service.Files, key)
	})

	t.Run("should ignore subagent logs and other files outside the spec", func(t *testing.T) {
		t.Parallel()
		// given
		f := newTranscriptFixture(t)
		f.writeConversation("-proj/session-id/subagents/agent-1.jsonl", "subagent\n", recently)
		f.writeConversation("-proj/session-id/tool-results/out.txt", "tool output\n", recently)

		// when
		report, err := f.sync(both)

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, report.Pushed)
		assert.Equal(t, 0, f.service.PublishCalls)
	})

	t.Run("should change nothing in a dry run", func(t *testing.T) {
		t.Parallel()
		// given
		f := newTranscriptFixture(t)
		f.writeConversation("-proj/session.jsonl", "mine\n", recently)
		f.service.Seed("claude/-other/theirs.jsonl", []byte("theirs\n"), recently, "desktop")

		// when
		report, err := f.sync(commands.PersonalSyncOptions{Direction: entities.SyncBoth, DryRun: true})

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, f.service.PublishCalls)
		assert.Equal(t, 0, f.apply.StageCalls)
		assert.Equal(t, 0, f.base.SaveCalls)
		assert.Equal(t, 1, report.Pushed)
		assert.Equal(t, 1, report.Pulled)
	})

	t.Run("should do nothing when no tool syncs its conversations", func(t *testing.T) {
		t.Parallel()
		// given
		f := newTranscriptFixture(t)
		tool := f.config.Tools["claude"]
		tool.Transcripts = nil
		f.config.Tools["claude"] = tool

		// when
		_, err := f.sync(both)

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, f.service.FetchCalls)
	})

	t.Run("should refuse to publish conversations without encryption recipients", func(t *testing.T) {
		t.Parallel()
		// given
		f := newTranscriptFixture(t)
		f.config.Encryption.Recipients = nil
		f.writeConversation("-proj/session.jsonl", "mine\n", recently)

		// when
		_, err := f.sync(both)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "encrypted")
		assert.Equal(t, 0, f.service.PublishCalls)
	})

	t.Run("should return an error for an invalid max_age", func(t *testing.T) {
		t.Parallel()
		// given
		f := newTranscriptFixture(t)
		f.config.Tools["claude"].Transcripts.MaxAge = "two weeks"

		// when
		_, err := f.sync(both)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max_age")
	})
}

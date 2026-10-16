package commands_test

import (
	"crypto/sha256"
	"errors"
	"fmt"
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

// syncFixture is a tool directory, a sync repo, and the doubles a
// PersonalSyncer needs, for one isolated test.
type syncFixture struct {
	t          *testing.T
	toolDir    string
	repoPath   string
	config     *entities.Config
	manifest   *doubles.MockManifestRepository
	encryption *doubles.MockEncryptionService
	apply      *doubles.MockApplyService
	prompt     *doubles.MockPromptService
	history    *doubles.MockRepoHistory
}

func newSyncFixture(t *testing.T) *syncFixture {
	t.Helper()
	root := t.TempDir()
	f := &syncFixture{
		t:        t,
		toolDir:  filepath.Join(root, "claude-home"),
		repoPath: filepath.Join(root, "repo"),
		manifest: &doubles.MockManifestRepository{},
		encryption: &doubles.MockEncryptionService{
			EncryptedData: []byte("ciphertext"),
		},
		apply:   &doubles.MockApplyService{WriteOnApply: true},
		prompt:  &doubles.MockPromptService{},
		history: &doubles.MockRepoHistory{Changes: map[string]doubles.MockRepoChange{}},
	}
	require.NoError(t, os.MkdirAll(f.toolDir, 0o700))
	require.NoError(t, os.MkdirAll(f.repoPath, 0o700))
	f.config = &entities.Config{
		Tools: map[string]entities.Tool{"claude": {Path: f.toolDir, Enabled: true}},
		Encryption: entities.EncryptionConfig{
			Identity:   filepath.Join(root, "key.txt"),
			Recipients: []string{"age1recipient"},
		},
	}
	return f
}

func (f *syncFixture) writeLocal(rel, content string) {
	f.t.Helper()
	writeFile(f.t, filepath.Join(f.toolDir, rel), content)
}

func (f *syncFixture) writeRepo(rel, content string) {
	f.t.Helper()
	writeFile(f.t, filepath.Join(f.repoPath, "personal", "claude", rel), content)
}

// base records content as what this device last synced for rel.
func (f *syncFixture) base(rel, content string) {
	if f.manifest.Manifest == nil {
		f.manifest.Manifest = entities.NewManifest("0.1.0", "host")
		f.manifest.ExistsVal = true
	}
	f.manifest.Manifest.SetFile(rel, "personal", "personal", checksumOf(content))
}

func (f *syncFixture) readLocal(rel string) (string, bool) {
	content, err := os.ReadFile(filepath.Join(f.toolDir, rel))
	return string(content), err == nil
}

func (f *syncFixture) readRepo(rel string) (string, bool) {
	content, err := os.ReadFile(filepath.Join(f.repoPath, "personal", "claude", rel))
	return string(content), err == nil
}

func (f *syncFixture) syncer() *commands.PersonalSyncer {
	return commands.NewPersonalSyncer(f.manifest, f.encryption, f.apply, f.prompt, &doubles.MockTextDiffer{}, f.history)
}

func (f *syncFixture) sync(opts commands.PersonalSyncOptions) (*commands.PersonalSyncReport, error) {
	report, err := f.syncer().Sync(f.config, f.repoPath, opts)
	if err == nil && report != nil {
		// In tests, assume commit/push succeeds, so commit manifests immediately.
		report.CommitManifests()
	}
	return report, err
}

// savedBase returns the checksum the saved manifest records for rel.
func (f *syncFixture) savedBase(rel string) (string, bool) {
	if f.manifest.SavedManifest == nil {
		return "", false
	}
	file, ok := f.manifest.SavedManifest.Files[rel]
	return file.Checksum, ok
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func checksumOf(content string) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(content)))
}

func TestPersonalSyncer_Sync(t *testing.T) {
	t.Parallel()

	outgoing := commands.PersonalSyncOptions{Direction: entities.SyncOutgoing}
	incoming := commands.PersonalSyncOptions{Direction: entities.SyncIncoming}
	both := commands.PersonalSyncOptions{Direction: entities.SyncBoth}

	t.Run("should copy a new local file into the repo and record it as synced", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.writeLocal("skills/review/SKILL.md", "review skill")

		// when
		report, err := f.sync(outgoing)

		// then
		require.NoError(t, err)
		content, ok := f.readRepo("skills/review/SKILL.md")
		require.True(t, ok)
		assert.Equal(t, "review skill", content)
		assert.Equal(t, 1, report.Pushed)
		assert.Equal(t, []byte("review skill"), report.Plaintext["personal/claude/skills/review/SKILL.md"])
		base, recorded := f.savedBase("skills/review/SKILL.md")
		assert.True(t, recorded)
		assert.Equal(t, checksumOf("review skill"), base)
	})

	t.Run("should bring a new repo file into the tool directory and record it as synced", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.writeRepo("agents/reviewer.md", "from the other machine")

		// when
		report, err := f.sync(incoming)

		// then
		require.NoError(t, err)
		content, ok := f.readLocal("agents/reviewer.md")
		require.True(t, ok)
		assert.Equal(t, "from the other machine", content)
		assert.Equal(t, 1, report.Pulled)
		base, _ := f.savedBase("agents/reviewer.md")
		assert.Equal(t, checksumOf("from the other machine"), base)
	})

	t.Run("should leave incoming changes pending when only pushing", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.writeRepo("agents/reviewer.md", "from the other machine")

		// when
		report, err := f.sync(outgoing)

		// then
		require.NoError(t, err)
		_, exists := f.readLocal("agents/reviewer.md")
		assert.False(t, exists)
		assert.Equal(t, 1, report.PendingIncoming)
		assert.Equal(t, 0, f.apply.StageCalls)
	})

	t.Run("should push a local change when the repo copy is unchanged since the last sync", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.base("rules/go.md", "v1")
		f.writeRepo("rules/go.md", "v1")
		f.writeLocal("rules/go.md", "v2 edited here")

		// when
		report, err := f.sync(both)

		// then
		require.NoError(t, err)
		content, _ := f.readRepo("rules/go.md")
		assert.Equal(t, "v2 edited here", content)
		assert.Equal(t, 1, report.Pushed)
		assert.Equal(t, 0, f.prompt.ConflictCalls)
	})

	t.Run("should delete the repo copy when the file was deleted here and is unchanged there", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.base("commands/old.md", "obsolete")
		f.writeRepo("commands/old.md", "obsolete")

		// when
		report, err := f.sync(outgoing)

		// then
		require.NoError(t, err)
		_, exists := f.readRepo("commands/old.md")
		assert.False(t, exists)
		_, statErr := os.Stat(filepath.Join(f.repoPath, "personal", "claude", "commands"))
		assert.True(t, os.IsNotExist(statErr), "the emptied directory is removed too")
		assert.Equal(t, 1, report.DeletedRepo)
		_, recorded := f.savedBase("commands/old.md")
		assert.False(t, recorded)
	})

	t.Run("should delete the local file when the repo deleted it and it is unchanged here", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.base("commands/old.md", "obsolete")
		f.writeLocal("commands/old.md", "obsolete")

		// when
		report, err := f.sync(incoming)

		// then
		require.NoError(t, err)
		_, exists := f.readLocal("commands/old.md")
		assert.False(t, exists)
		assert.Equal(t, 1, report.DeletedLocal)
	})

	t.Run("should prompt with the diff and timestamps and keep the local version when chosen", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.base("settings.json", "{}")
		f.writeRepo("settings.json", `{"theme":"dark"}`)
		f.writeLocal("settings.json", `{"theme":"light"}`)
		committed := time.Date(2026, 10, 12, 3, 26, 0, 0, time.UTC)
		f.history.Changes["personal/claude/settings.json"] = doubles.MockRepoChange{
			When: committed, Subject: "sync(laptop): updated personal configurations",
		}
		f.prompt.ConflictChoices = []entities.ConflictChoice{entities.ChoiceKeepLocal}

		// when
		report, err := f.sync(outgoing)

		// then
		require.NoError(t, err)
		require.Len(t, f.prompt.ConflictDetails, 1)
		detail := f.prompt.ConflictDetails[0]
		assert.Equal(t, "claude", detail.Tool)
		assert.Equal(t, "settings.json", detail.Path)
		assert.Equal(t, entities.ConflictBothModified, detail.Reason)
		assert.True(t, committed.Equal(detail.RemoteModified))
		assert.Equal(t, "sync(laptop): updated personal configurations", detail.RemoteSummary)
		assert.False(t, detail.LocalModified.IsZero())
		assert.Contains(t, detail.Diff, `-{"theme":"dark"}`)
		assert.Contains(t, detail.Diff, `+{"theme":"light"}`)
		content, _ := f.readRepo("settings.json")
		assert.JSONEq(t, `{"theme":"light"}`, content)
		assert.Equal(t, 1, report.Pushed)
	})

	t.Run("should write nothing and keep the base when a conflict is skipped", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.base("settings.json", "{}")
		f.writeRepo("settings.json", "theirs")
		f.writeLocal("settings.json", "mine")
		f.prompt.ConflictChoices = []entities.ConflictChoice{entities.ChoiceSkip}

		// when
		report, err := f.sync(both)

		// then
		require.NoError(t, err)
		local, _ := f.readLocal("settings.json")
		repo, _ := f.readRepo("settings.json")
		assert.Equal(t, "mine", local)
		assert.Equal(t, "theirs", repo)
		assert.Equal(t, 1, report.ConflictsSkipped)
		assert.Equal(t, checksumOf("{}"), f.manifest.Manifest.Files["settings.json"].Checksum)
	})

	t.Run("should abort without writing anything when the user quits at a conflict", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.writeLocal("agents/new.md", "would be pushed")
		f.base("settings.json", "{}")
		f.writeRepo("settings.json", "theirs")
		f.writeLocal("settings.json", "mine")
		f.prompt.ConflictChoices = []entities.ConflictChoice{entities.ChoiceQuit}

		// when
		_, err := f.sync(both)

		// then
		require.ErrorIs(t, err, commands.ErrSyncAborted)
		_, pushed := f.readRepo("agents/new.md")
		assert.False(t, pushed, "nothing is written when the run is aborted")
		assert.Equal(t, 0, f.manifest.SaveCalls)
	})

	t.Run("should apply a keep-local-for-all answer to the remaining conflicts without asking", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		for _, rel := range []string{"rules/a.md", "rules/b.md"} {
			f.base(rel, "base")
			f.writeRepo(rel, "theirs "+rel)
			f.writeLocal(rel, "mine "+rel)
		}
		f.prompt.ConflictChoices = []entities.ConflictChoice{entities.ChoiceKeepLocalAll}

		// when
		report, err := f.sync(both)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, f.prompt.ConflictCalls)
		a, _ := f.readRepo("rules/a.md")
		b, _ := f.readRepo("rules/b.md")
		assert.Equal(t, "mine rules/a.md", a)
		assert.Equal(t, "mine rules/b.md", b)
		assert.Equal(t, 2, report.Pushed)
	})

	t.Run("should keep the side that changed last when preferring the newer version", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.writeRepo("rules/go.md", "repo version")
		f.writeLocal("rules/go.md", "local version")
		old := time.Now().Add(-48 * time.Hour)
		require.NoError(t, os.Chtimes(filepath.Join(f.toolDir, "rules", "go.md"), old, old))
		f.history.Changes["personal/claude/rules/go.md"] = doubles.MockRepoChange{When: time.Now().Add(-time.Hour)}

		// when
		report, err := f.sync(
			commands.PersonalSyncOptions{Direction: entities.SyncBoth, Preference: entities.PreferNewer},
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, f.prompt.ConflictCalls)
		local, _ := f.readLocal("rules/go.md")
		assert.Equal(t, "repo version", local)
		assert.Equal(t, 1, report.Pulled)
	})

	t.Run("should skip conflicts without prompting when prompts are disabled", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.writeRepo("rules/go.md", "repo version")
		f.writeLocal("rules/go.md", "local version")

		// when
		report, err := f.sync(commands.PersonalSyncOptions{Direction: entities.SyncBoth, NoPrompt: true})

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, f.prompt.ConflictCalls)
		assert.Equal(t, 1, report.ConflictsSkipped)
		local, _ := f.readLocal("rules/go.md")
		assert.Equal(t, "local version", local)
	})

	t.Run("should store an encrypt-pattern file as ciphertext and drop any plaintext copy", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		writeFile(t, filepath.Join(f.repoPath, ".aisyncencrypt"), "personal/**/settings.local.json\n")
		f.writeLocal("settings.local.json", `{"token":"x"}`)
		f.writeRepo("settings.local.json", "stale plaintext")
		f.prompt.ConflictChoices = []entities.ConflictChoice{entities.ChoiceKeepLocal}

		// when
		report, err := f.sync(outgoing)

		// then
		require.NoError(t, err)
		encrypted, ok := f.readRepo("settings.local.json.age")
		require.True(t, ok)
		assert.Equal(t, "ciphertext", encrypted)
		_, plaintextLeft := f.readRepo("settings.local.json")
		assert.False(t, plaintextLeft)
		assert.JSONEq(t, `{"token":"x"}`, string(f.encryption.EncryptPlaintext))
		assert.Empty(t, report.Plaintext, "ciphertext is not handed to the content scanners")
	})

	t.Run("should not rewrite an encrypted file whose plaintext is unchanged", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		writeFile(t, filepath.Join(f.repoPath, ".aisyncencrypt"), "personal/**/settings.local.json\n")
		f.writeLocal("settings.local.json", "same")
		f.writeRepo("settings.local.json.age", "older ciphertext of the same plaintext")
		f.encryption.DecryptedData = []byte("same")

		// when
		report, err := f.sync(both)

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, f.encryption.EncryptCalls)
		assert.Equal(t, 0, report.Pushed)
		stored, _ := f.readRepo("settings.local.json.age")
		assert.Equal(t, "older ciphertext of the same plaintext", stored)
	})

	t.Run("should leave a repo file it cannot decrypt untouched on both sides", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		writeFile(t, filepath.Join(f.repoPath, ".aisyncencrypt"), "personal/**/settings.local.json\n")
		f.writeLocal("settings.local.json", "mine")
		f.writeRepo("settings.local.json.age", "encrypted for someone else")
		f.encryption.DecryptErr = errors.New("no identity matched any of the recipients")

		// when
		report, err := f.sync(both)

		// then
		require.NoError(t, err)
		stored, _ := f.readRepo("settings.local.json.age")
		assert.Equal(t, "encrypted for someone else", stored)
		local, _ := f.readLocal("settings.local.json")
		assert.Equal(t, "mine", local)
		assert.Equal(t, 0, report.Pushed+report.Pulled+report.ConflictsSkipped)
	})

	t.Run("should carry the executable bit to the repo and back", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.writeLocal("hooks/guard.sh", "#!/bin/sh\n")
		require.NoError(t, os.Chmod(filepath.Join(f.toolDir, "hooks", "guard.sh"), 0o700))
		f.writeRepo("scripts/tool.sh", "#!/bin/sh\necho hi\n")
		require.NoError(t, os.Chmod(filepath.Join(f.repoPath, "personal", "claude", "scripts", "tool.sh"), 0o700))

		// when
		_, err := f.sync(both)

		// then
		require.NoError(t, err)
		pushed, statErr := os.Stat(filepath.Join(f.repoPath, "personal", "claude", "hooks", "guard.sh"))
		require.NoError(t, statErr)
		assert.NotZero(t, pushed.Mode().Perm()&0o100, "the repo copy of an executable stays executable")
		pulled, statErr := os.Stat(filepath.Join(f.toolDir, "scripts", "tool.sh"))
		require.NoError(t, statErr)
		assert.NotZero(t, pulled.Mode().Perm()&0o100, "a pulled executable is restored as executable")
	})

	t.Run(
		"should ignore paths outside the allowlist, ignored paths, and paths from external sources",
		func(t *testing.T) {
			t.Parallel()
			// given
			f := newSyncFixture(t)
			writeFile(t, filepath.Join(f.repoPath, ".aisyncignore"), "*.tmp\n")
			f.writeLocal("projects/session.jsonl", "transcript")
			f.writeLocal("rules/draft.tmp", "scratch")
			f.writeLocal("rules/shared.md", "from an external source")
			f.manifest.Manifest = entities.NewManifest("0.1.0", "host")
			f.manifest.Manifest.SetFile("rules/shared.md", "guide", "shared", checksumOf("from an external source"))
			f.manifest.ExistsVal = true

			// when
			report, err := f.sync(outgoing)

			// then
			require.NoError(t, err)
			assert.Equal(t, 0, report.Pushed)
			_, statErr := os.Stat(filepath.Join(f.repoPath, "personal", "claude"))
			assert.True(t, os.IsNotExist(statErr), "nothing out of scope reaches the repo")
		},
	)

	t.Run("should never publish a file a symlink points to outside the tool directory", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		outside := filepath.Join(t.TempDir(), "id_ed25519")
		writeFile(t, outside, "private key material")
		require.NoError(t, os.MkdirAll(filepath.Join(f.toolDir, "rules"), 0o700))
		require.NoError(t, os.Symlink(outside, filepath.Join(f.toolDir, "rules", "leak.md")))
		f.writeLocal("rules/real.md", "real rule")
		require.NoError(t, os.Symlink("real.md", filepath.Join(f.toolDir, "rules", "alias.md")))

		// when
		report, err := f.sync(outgoing)

		// then
		require.NoError(t, err)
		_, leaked := f.readRepo("rules/leak.md")
		assert.False(t, leaked, "a link leading outside the tool directory is never followed")
		alias, linked := f.readRepo("rules/alias.md")
		assert.True(t, linked, "a relative link that stays inside the tool directory is still synced")
		assert.Equal(t, "real rule", alias)
		assert.Equal(t, 2, report.Pushed)
	})

	t.Run("should skip a tool that is not installed on this device", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.config.Tools["claude"] = entities.Tool{Path: filepath.Join(f.toolDir, "missing"), Enabled: true}
		f.writeRepo("agents/reviewer.md", "from the other machine")

		// when
		report, err := f.sync(both)

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, report.Pulled)
		assert.Equal(t, 0, f.apply.StageCalls)
	})

	t.Run("should only preview in a dry run while still reporting what would be published", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.writeLocal("skills/review/SKILL.md", "review skill")
		f.writeRepo("agents/reviewer.md", "from the other machine")

		// when
		report, err := f.sync(commands.PersonalSyncOptions{Direction: entities.SyncBoth, DryRun: true})

		// then
		require.NoError(t, err)
		_, pushed := f.readRepo("skills/review/SKILL.md")
		_, pulled := f.readLocal("agents/reviewer.md")
		assert.False(t, pushed)
		assert.False(t, pulled)
		assert.Equal(t, 1, report.Pushed)
		assert.Equal(t, 1, report.Pulled)
		assert.Equal(t, []byte("review skill"), report.Plaintext["personal/claude/skills/review/SKILL.md"])
		assert.Equal(t, 0, f.manifest.SaveCalls)
	})
}

func TestPersonalSyncer_Changes(t *testing.T) {
	t.Parallel()

	t.Run("should list every difference with both contents and the repo commit", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.writeLocal("rules/go.md", "local")
		f.writeRepo("rules/go.md", "remote")
		f.writeLocal("rules/same.md", "same")
		f.writeRepo("rules/same.md", "same")
		committed := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
		f.history.Changes["personal/claude/rules/go.md"] = doubles.MockRepoChange{
			When:    committed,
			Subject: "feat: tuned go rules",
		}

		// when
		changes := f.syncer().Changes(f.config, f.repoPath)

		// then
		require.Len(t, changes, 1)
		change := changes[0]
		assert.Equal(t, "rules/go.md", change.Entry.Path)
		assert.Equal(t, entities.SyncActionConflict, change.Entry.Action)
		assert.Equal(t, []byte("local"), change.LocalContent)
		assert.Equal(t, []byte("remote"), change.RemoteContent)
		assert.True(t, committed.Equal(change.RemoteModified))
		assert.Equal(t, "feat: tuned go rules", change.RemoteSummary)
	})
}

func TestPersonalSyncer_ExecutableBit(t *testing.T) {
	t.Parallel()

	t.Run("should publish a change of the executable bit alone", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.base("hooks/guard.sh", "#!/bin/sh\n")
		f.writeRepo("hooks/guard.sh", "#!/bin/sh\n")
		f.writeLocal("hooks/guard.sh", "#!/bin/sh\n")
		require.NoError(t, os.Chmod(filepath.Join(f.toolDir, "hooks", "guard.sh"), 0o700))

		// when
		report, err := f.sync(commands.PersonalSyncOptions{Direction: entities.SyncBoth})

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, report.Pushed)
		info, statErr := os.Stat(filepath.Join(f.repoPath, "personal", "claude", "hooks", "guard.sh"))
		require.NoError(t, statErr)
		assert.NotZero(t, info.Mode().Perm()&0o100)
	})

	t.Run("should report a conflict on first sync when only the executable bit differs", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.writeRepo("hooks/guard.sh", "#!/bin/sh\n")
		f.writeLocal("hooks/guard.sh", "#!/bin/sh\n")
		require.NoError(t, os.Chmod(filepath.Join(f.toolDir, "hooks", "guard.sh"), 0o700))

		// when
		report, err := f.sync(commands.PersonalSyncOptions{Direction: entities.SyncBoth, NoPrompt: true})

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, report.ConflictsSkipped)
	})

	t.Run("should keep plain content checksums for files that are not executable", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.base("rules/go.md", "rule")
		f.writeRepo("rules/go.md", "rule")
		f.writeLocal("rules/go.md", "rule")

		// when
		report, err := f.sync(commands.PersonalSyncOptions{Direction: entities.SyncBoth})

		// then
		require.NoError(t, err)
		assert.Zero(t, report.Pushed+report.Pulled+report.ConflictsSkipped, "an existing base still matches")
	})
}

func TestPersonalSyncer_EncryptStoredPlaintext(t *testing.T) {
	t.Parallel()

	encryptAll := "personal/**\n"

	t.Run("should encrypt a plaintext repo copy that matches .aisyncencrypt", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		writeFile(t, filepath.Join(f.repoPath, ".aisyncencrypt"), encryptAll)
		f.writeRepo("rules/go.md", "rule")

		// when
		encrypted := f.syncer().EncryptStoredPlaintext(f.config, f.repoPath, false)

		// then
		assert.Equal(t, 1, encrypted)
		stored, ok := f.readRepo("rules/go.md.age")
		require.True(t, ok)
		assert.Equal(t, "ciphertext", stored)
		_, plaintextLeft := f.readRepo("rules/go.md")
		assert.False(t, plaintextLeft)
		assert.Equal(t, "rule", string(f.encryption.EncryptPlaintext))
	})

	t.Run("should encrypt the plaintext files of a tool not installed on this device", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		writeFile(t, filepath.Join(f.repoPath, ".aisyncencrypt"), encryptAll)
		cursorRule := filepath.Join(f.repoPath, "personal", "cursor", "rules", "go.mdc")
		writeFile(t, cursorRule, "rule")

		// when
		encrypted := f.syncer().EncryptStoredPlaintext(f.config, f.repoPath, false)

		// then
		assert.Equal(t, 1, encrypted)
		assert.FileExists(t, cursorRule+".age")
		assert.NoFileExists(t, cursorRule)
	})

	t.Run("should leave files that match no pattern in plaintext", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		writeFile(t, filepath.Join(f.repoPath, ".aisyncencrypt"), "personal/**/settings.local.json\n")
		f.writeRepo("rules/go.md", "rule")

		// when
		encrypted := f.syncer().EncryptStoredPlaintext(f.config, f.repoPath, false)

		// then
		assert.Equal(t, 0, encrypted)
		_, plaintextLeft := f.readRepo("rules/go.md")
		assert.True(t, plaintextLeft)
	})

	t.Run("should keep the executable bit and leave .gitkeep placeholders alone", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		writeFile(t, filepath.Join(f.repoPath, ".aisyncencrypt"), encryptAll)
		f.writeRepo("scripts/run.sh", "#!/bin/sh\n")
		require.NoError(t, os.Chmod(filepath.Join(f.repoPath, "personal", "claude", "scripts", "run.sh"), 0o700))
		f.writeRepo("rules/.gitkeep", "")

		// when
		encrypted := f.syncer().EncryptStoredPlaintext(f.config, f.repoPath, false)

		// then
		assert.Equal(t, 1, encrypted)
		info, statErr := os.Stat(filepath.Join(f.repoPath, "personal", "claude", "scripts", "run.sh.age"))
		require.NoError(t, statErr)
		assert.NotZero(t, info.Mode().Perm()&0o100)
		_, gitkeepLeft := f.readRepo("rules/.gitkeep")
		assert.True(t, gitkeepLeft)
	})

	t.Run("should only count the files in a dry run", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		writeFile(t, filepath.Join(f.repoPath, ".aisyncencrypt"), encryptAll)
		f.writeRepo("rules/go.md", "rule")

		// when
		encrypted := f.syncer().EncryptStoredPlaintext(f.config, f.repoPath, true)

		// then
		assert.Equal(t, 1, encrypted)
		assert.Equal(t, 0, f.encryption.EncryptCalls)
		_, plaintextLeft := f.readRepo("rules/go.md")
		assert.True(t, plaintextLeft)
	})

	t.Run("should not encrypt anything when no recipients are configured", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.config.Encryption.Recipients = nil
		writeFile(t, filepath.Join(f.repoPath, ".aisyncencrypt"), encryptAll)
		f.writeRepo("rules/go.md", "rule")

		// when
		encrypted := f.syncer().EncryptStoredPlaintext(f.config, f.repoPath, false)

		// then
		assert.Equal(t, 0, encrypted)
		assert.Equal(t, 0, f.encryption.EncryptCalls)
	})

	t.Run("should not follow a symlink in the repo", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		writeFile(t, filepath.Join(f.repoPath, ".aisyncencrypt"), encryptAll)
		outside := filepath.Join(t.TempDir(), "id_ed25519")
		writeFile(t, outside, "private")
		link := filepath.Join(f.repoPath, "personal", "claude", "rules", "key.md")
		require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o700))
		require.NoError(t, os.Symlink(outside, link))

		// when
		encrypted := f.syncer().EncryptStoredPlaintext(f.config, f.repoPath, false)

		// then
		assert.Equal(t, 0, encrypted)
		assert.Equal(t, 0, f.encryption.EncryptCalls)
		assert.NoFileExists(t, link+".age")
	})
}

func TestPersonalSyncer_LineEndings(t *testing.T) {
	t.Parallel()

	t.Run("should stage pulled files byte for byte so their CRLF line endings survive", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.writeRepo("skills/fonts/LICENSE.txt", "Copyright\r\nreserved\n")

		// when
		report, err := f.sync(commands.PersonalSyncOptions{Direction: entities.SyncIncoming})

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, report.Pulled)
		assert.Equal(t, 1, f.apply.StageVerbatimCalls)
		local, _ := f.readLocal("skills/fonts/LICENSE.txt")
		assert.Equal(t, "Copyright\r\nreserved\n", local)
	})
}

package commands_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/domain/commands"
	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
	"github.com/rios0rios0/aisync/test/doubles"
)

// excluding returns an exclude repository holding lines, as the device's
// exclude file would.
func excluding(lines ...string) *doubles.MockExcludeRepository {
	return &doubles.MockExcludeRepository{Lines: lines, PathVal: "/home/user/.config/aisync/exclude"}
}

// syncExcluding runs a personal sync with the device's exclusions, saving the
// bases as a successful push would.
func (f *syncFixture) syncExcluding(
	excludes repositories.ExcludeRepository,
	opts commands.PersonalSyncOptions,
) (*commands.PersonalSyncReport, error) {
	report, err := f.syncer().WithExcludes(excludes).Sync(f.config, f.repoPath, opts)
	if err == nil && report != nil {
		report.CommitManifests()
	}
	return report, err
}

func TestPersonalSyncer_Sync_WithExcludes(t *testing.T) {
	t.Parallel()

	both := commands.PersonalSyncOptions{Direction: entities.SyncBoth}

	t.Run("should not push an excluded local file, and still push the rest", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.writeLocal("settings.json", `{"statusLine":"termux"}`)
		f.writeLocal("rules/go.md", "go rules")

		// when
		report, err := f.syncExcluding(excluding("claude/settings.json"), both)

		// then
		require.NoError(t, err)
		_, pushed := f.readRepo("settings.json")
		assert.False(t, pushed, "the phone's own settings never reach the repo")
		content, _ := f.readRepo("rules/go.md")
		assert.Equal(t, "go rules", content)
		assert.Equal(t, 1, report.Pushed)
		_, recorded := f.savedBase("settings.json")
		assert.False(t, recorded)
	})

	t.Run("should not pull an excluded repo file", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.writeRepo("CLAUDE.md", "WSL environment")

		// when
		report, err := f.syncExcluding(excluding("claude/CLAUDE.md"), both)

		// then
		require.NoError(t, err)
		_, pulled := f.readLocal("CLAUDE.md")
		assert.False(t, pulled)
		assert.Equal(t, 0, report.Pulled)
		assert.Equal(t, 0, report.PendingIncoming)
	})

	t.Run("should not raise a conflict when both copies of an excluded file differ", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.writeLocal("settings.json", "phone")
		f.writeRepo("settings.json", "linux")

		// when
		report, err := f.syncExcluding(excluding("claude/settings.json"), both)

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, f.prompt.ConflictCalls)
		assert.Equal(t, 0, report.ConflictsSkipped)
		local, _ := f.readLocal("settings.json")
		repo, _ := f.readRepo("settings.json")
		assert.Equal(t, "phone", local)
		assert.Equal(t, "linux", repo)
	})

	t.Run("should not delete the repo copy of a synced file that is excluded and gone here", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.base("settings.json", "v1")
		f.writeRepo("settings.json", "v1")

		// when
		report, err := f.syncExcluding(excluding("claude/settings.json"), both)

		// then
		require.NoError(t, err)
		content, kept := f.readRepo("settings.json")
		assert.True(t, kept)
		assert.Equal(t, "v1", content)
		assert.Equal(t, 0, report.DeletedRepo)
	})

	t.Run(
		"should not delete the local copy of a synced file that is excluded and gone from the repo",
		func(t *testing.T) {
			t.Parallel()
			// given
			f := newSyncFixture(t)
			f.base("settings.json", "v1")
			f.writeLocal("settings.json", "v1")

			// when
			report, err := f.syncExcluding(excluding("claude/settings.json"), both)

			// then
			require.NoError(t, err)
			content, kept := f.readLocal("settings.json")
			assert.True(t, kept)
			assert.Equal(t, "v1", content)
			assert.Equal(t, 0, report.DeletedLocal)
		},
	)

	t.Run("should keep the base of an excluded file, so lifting the exclusion resumes the merge", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.base("settings.json", "v1")
		f.writeLocal("settings.json", "v1")
		f.writeRepo("settings.json", "v2 from the desktop")
		f.writeLocal("rules/go.md", "go rules")

		// when
		_, err := f.syncExcluding(excluding("claude/settings.json"), both)

		// then
		require.NoError(t, err)
		base, recorded := f.savedBase("settings.json")
		assert.True(t, recorded, "the manifest was rewritten for rules/go.md and kept the excluded base")
		assert.Equal(t, checksumOf("v1"), base)
		local, _ := f.readLocal("settings.json")
		assert.Equal(t, "v1", local)

		// when the exclusion is lifted
		_, err = f.syncExcluding(excluding(), both)

		// then only the repo side changed since the base, so it arrives without a conflict
		require.NoError(t, err)
		local, _ = f.readLocal("settings.json")
		assert.Equal(t, "v2 from the desktop", local)
		assert.Equal(t, 0, f.prompt.ConflictCalls)
	})

	t.Run("should exclude a path of every tool with the * tool", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.writeLocal("settings.json", "phone")

		// when
		report, err := f.syncExcluding(excluding("*/settings.json"), both)

		// then
		require.NoError(t, err)
		_, pushed := f.readRepo("settings.json")
		assert.False(t, pushed)
		assert.Equal(t, 0, report.Pushed)
	})

	t.Run("should fail without touching either side when the exclusions cannot be read", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.writeLocal("settings.json", "phone")
		unreadable := &doubles.MockExcludeRepository{LoadErr: errors.New("reading the exclusions: permission denied")}

		// when
		_, err := f.syncExcluding(unreadable, both)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "permission denied")
		_, pushed := f.readRepo("settings.json")
		assert.False(t, pushed)
	})
}

func TestPersonalSyncer_Changes_WithExcludes(t *testing.T) {
	t.Parallel()

	t.Run("should not list an excluded file as a change", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.writeLocal("settings.json", "phone")
		f.writeLocal("rules/go.md", "go rules")

		// when
		changes := f.syncer().WithExcludes(excluding("claude/settings.json")).Changes(f.config, f.repoPath)

		// then
		require.Len(t, changes, 1)
		assert.Equal(t, "rules/go.md", changes[0].Entry.Path)
	})
}

func TestPullCommand_Execute_WithExcludes(t *testing.T) {
	t.Parallel()

	// pullFixture is a pull of one source into one Claude directory.
	newPull := func(t *testing.T, toolDir string, files map[string][]byte) (
		*doubles.MockGitRepository, *doubles.MockManifestRepository,
		*doubles.MockApplyService, func(repositories.ExcludeRepository) *commands.PullCommand,
	) {
		t.Helper()
		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, promptSvc, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()
		configRepo.Config = &entities.Config{
			Sources:    []entities.Source{{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"}},
			Tools:      map[string]entities.Tool{"claude": {Path: toolDir, Enabled: true}},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{Files: files, ETag: "etag-1"}
		gitRepo.HasRemoteVal = false
		build := func(excludes repositories.ExcludeRepository) *commands.PullCommand {
			return newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo, encSvc, promptSvc,
				hooksMerger, settingsMerger, sectionMerger, applySvc).WithExcludes(excludes)
		}
		return gitRepo, manifestRepo, applySvc, build
	}

	t.Run("should not apply an excluded source file, and still apply the rest", func(t *testing.T) {
		t.Parallel()
		// given
		tmpDir := t.TempDir()
		toolDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(toolDir, 0o700))
		_, _, applySvc, build := newPull(t, toolDir, map[string][]byte{
			"shared/claude/settings.json": []byte(`{"statusLine":"linux"}`),
			"shared/claude/rules/test.md": []byte("# Test Rule"),
		})

		// when
		err := build(excluding("claude/settings.json")).
			Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		assert.Contains(t, applySvc.StagedFiles, filepath.Join(toolDir, "rules", "test.md"))
		assert.NotContains(t, applySvc.StagedFiles, filepath.Join(toolDir, "settings.json"))
	})

	t.Run("should not delete a source file applied before it was excluded", func(t *testing.T) {
		t.Parallel()
		// given
		tmpDir := t.TempDir()
		toolDir := filepath.Join(tmpDir, "claude-home")
		applied := filepath.Join(toolDir, "rules", "old.md")
		writeFile(t, applied, "old")
		_, manifestRepo, _, build := newPull(t, toolDir, map[string][]byte{
			"shared/claude/rules/new.md": []byte("# New"),
		})
		oldManifest := entities.NewManifest("0.1.0", "host")
		oldManifest.SetFile("rules/old.md", "guide", "shared", "old-checksum")
		manifestRepo.ExistsVal = true
		manifestRepo.Manifest = oldManifest

		// when
		err := build(excluding("claude/rules/old.md")).
			Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		content, readErr := os.ReadFile(applied)
		require.NoError(t, readErr, "an excluded file is no longer managed here, so it is left alone")
		assert.Equal(t, "old", string(content))
	})

	t.Run("should fail before pulling anything when the exclusions cannot be read", func(t *testing.T) {
		t.Parallel()
		// given
		tmpDir := t.TempDir()
		gitRepo, _, applySvc, build := newPull(t, filepath.Join(tmpDir, "claude-home"), map[string][]byte{
			"shared/claude/rules/test.md": []byte("# Test Rule"),
		})
		unreadable := &doubles.MockExcludeRepository{LoadErr: errors.New("permission denied")}

		// when
		err := build(unreadable).Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.Error(t, err)
		assert.Equal(t, 0, gitRepo.PullCalls)
		assert.Equal(t, 0, applySvc.StageCalls)
	})
}

func TestTranscriptSyncer_Sync_WithExcludes(t *testing.T) {
	t.Parallel()

	both := commands.PersonalSyncOptions{Direction: entities.SyncBoth}
	recently := time.Now().Add(-2 * time.Hour).Truncate(time.Second)

	syncExcluding := func(f *transcriptFixture, excludes repositories.ExcludeRepository) (*commands.PersonalSyncReport, error) {
		syncer := commands.NewTranscriptSyncer(f.service, f.base, f.apply, f.prompt, &doubles.MockTextDiffer{}).
			WithExcludes(excludes)
		return syncer.Sync(f.config, f.repoPath, both)
	}

	t.Run("should neither publish nor pull excluded conversations, and leave them in the snapshot", func(t *testing.T) {
		t.Parallel()
		// given
		f := newTranscriptFixture(t)
		f.writeConversation("-phone/local.jsonl", "phone\n", recently)
		f.service.Seed("claude/-desktop/remote.jsonl", []byte("desktop\n"), recently, "desktop")
		f.writeConversation("-phone/memory/MEMORY.md", "- [Rule](rule.md)\n", recently)

		// when
		report, err := syncExcluding(f, excluding("claude/projects/**/*.jsonl"))

		// then
		require.NoError(t, err)
		assert.NotContains(t, f.service.Files, "claude/-phone/local.jsonl")
		assert.Contains(t, f.service.Files, "claude/-desktop/remote.jsonl", "another device's conversation stays")
		_, pulled := f.readConversation("-desktop/remote.jsonl")
		assert.False(t, pulled)
		assert.Contains(t, f.service.Files, "claude/-phone/memory/MEMORY.md", "memories are not excluded")
		assert.Equal(t, 1, report.Pushed)
		assert.Equal(t, 0, report.Pulled)
	})

	t.Run("should keep the base of an excluded conversation file", func(t *testing.T) {
		t.Parallel()
		// given
		f := newTranscriptFixture(t)
		key := "claude/-phone/session.jsonl"
		f.base.Base[key] = checksumOf("line1\n")
		f.service.Seed(key, []byte("line1\n"), recently, "phone")
		f.writeConversation("-phone/memory/MEMORY.md", "memory\n", recently)

		// when
		_, err := syncExcluding(f, excluding("claude/projects/-phone/session.jsonl"))

		// then
		require.NoError(t, err)
		assert.Equal(t, checksumOf("line1\n"), f.base.Base[key])
		_, restored := f.readConversation("-phone/session.jsonl")
		assert.False(t, restored, "a conversation deleted here and excluded is neither restored nor deleted")
		assert.Contains(t, f.service.Files, key)
	})

	t.Run("should fail without touching the snapshot when the exclusions cannot be read", func(t *testing.T) {
		t.Parallel()
		// given
		f := newTranscriptFixture(t)
		f.writeConversation("-phone/local.jsonl", "phone\n", recently)
		unreadable := &doubles.MockExcludeRepository{LoadErr: errors.New("permission denied")}

		// when
		_, err := syncExcluding(f, unreadable)

		// then
		require.Error(t, err)
		assert.Equal(t, 0, f.service.FetchCalls)
		assert.Equal(t, 0, f.service.PublishCalls)
	})
}

func TestDiffCommand_Execute_WithExcludes(t *testing.T) {
	t.Parallel()

	t.Run("should leave excluded source files out of the shared diff", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		sourceRepo, diffService := sharedSource(f)
		sourceRepo.Result.Files["shared/claude/settings.json"] = []byte(`{"statusLine":"linux"}`)
		cmd, _, _ := newDiffCmd(f, sourceRepo, diffService)

		// when
		err := cmd.WithExcludes(excluding("claude/settings.json")).
			Execute("/tmp/config.yaml", f.repoPath, commands.DiffOptions{Shared: true})

		// then
		require.NoError(t, err)
		assert.Contains(t, diffService.Incoming, "shared/claude/rules/test.md")
		assert.NotContains(t, diffService.Incoming, "shared/claude/settings.json")
	})
}

//nolint:paralleltest // subtests build per-case fixtures through the shared newStatus closure
func TestStatusCommand_Execute_WithExcludes(t *testing.T) {
	newStatus := func(t *testing.T, excludes repositories.ExcludeRepository) (*commands.StatusCommand, string, *bytes.Buffer) {
		t.Helper()
		root := t.TempDir()
		toolDir := filepath.Join(root, "claude-home")
		writeFile(t, filepath.Join(toolDir, "settings.json"), "phone")
		writeFile(t, filepath.Join(toolDir, "rules", "go.md"), "go rules")
		config := &entities.Config{
			Sync:       entities.SyncConfig{Branch: "main"},
			Encryption: entities.EncryptionConfig{Identity: filepath.Join(root, "key.txt")},
			Tools:      map[string]entities.Tool{"claude": {Path: toolDir, Enabled: true}},
		}
		manifestRepo := &doubles.MockManifestRepository{}
		cmd := commands.NewStatusCommand(
			&doubles.MockConfigRepository{Config: config},
			&doubles.MockStateRepository{},
			manifestRepo,
			newStatusSyncer(manifestRepo).WithExcludes(excludes),
		).WithExcludes(excludes)
		out := &bytes.Buffer{}
		commands.SetStatusOutput(cmd, out)
		return cmd, filepath.Join(root, "repo"), out
	}

	t.Run("should name the exclude file and leave its paths out of the pending changes", func(t *testing.T) {
		// given
		cmd, repoPath, out := newStatus(t, excluding("claude/settings.json"))

		// when
		require.NoError(t, cmd.Execute("/tmp/config.yaml", repoPath))
		output := out.String()

		// then
		assert.Contains(t, output,
			"Excludes:   /home/user/.config/aisync/exclude (1 pattern(s), never synced on this device)")
		assert.Contains(t, output, "1 file(s) not yet pushed", "rules/go.md is pending and settings.json is not")
	})

	t.Run("should say nothing about an exclude file without patterns", func(t *testing.T) {
		// given
		cmd, repoPath, out := newStatus(t, excluding())

		// when
		require.NoError(t, cmd.Execute("/tmp/config.yaml", repoPath))
		output := out.String()

		// then
		assert.NotContains(t, output, "Excludes:")
		assert.Contains(t, output, "2 file(s) not yet pushed")
	})

	t.Run("should report an exclude file it cannot read", func(t *testing.T) {
		// given
		cmd, repoPath, out := newStatus(t, &doubles.MockExcludeRepository{LoadErr: errors.New("permission denied")})

		// when
		require.NoError(t, cmd.Execute("/tmp/config.yaml", repoPath))
		output := out.String()

		// then
		assert.Contains(t, output, "Excludes:   unreadable (permission denied)")
	})
}

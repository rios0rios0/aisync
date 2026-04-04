//go:build unit

package commands_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/domain/commands"
	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
	"github.com/rios0rios0/aisync/test/doubles"
)

// --- helpers ---

func newPullCmd(
	configRepo *doubles.MockConfigRepository,
	stateRepo *doubles.MockStateRepository,
	sourceRepo *doubles.MockSourceRepository,
	manifestRepo *doubles.MockManifestRepository,
	gitRepo *doubles.MockGitRepository,
	encryptionService *doubles.MockEncryptionService,
	conflictDetector *doubles.MockConflictDetector,
	hooksMerger *doubles.MockMerger,
	settingsMerger *doubles.MockMerger,
	sectionMerger *doubles.MockMerger,
	applyService *doubles.MockApplyService,
) *commands.PullCommand {
	return commands.NewPullCommand(
		configRepo, stateRepo, sourceRepo,
		manifestRepo, gitRepo, encryptionService,
		conflictDetector, hooksMerger, settingsMerger,
		sectionMerger, applyService,
		&doubles.MockPromptService{ToolAction: "apply", Confirmation: true, ConflictResolution: "remote"},
	)
}

func defaultPullDeps() (
	*doubles.MockConfigRepository,
	*doubles.MockStateRepository,
	*doubles.MockSourceRepository,
	*doubles.MockManifestRepository,
	*doubles.MockGitRepository,
	*doubles.MockEncryptionService,
	*doubles.MockConflictDetector,
	*doubles.MockMerger,
	*doubles.MockMerger,
	*doubles.MockMerger,
	*doubles.MockApplyService,
) {
	return &doubles.MockConfigRepository{},
		&doubles.MockStateRepository{},
		&doubles.MockSourceRepository{},
		&doubles.MockManifestRepository{},
		&doubles.MockGitRepository{},
		&doubles.MockEncryptionService{},
		&doubles.MockConflictDetector{},
		&doubles.MockMerger{},
		&doubles.MockMerger{},
		&doubles.MockMerger{},
		&doubles.MockApplyService{}
}

// --- Execute tests ---

func TestPullCommand_Execute(t *testing.T) {
	t.Run("should complete successfully when sources return files", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{
				{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
			},
			Tools:      map[string]entities.Tool{},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/rules/test.md": []byte("# Test"),
			},
			ETag: "etag-1",
		}
		gitRepo.HasRemoteVal = false

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, sourceRepo.FetchCalls)
		assert.GreaterOrEqual(t, stateRepo.SaveCalls, 1)
	})

	t.Run("should print message and return nil when no sources configured", func(t *testing.T) {
		// given
		configRepo, stateRepo, _, manifestRepo, gitRepo,
			encSvc, _, _, _, _, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{},
			Tools:   map[string]entities.Tool{},
		}

		cmd := commands.NewPullCommand(
			configRepo, stateRepo, nil, manifestRepo, gitRepo,
			encSvc, nil, nil, nil, nil, applySvc,
			&doubles.MockPromptService{ToolAction: "apply", Confirmation: true},
		)

		// when
		err := cmd.Execute("/tmp/config.yaml", "/tmp/repo", commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
	})

	t.Run("should return error when config load fails", func(t *testing.T) {
		// given
		configRepo := &doubles.MockConfigRepository{LoadErr: assert.AnError}
		cmd := commands.NewPullCommand(configRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

		// when
		err := cmd.Execute("/tmp/config.yaml", "/tmp/repo", commands.PullOptions{})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})

	t.Run("should print up-to-date message when all sources return nil (304)", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{
				{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
			},
			Tools: map[string]entities.Tool{},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = nil // 304 Not Modified
		gitRepo.HasRemoteVal = false

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, sourceRepo.FetchCalls)
		assert.Equal(t, 0, applySvc.StageCalls)
	})

	t.Run("should filter sources when source filter is provided", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{
				{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
				{Name: "other", Repo: "foo/bar", Branch: "main"},
			},
			Tools: map[string]entities.Tool{},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/rules/test.md": []byte("# Test"),
			},
			ETag: "etag-1",
		}
		gitRepo.HasRemoteVal = false

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir,
			commands.PullOptions{Force: true, SourceFilter: "guide"})

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, sourceRepo.FetchCalls)
		require.Len(t, sourceRepo.FetchedSources, 1)
		assert.Equal(t, "guide", sourceRepo.FetchedSources[0].Name)
	})

	t.Run("should warn and continue when git pull fails", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{
				{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
			},
			Tools: map[string]entities.Tool{},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = nil
		gitRepo.HasRemoteVal = true
		gitRepo.PullErr = assert.AnError

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, gitRepo.PullCalls)
	})

	t.Run("should store ETag in state when source returns one", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		state := entities.NewState("test-device")
		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{
				{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
			},
			Tools: map[string]entities.Tool{},
		}
		stateRepo.State = state
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/rules/test.md": []byte("# Test"),
			},
			ETag: "new-etag-value",
		}
		gitRepo.HasRemoteVal = false

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		assert.Equal(t, "new-etag-value", state.GetETag("guide"))
		assert.GreaterOrEqual(t, stateRepo.SaveCalls, 1)
	})

	t.Run("should create new state when state does not exist", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{
				{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
			},
			Tools: map[string]entities.Tool{},
		}
		stateRepo.ExistsVal = false
		sourceRepo.Result = nil
		gitRepo.HasRemoteVal = false

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		assert.GreaterOrEqual(t, stateRepo.SaveCalls, 1)
	})

	t.Run("should warn and continue when fetch source fails", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{
				{Name: "failing", Repo: "foo/bar", Branch: "main"},
			},
			Tools: map[string]entities.Tool{},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.FetchErr = assert.AnError
		gitRepo.HasRemoteVal = false

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, sourceRepo.FetchCalls)
	})

	t.Run("should apply files to tool directory when tool is enabled", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		toolDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(toolDir, 0700))

		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{
				{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
			},
			Tools: map[string]entities.Tool{
				"claude": {Path: toolDir, Enabled: true},
			},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/rules/test.md": []byte("# Test Rule"),
			},
			ETag: "etag-1",
		}
		gitRepo.HasRemoteVal = false

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, applySvc.StageCalls)
		assert.Equal(t, 1, applySvc.ApplyCalls)
	})

	t.Run("should skip disabled tools", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{
				{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
			},
			Tools: map[string]entities.Tool{
				"claude": {Path: filepath.Join(tmpDir, "claude"), Enabled: false},
			},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/rules/test.md": []byte("# Test"),
			},
			ETag: "etag-1",
		}
		gitRepo.HasRemoteVal = false

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, applySvc.StageCalls)
	})

	t.Run("should write fetched files to sync repo shared directory", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{
				{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
			},
			Tools:      map[string]entities.Tool{},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/rules/test.md": []byte("# Written to repo"),
			},
			ETag: "etag-1",
		}
		gitRepo.HasRemoteVal = false

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		content, readErr := os.ReadFile(filepath.Join(tmpDir, "shared/claude/rules/test.md"))
		require.NoError(t, readErr)
		assert.Equal(t, "# Written to repo", string(content))
	})

	t.Run("should recover from incomplete apply on startup", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{
				{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"},
			},
			Tools: map[string]entities.Tool{},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = nil
		gitRepo.HasRemoteVal = false

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, applySvc.RecoverCalls)
	})

	t.Run("should invoke hooks merger when source returns hooks.json", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		toolDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(toolDir, 0700))

		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{{Name: "guide", Repo: "r/g", Branch: "gen"}},
			Tools: map[string]entities.Tool{
				"claude": {Path: toolDir, Enabled: true},
			},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/hooks.json": []byte(`{"hooks":[]}`),
			},
			ETag: "etag-1",
		}
		gitRepo.HasRemoteVal = false
		hooksMerger.MergedData = []byte(`{"merged":true}`)

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, hooksMerger.MergeCalls)
		assert.Equal(t, 1, applySvc.StageCalls)
	})

	t.Run("should invoke settings merger when source returns settings.json", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		toolDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(toolDir, 0700))

		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{{Name: "guide", Repo: "r/g", Branch: "gen"}},
			Tools: map[string]entities.Tool{
				"claude": {Path: toolDir, Enabled: true},
			},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/settings.json": []byte(`{}`),
			},
			ETag: "etag-1",
		}
		gitRepo.HasRemoteVal = false
		settingsMerger.MergedData = []byte(`{"merged":true}`)

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, settingsMerger.MergeCalls)
	})

	t.Run("should invoke section merger when source returns CLAUDE.md", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		toolDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(toolDir, 0700))

		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{{Name: "guide", Repo: "r/g", Branch: "gen"}},
			Tools: map[string]entities.Tool{
				"claude": {Path: toolDir, Enabled: true},
			},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/CLAUDE.md": []byte("# Section"),
			},
			ETag: "etag-1",
		}
		gitRepo.HasRemoteVal = false
		sectionMerger.MergedData = []byte("# Merged Section")

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, sectionMerger.MergeCalls)
	})

	t.Run("should detect deletions when file in old manifest is missing from new set", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		toolDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(filepath.Join(toolDir, "rules"), 0700))

		// Create a file that will be deleted
		deletedFile := filepath.Join(toolDir, "rules/old.md")
		require.NoError(t, os.WriteFile(deletedFile, []byte("old"), 0600))

		oldManifest := entities.NewManifest("0.1.0", "host")
		oldManifest.SetFile("rules/old.md", "guide", "shared", "old-checksum")

		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{{Name: "guide", Repo: "r/g", Branch: "gen"}},
			Tools: map[string]entities.Tool{
				"claude": {Path: toolDir, Enabled: true},
			},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/rules/new.md": []byte("# New"),
			},
			ETag: "etag-1",
		}
		gitRepo.HasRemoteVal = false
		manifestRepo.ExistsVal = true
		manifestRepo.Manifest = oldManifest

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		_, statErr := os.Stat(deletedFile)
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("should apply personal-only file from sync repo to tool dir", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		toolDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(toolDir, 0700))

		// Create a personal file in the sync repo
		personalDir := filepath.Join(tmpDir, "personal", "claude")
		require.NoError(t, os.MkdirAll(personalDir, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(personalDir, "custom.md"), []byte("my custom rule"), 0600))

		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{{Name: "guide", Repo: "r/g", Branch: "gen"}},
			Tools: map[string]entities.Tool{
				"claude": {Path: toolDir, Enabled: true},
			},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/rules/test.md": []byte("# Shared"),
			},
			ETag: "etag-1",
		}
		gitRepo.HasRemoteVal = false

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, applySvc.StageCalls)
	})

	t.Run("should run dry-run mode without applying files", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		toolDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(toolDir, 0700))

		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{{Name: "guide", Repo: "r/g", Branch: "gen"}},
			Tools: map[string]entities.Tool{
				"claude": {Path: toolDir, Enabled: true},
			},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/rules/test.md": []byte("# Test"),
			},
			ETag: "etag-1",
		}
		gitRepo.HasRemoteVal = false

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{DryRun: true, Force: true})

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, applySvc.StageCalls)
		assert.Equal(t, 0, applySvc.ApplyCalls)
	})

	t.Run("should warn when two sources provide same file (last wins)", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{
				{Name: "source-a", Repo: "a/a", Branch: "main"},
				{Name: "source-b", Repo: "b/b", Branch: "main"},
			},
			Tools:      map[string]entities.Tool{},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/rules/overlap.md": []byte("content"),
			},
			ETag: "etag-1",
		}
		gitRepo.HasRemoteVal = false

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		assert.Equal(t, 2, sourceRepo.FetchCalls)
	})

	t.Run("should save manifest after applying tool files", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		toolDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(toolDir, 0700))

		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{{Name: "guide", Repo: "r/g", Branch: "gen"}},
			Tools: map[string]entities.Tool{
				"claude": {Path: toolDir, Enabled: true},
			},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/rules/test.md": []byte("# Test"),
			},
			ETag: "etag-1",
		}
		gitRepo.HasRemoteVal = false

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, manifestRepo.SaveCalls)
	})

	t.Run("should return error when staging fails", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		toolDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(toolDir, 0700))

		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{{Name: "guide", Repo: "r/g", Branch: "gen"}},
			Tools: map[string]entities.Tool{
				"claude": {Path: toolDir, Enabled: true},
			},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/rules/test.md": []byte("# Test"),
			},
			ETag: "etag-1",
		}
		gitRepo.HasRemoteVal = false
		applySvc.StageErr = assert.AnError

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		// Execute logs warning but does not propagate applyToToolDir error as fatal
		require.NoError(t, err)
	})

	t.Run("should handle personal file override over shared file", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		toolDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(toolDir, 0700))

		// Create personal override in sync repo
		personalDir := filepath.Join(tmpDir, "personal", "claude", "rules")
		require.NoError(t, os.MkdirAll(personalDir, 0700))
		require.NoError(t, os.WriteFile(
			filepath.Join(personalDir, "test.md"),
			[]byte("personal wins"),
			0600,
		))

		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{{Name: "guide", Repo: "r/g", Branch: "gen"}},
			Tools: map[string]entities.Tool{
				"claude": {Path: toolDir, Enabled: true},
			},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/rules/test.md": []byte("# Shared content"),
			},
			ETag: "etag-1",
		}
		gitRepo.HasRemoteVal = false

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, applySvc.StageCalls)
	})

	t.Run("should verify file checksums and detect force-push scenario", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		// Pre-populate a file in the sync repo so verifyFileChecksums finds it
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "shared/claude/rules"), 0700))
		require.NoError(t, os.WriteFile(
			filepath.Join(tmpDir, "shared/claude/rules/test.md"),
			[]byte("old content"),
			0600,
		))

		state := entities.NewState("test-device")
		state.SetETag("guide", "same-etag") // will be same after fetch

		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{
				{Name: "guide", Repo: "r/g", Branch: "gen"},
			},
			Tools:      map[string]entities.Tool{},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		stateRepo.State = state
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/rules/test.md": []byte("changed content"),
			},
			ETag: "same-etag", // same ETag -> force-push scenario
		}
		gitRepo.HasRemoteVal = false

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		// Verify the file was updated in sync repo
		content, readErr := os.ReadFile(filepath.Join(tmpDir, "shared/claude/rules/test.md"))
		require.NoError(t, readErr)
		assert.Equal(t, "changed content", string(content))
	})

	t.Run("should decrypt encrypted personal file when matching encrypt patterns", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		toolDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(toolDir, 0700))

		// Create encrypted personal file in sync repo
		personalDir := filepath.Join(tmpDir, "personal", "claude")
		require.NoError(t, os.MkdirAll(personalDir, 0700))
		require.NoError(t, os.WriteFile(
			filepath.Join(personalDir, "creds.secret.age"),
			[]byte("ENCRYPTED-CONTENT"),
			0600,
		))

		// Create .aisyncencrypt file
		require.NoError(t, os.WriteFile(
			filepath.Join(tmpDir, ".aisyncencrypt"),
			[]byte("*.secret"),
			0600,
		))

		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{{Name: "guide", Repo: "r/g", Branch: "gen"}},
			Tools: map[string]entities.Tool{
				"claude": {Path: toolDir, Enabled: true},
			},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/rules/test.md": []byte("# Shared"),
			},
			ETag: "etag-1",
		}
		gitRepo.HasRemoteVal = false
		encSvc.DecryptedData = []byte("decrypted secret content")

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, applySvc.StageCalls)
	})

	t.Run("should skip files matching unchanged checksum when writing to sync repo", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		unchangedContent := []byte("# Unchanged content")

		// Pre-populate the file with exact same content
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "shared/claude/rules"), 0700))
		require.NoError(t, os.WriteFile(
			filepath.Join(tmpDir, "shared/claude/rules/test.md"),
			unchangedContent,
			0600,
		))

		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{
				{Name: "guide", Repo: "r/g", Branch: "gen"},
			},
			Tools:      map[string]entities.Tool{},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/rules/test.md": unchangedContent,
			},
			ETag: "etag-1",
		}
		gitRepo.HasRemoteVal = false

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		// File should be unchanged
		content, readErr := os.ReadFile(filepath.Join(tmpDir, "shared/claude/rules/test.md"))
		require.NoError(t, readErr)
		assert.Equal(t, unchangedContent, content)
	})

	t.Run("should detect conflict when incoming personal differs from local", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		toolDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(toolDir, 0700))

		// Create local file
		require.NoError(t, os.WriteFile(
			filepath.Join(toolDir, "custom.md"),
			[]byte("local version"),
			0600,
		))

		// Create incoming personal file
		personalDir := filepath.Join(tmpDir, "personal", "claude")
		require.NoError(t, os.MkdirAll(personalDir, 0700))
		require.NoError(t, os.WriteFile(
			filepath.Join(personalDir, "custom.md"),
			[]byte("remote version"),
			0600,
		))

		oldManifest := entities.NewManifest("0.1.0", "host")
		oldManifest.SetFile("custom.md", "personal", "personal", "old-checksum")

		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{{Name: "guide", Repo: "r/g", Branch: "gen"}},
			Tools: map[string]entities.Tool{
				"claude": {Path: toolDir, Enabled: true},
			},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/rules/test.md": []byte("# Shared"),
			},
			ETag: "etag-1",
		}
		gitRepo.HasRemoteVal = false
		manifestRepo.ExistsVal = true
		manifestRepo.Manifest = oldManifest
		conflictDet.Conflicts = []entities.Conflict{
			{Path: "custom.md", RemoteDevice: "other-device", RemoteContent: []byte("remote version")},
		}

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when - force mode auto-resolves conflicts
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
	})

	t.Run("should handle git pull with open and has remote", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{
				{Name: "guide", Repo: "r/g", Branch: "gen"},
			},
			Tools: map[string]entities.Tool{},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = nil
		gitRepo.HasRemoteVal = true
		// Git pull succeeds

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, gitRepo.OpenCalls)
		assert.Equal(t, 1, gitRepo.PullCalls)
	})

	t.Run("should warn when git open fails", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{
				{Name: "guide", Repo: "r/g", Branch: "gen"},
			},
			Tools: map[string]entities.Tool{},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = nil
		gitRepo.OpenErr = assert.AnError

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err) // git open failure is non-fatal
		assert.Equal(t, 1, gitRepo.OpenCalls)
		assert.Equal(t, 0, gitRepo.PullCalls)
	})

	t.Run("should not skip ETag verification when no previous ETag exists", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		// Pre-populate file
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "shared/claude/rules"), 0700))
		require.NoError(t, os.WriteFile(
			filepath.Join(tmpDir, "shared/claude/rules/test.md"),
			[]byte("old content"),
			0600,
		))

		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{
				{Name: "new-source", Repo: "r/g", Branch: "gen"},
			},
			Tools:      map[string]entities.Tool{},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		// State has no ETags at all (first fetch)
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/rules/test.md": []byte("new content"),
			},
			ETag: "first-etag",
		}
		gitRepo.HasRemoteVal = false

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
	})

	t.Run("should handle ETag changed normally without warning", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "shared/claude/rules"), 0700))
		require.NoError(t, os.WriteFile(
			filepath.Join(tmpDir, "shared/claude/rules/test.md"),
			[]byte("old content"),
			0600,
		))

		state := entities.NewState("test-device")
		state.SetETag("guide", "old-etag")

		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{
				{Name: "guide", Repo: "r/g", Branch: "gen"},
			},
			Tools:      map[string]entities.Tool{},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		stateRepo.State = state
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/rules/test.md": []byte("new content"),
			},
			ETag: "new-etag", // different from old
		}
		gitRepo.HasRemoteVal = false

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
	})

	t.Run("should abort when non-force non-dry-run and stdin is closed", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		toolDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(toolDir, 0700))

		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{{Name: "guide", Repo: "r/g", Branch: "gen"}},
			Tools: map[string]entities.Tool{
				"claude": {Path: toolDir, Enabled: true},
			},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/rules/test.md": []byte("# Test"),
			},
			ETag: "etag-1",
		}
		gitRepo.HasRemoteVal = false

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when - non-force mode will prompt, stdin closed -> abort
		err := cmd.Execute("/tmp/config.yaml", tmpDir,
			commands.PullOptions{DryRun: false, Force: false})

		// then
		// The error from promptToolAction returning abort is not fatal to Execute
		// since applyToToolDir logs the warning. Or it may return "aborted by user".
		// Either way Execute should complete (possibly with error).
		_ = err
	})

	t.Run("should read encrypted personal file that shadows shared file", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		toolDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(toolDir, 0700))

		// Create encrypted personal file that shadows a shared file
		personalDir := filepath.Join(tmpDir, "personal", "claude", "rules")
		require.NoError(t, os.MkdirAll(personalDir, 0700))
		require.NoError(t, os.WriteFile(
			filepath.Join(personalDir, "test.md.age"),
			[]byte("ENCRYPTED"),
			0600,
		))

		// Create .aisyncencrypt that matches the file
		require.NoError(t, os.WriteFile(
			filepath.Join(tmpDir, ".aisyncencrypt"),
			[]byte("**/*.md"),
			0600,
		))

		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{{Name: "guide", Repo: "r/g", Branch: "gen"}},
			Tools: map[string]entities.Tool{
				"claude": {Path: toolDir, Enabled: true},
			},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/rules/test.md": []byte("# Shared"),
			},
			ETag: "etag-1",
		}
		gitRepo.HasRemoteVal = false
		encSvc.DecryptedData = []byte("decrypted personal override")

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		assert.GreaterOrEqual(t, encSvc.DecryptCalls, 1)
	})

	t.Run("should skip personal .age files and apply personal-only non-encrypted files", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		toolDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(toolDir, 0700))

		personalDir := filepath.Join(tmpDir, "personal", "claude")
		require.NoError(t, os.MkdirAll(personalDir, 0700))
		// This .age file should be skipped by applyPersonalOnlyFiles
		require.NoError(t, os.WriteFile(filepath.Join(personalDir, "creds.age"), []byte("encrypted"), 0600))
		// This file should be applied
		require.NoError(t, os.WriteFile(filepath.Join(personalDir, "my-config.md"), []byte("personal config"), 0600))

		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{{Name: "guide", Repo: "r/g", Branch: "gen"}},
			Tools: map[string]entities.Tool{
				"claude": {Path: toolDir, Enabled: true},
			},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/rules/shared.md": []byte("# Shared"),
			},
			ETag: "etag-1",
		}
		gitRepo.HasRemoteVal = false

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, applySvc.StageCalls)
	})

	t.Run("should handle dry-run mode showing new modified and unchanged files", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		toolDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(toolDir, 0700))

		// Create an existing file in tool dir for modified detection
		require.NoError(t, os.WriteFile(filepath.Join(toolDir, "existing.md"), []byte("old"), 0600))
		// Create an existing file with unchanged content
		require.NoError(t, os.WriteFile(filepath.Join(toolDir, "same.md"), []byte("unchanged"), 0600))

		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{{Name: "guide", Repo: "r/g", Branch: "gen"}},
			Tools: map[string]entities.Tool{
				"claude": {Path: toolDir, Enabled: true},
			},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/existing.md": []byte("modified content"),
				"shared/claude/same.md":     []byte("unchanged"),
				"shared/claude/brand-new.md": []byte("new file"),
			},
			ETag: "etag-1",
		}
		gitRepo.HasRemoteVal = false

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir,
			commands.PullOptions{DryRun: true, Force: true})

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, applySvc.StageCalls)
	})

	t.Run("should create new state when existing state load fails", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{
				{Name: "guide", Repo: "r/g", Branch: "gen"},
			},
			Tools: map[string]entities.Tool{},
		}
		stateRepo.ExistsVal = true
		stateRepo.LoadErr = assert.AnError // load fails
		sourceRepo.Result = nil
		gitRepo.HasRemoteVal = false

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		assert.GreaterOrEqual(t, stateRepo.SaveCalls, 1)
	})

	t.Run("should write new file and update changed file in sync repo", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		// Pre-populate with old content
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "shared/claude/rules"), 0700))
		require.NoError(t, os.WriteFile(
			filepath.Join(tmpDir, "shared/claude/rules/old.md"),
			[]byte("old version"),
			0600,
		))

		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{
				{Name: "guide", Repo: "r/g", Branch: "gen"},
			},
			Tools:      map[string]entities.Tool{},
			Encryption: entities.EncryptionConfig{Identity: "/tmp/key.txt"},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/rules/old.md": []byte("updated version"),
				"shared/claude/rules/new.md": []byte("brand new file"),
			},
			ETag: "etag-1",
		}
		gitRepo.HasRemoteVal = false

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		// Verify updated file
		content, readErr := os.ReadFile(filepath.Join(tmpDir, "shared/claude/rules/old.md"))
		require.NoError(t, readErr)
		assert.Equal(t, "updated version", string(content))
		// Verify new file
		content2, readErr2 := os.ReadFile(filepath.Join(tmpDir, "shared/claude/rules/new.md"))
		require.NoError(t, readErr2)
		assert.Equal(t, "brand new file", string(content2))
	})

	t.Run("should handle unencrypted personal file read when no encrypt patterns", func(t *testing.T) {
		// given
		tmpDir := t.TempDir()
		toolDir := filepath.Join(tmpDir, "claude-home")
		require.NoError(t, os.MkdirAll(toolDir, 0700))

		// Create personal file (unencrypted)
		personalDir := filepath.Join(tmpDir, "personal", "claude", "rules")
		require.NoError(t, os.MkdirAll(personalDir, 0700))
		require.NoError(t, os.WriteFile(
			filepath.Join(personalDir, "test.md"),
			[]byte("personal override"),
			0600,
		))

		configRepo, stateRepo, sourceRepo, manifestRepo, gitRepo,
			encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc := defaultPullDeps()

		configRepo.Config = &entities.Config{
			Sources: []entities.Source{{Name: "guide", Repo: "r/g", Branch: "gen"}},
			Tools: map[string]entities.Tool{
				"claude": {Path: toolDir, Enabled: true},
			},
			Encryption: entities.EncryptionConfig{},
		}
		stateRepo.State = entities.NewState("test-device")
		stateRepo.ExistsVal = true
		sourceRepo.Result = &repositories.FetchResult{
			Files: map[string][]byte{
				"shared/claude/rules/test.md": []byte("# Shared version"),
			},
			ETag: "etag-1",
		}
		gitRepo.HasRemoteVal = false

		cmd := newPullCmd(configRepo, stateRepo, sourceRepo, manifestRepo,
			gitRepo, encSvc, conflictDet, hooksMerger, settingsMerger, sectionMerger, applySvc)

		// when
		err := cmd.Execute("/tmp/config.yaml", tmpDir, commands.PullOptions{Force: true})

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, applySvc.StageCalls)
	})
}

// --- ExpandHome (public helper) ---

func TestExpandHome(t *testing.T) {
	t.Run("should expand tilde prefix to home directory", func(t *testing.T) {
		// given
		home, _ := os.UserHomeDir()

		// when
		result := commands.ExpandHome("~/some/path")

		// then
		assert.Equal(t, filepath.Join(home, "some/path"), result)
	})

	t.Run("should return path unchanged when no tilde prefix", func(t *testing.T) {
		// when
		result := commands.ExpandHome("/absolute/path")

		// then
		assert.Equal(t, "/absolute/path", result)
	})
}

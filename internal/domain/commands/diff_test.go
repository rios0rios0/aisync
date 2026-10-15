package commands_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/domain/commands"
	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
	"github.com/rios0rios0/aisync/test/doubles"
)

// newDiffCmd wires a DiffCommand around a sync fixture, with a viewer that
// records what it would page.
func newDiffCmd(
	f *syncFixture,
	sourceRepo *doubles.MockSourceRepository,
	diffService *doubles.MockDiffService,
) (*commands.DiffCommand, *doubles.MockDiffViewer, *doubles.MockConfigRepository) {
	viewer := &doubles.MockDiffViewer{}
	configRepo := &doubles.MockConfigRepository{Config: f.config}
	cmd := commands.NewDiffCommand(
		configRepo, sourceRepo, diffService, f.syncer(), &doubles.MockTextDiffer{},
		&entities.PlainFormatter{}, viewer,
	)
	return cmd, viewer, configRepo
}

// sharedSource configures one external source that delivers one changed file.
func sharedSource(f *syncFixture) (*doubles.MockSourceRepository, *doubles.MockDiffService) {
	f.config.Sources = []entities.Source{{Name: "guide", Repo: "rios0rios0/guide", Branch: "generated"}}
	sourceRepo := &doubles.MockSourceRepository{
		Result: &repositories.FetchResult{Files: map[string][]byte{"shared/claude/rules/test.md": []byte("# Test")}},
	}
	diffService := &doubles.MockDiffService{SharedDiff: []entities.FileChange{
		{Path: "shared/claude/rules/test.md", Direction: entities.ChangeAdded, Source: "guide"},
	}}
	return sourceRepo, diffService
}

func TestDiffCommand_Execute(t *testing.T) {
	t.Parallel()

	t.Run("should show changes arriving from external sources", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		sourceRepo, diffService := sharedSource(f)
		cmd, viewer, _ := newDiffCmd(f, sourceRepo, diffService)

		// when
		err := cmd.Execute("/tmp/config.yaml", f.repoPath, commands.DiffOptions{})

		// then
		require.NoError(t, err)
		require.Len(t, viewer.Shown, 1)
		assert.Contains(t, viewer.Shown[0], "External sources:")
		assert.Contains(t, viewer.Shown[0], "shared/claude/rules/test.md")
	})

	t.Run("should report no changes when nothing differs", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.writeLocal("rules/go.md", "same")
		f.writeRepo("rules/go.md", "same")
		cmd, viewer, _ := newDiffCmd(f, &doubles.MockSourceRepository{}, &doubles.MockDiffService{})

		// when
		err := cmd.Execute("/tmp/config.yaml", f.repoPath, commands.DiffOptions{})

		// then
		require.NoError(t, err)
		assert.Empty(t, viewer.Shown)
	})

	t.Run("should return an error when the config cannot be loaded", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		cmd, _, configRepo := newDiffCmd(f, &doubles.MockSourceRepository{}, &doubles.MockDiffService{})
		configRepo.LoadErr = errors.New("config not found")

		// when
		err := cmd.Execute("/tmp/config.yaml", f.repoPath, commands.DiffOptions{})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})

	t.Run("should show a personal file changed here as the repo becoming this version", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.base("rules/go.md", "old")
		f.writeRepo("rules/go.md", "old")
		f.writeLocal("rules/go.md", "new")
		cmd, viewer, _ := newDiffCmd(f, &doubles.MockSourceRepository{}, &doubles.MockDiffService{})

		// when
		err := cmd.Execute("/tmp/config.yaml", f.repoPath, commands.DiffOptions{})

		// then
		require.NoError(t, err)
		require.Len(t, viewer.Shown, 1)
		shown := viewer.Shown[0]
		assert.Contains(t, shown, "→ repo")
		assert.Contains(t, shown, "claude/rules/go.md")
		assert.Contains(t, shown, "changed here")
		assert.Contains(t, shown, "--- repo: claude/rules/go.md\n+++ local: claude/rules/go.md\n-old\n+new\n")
	})

	t.Run("should show an incoming personal change as this device becoming the repo version", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.base("rules/go.md", "old")
		f.writeLocal("rules/go.md", "old")
		f.writeRepo("rules/go.md", "from the other machine")
		cmd, viewer, _ := newDiffCmd(f, &doubles.MockSourceRepository{}, &doubles.MockDiffService{})

		// when
		err := cmd.Execute("/tmp/config.yaml", f.repoPath, commands.DiffOptions{})

		// then
		require.NoError(t, err)
		require.Len(t, viewer.Shown, 1)
		assert.Contains(t, viewer.Shown[0], "← repo")
		assert.Contains(t, viewer.Shown[0], "--- local: claude/rules/go.md\n+++ repo: claude/rules/go.md\n")
	})

	t.Run("should print without paging or diffs when summarizing", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.writeLocal("rules/go.md", "new here")
		cmd, viewer, _ := newDiffCmd(f, &doubles.MockSourceRepository{}, &doubles.MockDiffService{})

		// when
		err := cmd.Execute("/tmp/config.yaml", f.repoPath, commands.DiffOptions{Summary: true})

		// then
		require.NoError(t, err)
		assert.Empty(t, viewer.Shown)
	})

	t.Run("should show only what a push would send, plus conflicts, in reverse mode", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		f.writeLocal("skills/mine/SKILL.md", "only here")
		f.writeRepo("agents/theirs.md", "only in the repo")
		f.writeLocal("rules/both.md", "mine")
		f.writeRepo("rules/both.md", "theirs")
		cmd, viewer, _ := newDiffCmd(f, &doubles.MockSourceRepository{}, &doubles.MockDiffService{})

		// when
		err := cmd.Execute("/tmp/config.yaml", f.repoPath, commands.DiffOptions{Reverse: true})

		// then
		require.NoError(t, err)
		require.Len(t, viewer.Shown, 1)
		assert.Contains(t, viewer.Shown[0], "skills/mine/SKILL.md")
		assert.Contains(t, viewer.Shown[0], "rules/both.md")
		assert.NotContains(t, viewer.Shown[0], "agents/theirs.md")
	})

	t.Run("should leave out external sources when the personal filter is set", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		sourceRepo, diffService := sharedSource(f)
		f.writeLocal("rules/go.md", "new here")
		cmd, viewer, _ := newDiffCmd(f, sourceRepo, diffService)

		// when
		err := cmd.Execute("/tmp/config.yaml", f.repoPath, commands.DiffOptions{Personal: true})

		// then
		require.NoError(t, err)
		require.Len(t, viewer.Shown, 1)
		assert.NotContains(t, viewer.Shown[0], "External sources:")
		assert.Contains(t, viewer.Shown[0], "claude/rules/go.md")
		assert.Equal(t, 0, sourceRepo.FetchCalls)
	})

	t.Run("should leave out personal files when the shared filter is set", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		sourceRepo, diffService := sharedSource(f)
		f.writeLocal("rules/go.md", "new here")
		cmd, viewer, _ := newDiffCmd(f, sourceRepo, diffService)

		// when
		err := cmd.Execute("/tmp/config.yaml", f.repoPath, commands.DiffOptions{Shared: true})

		// then
		require.NoError(t, err)
		require.Len(t, viewer.Shown, 1)
		assert.Contains(t, viewer.Shown[0], "External sources:")
		assert.NotContains(t, viewer.Shown[0], "claude/rules/go.md")
	})

	t.Run("should skip sources that do not match the source filter", func(t *testing.T) {
		t.Parallel()
		// given
		f := newSyncFixture(t)
		sourceRepo, diffService := sharedSource(f)
		cmd, _, _ := newDiffCmd(f, sourceRepo, diffService)

		// when
		err := cmd.Execute("/tmp/config.yaml", f.repoPath, commands.DiffOptions{SourceFilter: "another"})

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, sourceRepo.FetchCalls)
	})
}

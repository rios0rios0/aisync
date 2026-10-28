package repositories_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repositories "github.com/rios0rios0/aisync/internal/infrastructure/repositories"
)

func skipIfNoGit(t *testing.T) *repositories.ExecGitRepository {
	t.Helper()
	repo, err := repositories.NewExecGitRepository()
	if err != nil {
		t.Skip("git binary not found, skipping exec git tests")
	}
	return repo
}

func initTempRepo(t *testing.T) (*repositories.ExecGitRepository, string) {
	t.Helper()
	repo := skipIfNoGit(t)
	dir := t.TempDir()
	require.NoError(t, repo.Init(dir))
	// Commit as a fixed test identity, and keep the developer's global git
	// setup out of the way: commit signing can need an agent (1Password
	// refuses requests under parallel load) and global hooks can call out to
	// services. Repository-local config overrides the global one.
	for _, setting := range [][2]string{
		{"user.email", "test@test.com"},
		{"user.name", "Test"},
		{"commit.gpgsign", "false"},
		{"tag.gpgsign", "false"},
		{"core.hooksPath", os.DevNull},
	} {
		out, err := exec.Command("git", "-C", dir, "config", setting[0], setting[1]).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	return repo, dir
}

func TestExecGitRepository_NewExecGitRepository_ShouldSucceedWhenGitOnPath(t *testing.T) {
	t.Parallel()

	// given / when
	repo, err := repositories.NewExecGitRepository()

	// then
	if err != nil {
		t.Skip("git not on PATH")
	}
	assert.NotNil(t, repo)
}

func TestExecGitRepository_Init_ShouldCreateGitDirAndSetHEADToMain(t *testing.T) {
	t.Parallel()

	// given
	repo := skipIfNoGit(t)
	dir := t.TempDir()

	// when
	err := repo.Init(dir)

	// then — the .git directory exists AND HEAD points at refs/heads/main,
	// regardless of the system's `init.defaultBranch` value.
	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(dir, ".git", "HEAD"))
	require.NoError(t, statErr)

	headContent, readErr := os.ReadFile(filepath.Join(dir, ".git", "HEAD"))
	require.NoError(t, readErr)
	assert.Equal(t, "ref: refs/heads/main\n", string(headContent))
}

func TestExecGitRepository_Open_ShouldSucceedAfterInit(t *testing.T) {
	t.Parallel()

	// given
	repo, dir := initTempRepo(t)

	// when
	err := repo.Open(dir)

	// then
	assert.NoError(t, err)
}

func TestExecGitRepository_Open_ShouldFailForNonGitDir(t *testing.T) {
	t.Parallel()

	// given
	repo := skipIfNoGit(t)
	dir := t.TempDir()

	// when
	err := repo.Open(dir)

	// then
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a git repository")
}

func TestExecGitRepository_IsClean_ShouldReturnTrueForFreshRepo(t *testing.T) {
	t.Parallel()

	// given
	repo, _ := initTempRepo(t)

	// when
	clean, err := repo.IsClean()

	// then
	require.NoError(t, err)
	assert.True(t, clean)
}

func TestExecGitRepository_IsClean_ShouldReturnFalseWithUnstagedChanges(t *testing.T) {
	t.Parallel()

	// given
	repo, dir := initTempRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0600))

	// when
	clean, err := repo.IsClean()

	// then
	require.NoError(t, err)
	assert.False(t, clean)
}

func TestExecGitRepository_CommitAll_ShouldMakeRepoClean(t *testing.T) {
	t.Parallel()

	// given
	repo, dir := initTempRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0600))

	// when
	err := repo.CommitAll("test commit")

	// then
	require.NoError(t, err)
	clean, _ := repo.IsClean()
	assert.True(t, clean, "repo should be clean after commit")

	// Verify the file exists in the working tree
	_, statErr := os.Stat(filepath.Join(dir, "file.txt"))
	assert.NoError(t, statErr)
}

func TestExecGitRepository_HasRemote_ShouldReturnFalseForLocalOnly(t *testing.T) {
	t.Parallel()

	// given
	repo, _ := initTempRepo(t)

	// when
	has := repo.HasRemote()

	// then
	assert.False(t, has)
}

func TestExecGitRepository_AddRemote_ShouldMakeHasRemoteTrue(t *testing.T) {
	t.Parallel()

	// given
	repo, _ := initTempRepo(t)

	// when
	err := repo.AddRemote("origin", "https://github.com/test/repo.git")

	// then
	require.NoError(t, err)
	assert.True(t, repo.HasRemote())
}

func TestExecGitRepository_SetConfig_ShouldSetValue(t *testing.T) {
	t.Parallel()

	// given
	repo, dir := initTempRepo(t)

	// when
	err := repo.SetConfig("user.name", "TestUser")

	// then
	require.NoError(t, err)

	// Verify with git config
	out, _ := exec.Command("git", "-C", dir, "config", "user.name").Output()
	assert.Equal(t, "TestUser\n", string(out))
}

func TestExecGitRepository_Clone_ShouldCloneFromLocalRepo(t *testing.T) {
	t.Parallel()

	// given
	repo := skipIfNoGit(t)

	// Create a source repo with a commit
	srcDir := t.TempDir()
	require.NoError(t, repo.Init(srcDir))
	_ = exec.Command("git", "-C", srcDir, "config", "user.email", "test@test.com").Run()
	_ = exec.Command("git", "-C", srcDir, "config", "user.name", "Test").Run()
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "README.md"), []byte("# Test"), 0600))
	require.NoError(t, repo.CommitAll("initial commit"))

	// when
	destDir := filepath.Join(t.TempDir(), "cloned")
	cloneRepo := skipIfNoGit(t)
	err := cloneRepo.Clone(srcDir, destDir, "main")

	// then
	require.NoError(t, err)
	data, readErr := os.ReadFile(filepath.Join(destDir, "README.md"))
	require.NoError(t, readErr)
	assert.Equal(t, "# Test", string(data))
}

func TestExecGitRepository_Push_ShouldReportPushedWhenTheRemoteReceivesACommit(t *testing.T) {
	t.Parallel()

	// given
	repo, dir := initTempRepo(t)
	remoteDir := addBareRemote(t, repo, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "second.txt"), []byte("second"), 0600))
	require.NoError(t, repo.CommitAll("feat: added a second file"))

	// when
	pushed, err := repo.Push()

	// then
	require.NoError(t, err)
	assert.True(t, pushed, "the remote received the new commit")
	assert.Equal(t, gitOutput(t, dir, "rev-parse", "HEAD"), gitOutput(t, remoteDir, "rev-parse", "main"))
}

func TestExecGitRepository_Push_ShouldReportNothingPushedWhenTheRemoteIsUpToDate(t *testing.T) {
	t.Parallel()

	// given
	repo, dir := initTempRepo(t)
	addBareRemote(t, repo, dir)

	// when
	pushed, err := repo.Push()

	// then
	require.NoError(t, err)
	assert.False(t, pushed, "the remote already had every commit")
}

// addBareRemote commits a first file, creates a bare repository as origin,
// and pushes main to it with upstream tracking, as a cloned sync repo has.
// It returns the bare repository's directory.
func addBareRemote(t *testing.T, repo *repositories.ExecGitRepository, dir string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "first.txt"), []byte("first"), 0600))
	require.NoError(t, repo.CommitAll("chore: added a first file"))

	remoteDir := filepath.Join(t.TempDir(), "remote.git")
	gitOutput(t, "", "init", "--bare", "--initial-branch=main", remoteDir)
	gitOutput(t, remoteDir, "config", "core.hooksPath", os.DevNull)
	require.NoError(t, repo.AddRemote("origin", remoteDir))
	gitOutput(t, dir, "push", "--set-upstream", "origin", "main")
	return remoteDir
}

// gitOutput runs git in dir (the current directory when empty) and returns
// its trimmed standard output, failing the test when git fails.
func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	out, err := exec.Command("git", args...).CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.TrimSpace(string(out))
}

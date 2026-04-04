//go:build unit

package repositories_test

import (
	"os"
	"os/exec"
	"path/filepath"
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
	// Configure git user for commits
	_ = exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run()  //nolint:gosec
	_ = exec.Command("git", "-C", dir, "config", "user.name", "Test").Run()             //nolint:gosec
	return repo, dir
}

func TestExecGitRepository_NewExecGitRepository_ShouldSucceedWhenGitOnPath(t *testing.T) {
	// given / when
	repo, err := repositories.NewExecGitRepository()

	// then
	if err != nil {
		t.Skip("git not on PATH")
	}
	assert.NotNil(t, repo)
}

func TestExecGitRepository_Init_ShouldCreateGitDir(t *testing.T) {
	// given
	repo := skipIfNoGit(t)
	dir := t.TempDir()

	// when
	err := repo.Init(dir)

	// then
	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(dir, ".git", "HEAD"))
	assert.NoError(t, statErr)
}

func TestExecGitRepository_Open_ShouldSucceedAfterInit(t *testing.T) {
	// given
	repo, dir := initTempRepo(t)

	// when
	err := repo.Open(dir)

	// then
	assert.NoError(t, err)
}

func TestExecGitRepository_Open_ShouldFailForNonGitDir(t *testing.T) {
	// given
	repo := skipIfNoGit(t)
	dir := t.TempDir()

	// when
	err := repo.Open(dir)

	// then
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a git repository")
}

func TestExecGitRepository_IsClean_ShouldReturnTrueForFreshRepo(t *testing.T) {
	// given
	repo, _ := initTempRepo(t)

	// when
	clean, err := repo.IsClean()

	// then
	require.NoError(t, err)
	assert.True(t, clean)
}

func TestExecGitRepository_IsClean_ShouldReturnFalseWithUnstagedChanges(t *testing.T) {
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
	// given
	repo, _ := initTempRepo(t)

	// when
	has := repo.HasRemote()

	// then
	assert.False(t, has)
}

func TestExecGitRepository_AddRemote_ShouldMakeHasRemoteTrue(t *testing.T) {
	// given
	repo, _ := initTempRepo(t)

	// when
	err := repo.AddRemote("origin", "https://github.com/test/repo.git")

	// then
	require.NoError(t, err)
	assert.True(t, repo.HasRemote())
}

func TestExecGitRepository_SetConfig_ShouldSetValue(t *testing.T) {
	// given
	repo, dir := initTempRepo(t)

	// when
	err := repo.SetConfig("user.name", "TestUser")

	// then
	require.NoError(t, err)

	// Verify with git config
	out, _ := exec.Command("git", "-C", dir, "config", "user.name").Output() //nolint:gosec
	assert.Equal(t, "TestUser\n", string(out))
}

func TestExecGitRepository_Clone_ShouldCloneFromLocalRepo(t *testing.T) {
	// given
	repo := skipIfNoGit(t)

	// Create a source repo with a commit
	srcDir := t.TempDir()
	require.NoError(t, repo.Init(srcDir))
	_ = exec.Command("git", "-C", srcDir, "config", "user.email", "test@test.com").Run() //nolint:gosec
	_ = exec.Command("git", "-C", srcDir, "config", "user.name", "Test").Run()           //nolint:gosec
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "README.md"), []byte("# Test"), 0600))
	require.NoError(t, repo.CommitAll("initial commit"))

	// when
	destDir := filepath.Join(t.TempDir(), "cloned")
	cloneRepo := skipIfNoGit(t)
	err := cloneRepo.Clone(srcDir, destDir, "master")

	// then
	require.NoError(t, err)
	data, readErr := os.ReadFile(filepath.Join(destDir, "README.md"))
	require.NoError(t, readErr)
	assert.Equal(t, "# Test", string(data))
}

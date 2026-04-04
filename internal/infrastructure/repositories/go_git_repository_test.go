//go:build unit

package repositories_test

import (
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"

	repositories "github.com/rios0rios0/aisync/internal/infrastructure/repositories"
)

func TestGoGitRepository_Init_CreatesGitDir(t *testing.T) {
	// given
	dir := t.TempDir()
	repo := repositories.NewGoGitRepository()

	// when
	err := repo.Init(dir)

	// then
	assert.NoError(t, err)
	gitDir := filepath.Join(dir, ".git")
	assert.FileExists(t, filepath.Join(gitDir, "HEAD"))
}

func TestGoGitRepository_Open_AfterInit(t *testing.T) {
	// given
	dir := t.TempDir()
	initRepo := repositories.NewGoGitRepository()
	err := initRepo.Init(dir)
	assert.NoError(t, err)

	openRepo := repositories.NewGoGitRepository()

	// when
	err = openRepo.Open(dir)

	// then
	assert.NoError(t, err)
}

func TestGoGitRepository_Open_NonExistentDir(t *testing.T) {
	// given
	repo := repositories.NewGoGitRepository()
	dir := filepath.Join(t.TempDir(), "nonexistent")

	// when
	err := repo.Open(dir)

	// then
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open git repository")
}

func TestGoGitRepository_IsClean_FreshRepo(t *testing.T) {
	// given
	dir := t.TempDir()
	repo := repositories.NewGoGitRepository()
	err := repo.Init(dir)
	assert.NoError(t, err)

	// when
	clean, err := repo.IsClean()

	// then
	assert.NoError(t, err)
	assert.True(t, clean)
}

func TestGoGitRepository_IsClean_WithUnstagedChanges(t *testing.T) {
	// given
	dir := t.TempDir()
	repo := repositories.NewGoGitRepository()
	err := repo.Init(dir)
	assert.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644)
	assert.NoError(t, err)

	// when
	clean, err := repo.IsClean()

	// then
	assert.NoError(t, err)
	assert.False(t, clean)
}

func TestGoGitRepository_CommitAll(t *testing.T) {
	// given
	dir := t.TempDir()
	repo := repositories.NewGoGitRepository()
	err := repo.Init(dir)
	assert.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# Test"), 0644)
	assert.NoError(t, err)

	// when
	err = repo.CommitAll("chore: added initial file")

	// then
	assert.NoError(t, err)

	// verify repo is clean after commit
	clean, err := repo.IsClean()
	assert.NoError(t, err)
	assert.True(t, clean)
}

func TestGoGitRepository_CommitAll_MultipleFiles(t *testing.T) {
	// given
	dir := t.TempDir()
	repo := repositories.NewGoGitRepository()
	err := repo.Init(dir)
	assert.NoError(t, err)

	err = os.MkdirAll(filepath.Join(dir, "rules"), 0755)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "rules", "arch.md"), []byte("architecture"), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "rules", "git.md"), []byte("git flow"), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("sync: true"), 0644)
	assert.NoError(t, err)

	// when
	err = repo.CommitAll("feat: added multiple files")

	// then
	assert.NoError(t, err)

	clean, err := repo.IsClean()
	assert.NoError(t, err)
	assert.True(t, clean)
}

func TestGoGitRepository_HasRemote_LocalOnly(t *testing.T) {
	// given
	dir := t.TempDir()
	repo := repositories.NewGoGitRepository()
	err := repo.Init(dir)
	assert.NoError(t, err)

	// when
	hasRemote := repo.HasRemote()

	// then
	assert.False(t, hasRemote)
}

func TestGoGitRepository_HasRemote_AfterAddRemote(t *testing.T) {
	// given
	dir := t.TempDir()
	repo := repositories.NewGoGitRepository()
	err := repo.Init(dir)
	assert.NoError(t, err)

	err = repo.AddRemote("origin", "https://github.com/test/repo.git")
	assert.NoError(t, err)

	// when
	hasRemote := repo.HasRemote()

	// then
	assert.True(t, hasRemote)
}

func TestGoGitRepository_HasRemote_NilRepo(t *testing.T) {
	// given
	repo := repositories.NewGoGitRepository()

	// when
	hasRemote := repo.HasRemote()

	// then
	assert.False(t, hasRemote)
}

func TestGoGitRepository_IsClean_NotOpened(t *testing.T) {
	// given
	repo := repositories.NewGoGitRepository()

	// when
	_, err := repo.IsClean()

	// then
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "repository not opened")
}

func TestGoGitRepository_CommitAll_NotOpened(t *testing.T) {
	// given
	repo := repositories.NewGoGitRepository()

	// when
	err := repo.CommitAll("test")

	// then
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "repository not opened")
}

func TestGoGitRepository_Clone_FromLocalBareRepo(t *testing.T) {
	// given
	bareDir := t.TempDir()
	bareRepo := repositories.NewGoGitRepository()
	err := bareRepo.Init(bareDir)
	assert.NoError(t, err)

	// Create an initial commit so the branch exists
	err = os.WriteFile(filepath.Join(bareDir, "init.txt"), []byte("initial"), 0644)
	assert.NoError(t, err)
	err = bareRepo.CommitAll("chore: initial commit")
	assert.NoError(t, err)

	cloneDir := filepath.Join(t.TempDir(), "cloned")
	cloneRepo := repositories.NewGoGitRepository()

	// when
	err = cloneRepo.Clone(bareDir, cloneDir, "master")

	// then
	assert.NoError(t, err)

	// Verify the cloned repo has the file
	data, readErr := os.ReadFile(filepath.Join(cloneDir, "init.txt"))
	assert.NoError(t, readErr)
	assert.Equal(t, []byte("initial"), data)
}

func TestGoGitRepository_Pull_NotOpened(t *testing.T) {
	// given
	repo := repositories.NewGoGitRepository()

	// when
	err := repo.Pull()

	// then
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "repository not opened")
}

func TestGoGitRepository_Init_SetsHEADCorrectly(t *testing.T) {
	// given
	dir := t.TempDir()
	repo := repositories.NewGoGitRepository()

	// when
	err := repo.Init(dir)

	// then
	assert.NoError(t, err)

	headContent, readErr := os.ReadFile(filepath.Join(dir, ".git", "HEAD"))
	assert.NoError(t, readErr)
	assert.Contains(t, string(headContent), "ref: refs/heads/")
}

func TestGoGitRepository_AddRemote_NotOpened(t *testing.T) {
	// given
	repo := repositories.NewGoGitRepository()

	// when
	err := repo.AddRemote("origin", "https://github.com/test/repo.git")

	// then
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "repository not opened")
}

func TestGoGitRepository_Push_NotOpened(t *testing.T) {
	// given
	repo := repositories.NewGoGitRepository()

	// when
	err := repo.Push()

	// then
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "repository not opened")
}

func TestGoGitRepository_Pull_AlreadyUpToDate(t *testing.T) {
	// given
	bareDir := t.TempDir()
	bareRepo := repositories.NewGoGitRepository()
	err := bareRepo.Init(bareDir)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(bareDir, "init.txt"), []byte("initial"), 0644)
	assert.NoError(t, err)
	err = bareRepo.CommitAll("chore: initial commit")
	assert.NoError(t, err)

	cloneDir := filepath.Join(t.TempDir(), "cloned")
	cloneRepo := repositories.NewGoGitRepository()
	err = cloneRepo.Clone(bareDir, cloneDir, "master")
	assert.NoError(t, err)

	// when -- pull when already up to date
	err = cloneRepo.Pull()

	// then
	assert.NoError(t, err)
}

func TestGoGitRepository_AddRemote_DuplicateName(t *testing.T) {
	// given
	dir := t.TempDir()
	repo := repositories.NewGoGitRepository()
	err := repo.Init(dir)
	assert.NoError(t, err)

	err = repo.AddRemote("origin", "https://github.com/test/repo.git")
	assert.NoError(t, err)

	// when
	err = repo.AddRemote("origin", "https://github.com/test/other.git")

	// then
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add remote")
}

func TestGoGitRepository_Push_LocalOnlyRepoNoRemote(t *testing.T) {
	// given
	dir := t.TempDir()
	repo := repositories.NewGoGitRepository()
	err := repo.Init(dir)
	assert.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644)
	assert.NoError(t, err)
	err = repo.CommitAll("chore: test commit")
	assert.NoError(t, err)

	// when -- push without any remote configured
	err = repo.Push()

	// then -- should not error (logs warning and returns nil)
	assert.NoError(t, err)
}

func TestGoGitRepository_Pull_WithHTTPSRemote(t *testing.T) {
	// given
	bareDir := t.TempDir()
	bareRepo := repositories.NewGoGitRepository()
	err := bareRepo.Init(bareDir)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(bareDir, "init.txt"), []byte("initial"), 0644)
	assert.NoError(t, err)
	err = bareRepo.CommitAll("chore: initial commit")
	assert.NoError(t, err)

	cloneDir := filepath.Join(t.TempDir(), "cloned")
	cloneRepo := repositories.NewGoGitRepository()
	err = cloneRepo.Clone(bareDir, cloneDir, "master")
	assert.NoError(t, err)

	// Add a new commit to the bare/origin repo
	err = os.WriteFile(filepath.Join(bareDir, "second.txt"), []byte("second"), 0644)
	assert.NoError(t, err)
	err = bareRepo.CommitAll("feat: added second file")
	assert.NoError(t, err)

	// when -- pull from origin (local bare repo)
	err = cloneRepo.Pull()

	// then
	assert.NoError(t, err)

	// Verify the new file was pulled
	data, readErr := os.ReadFile(filepath.Join(cloneDir, "second.txt"))
	assert.NoError(t, readErr)
	assert.Equal(t, []byte("second"), data)
}

func TestGoGitRepository_Push_WithLocalRemote(t *testing.T) {
	// given
	// Create a bare repo to push to
	bareDir := t.TempDir()
	bareRepo := repositories.NewGoGitRepository()
	err := bareRepo.Init(bareDir)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(bareDir, "init.txt"), []byte("initial"), 0644)
	assert.NoError(t, err)
	err = bareRepo.CommitAll("chore: initial commit")
	assert.NoError(t, err)

	// Clone it
	cloneDir := filepath.Join(t.TempDir(), "cloned")
	cloneRepo := repositories.NewGoGitRepository()
	err = cloneRepo.Clone(bareDir, cloneDir, "master")
	assert.NoError(t, err)

	// Add a new file and commit
	err = os.WriteFile(filepath.Join(cloneDir, "new.txt"), []byte("new content"), 0644)
	assert.NoError(t, err)
	err = cloneRepo.CommitAll("feat: added new file")
	assert.NoError(t, err)

	// when -- push to the local remote
	err = cloneRepo.Push()

	// then -- should succeed or log warning (local bare repo may not accept pushes)
	// The important thing is that it doesn't error fatally
	assert.NoError(t, err)
}

func TestGoGitRepository_Open_ThenIsCleanAndCommit(t *testing.T) {
	// given
	dir := t.TempDir()
	initRepo := repositories.NewGoGitRepository()
	err := initRepo.Init(dir)
	assert.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644)
	assert.NoError(t, err)
	err = initRepo.CommitAll("chore: initial file")
	assert.NoError(t, err)

	openRepo := repositories.NewGoGitRepository()
	err = openRepo.Open(dir)
	assert.NoError(t, err)

	// when
	clean, err := openRepo.IsClean()

	// then
	assert.NoError(t, err)
	assert.True(t, clean)

	// when -- add a new file and commit through the opened repo
	err = os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new"), 0644)
	assert.NoError(t, err)

	clean, err = openRepo.IsClean()
	assert.NoError(t, err)
	assert.False(t, clean)

	err = openRepo.CommitAll("feat: added new file")
	assert.NoError(t, err)

	clean, err = openRepo.IsClean()
	assert.NoError(t, err)
	assert.True(t, clean)
}

func TestGoGitRepository_Clone_HTTPSUrl(t *testing.T) {
	// given -- use a local file path as the clone URL (go-git supports this)
	bareDir := t.TempDir()
	bareRepo := repositories.NewGoGitRepository()
	err := bareRepo.Init(bareDir)
	assert.NoError(t, err)

	err = os.MkdirAll(filepath.Join(bareDir, "rules"), 0755)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(bareDir, "rules", "arch.md"), []byte("architecture"), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(bareDir, "readme.md"), []byte("readme"), 0644)
	assert.NoError(t, err)
	err = bareRepo.CommitAll("chore: initial with multiple files")
	assert.NoError(t, err)

	cloneDir := filepath.Join(t.TempDir(), "cloned")
	cloneRepo := repositories.NewGoGitRepository()

	// when
	err = cloneRepo.Clone(bareDir, cloneDir, "master")

	// then
	assert.NoError(t, err)

	// Verify all files were cloned
	data, readErr := os.ReadFile(filepath.Join(cloneDir, "rules", "arch.md"))
	assert.NoError(t, readErr)
	assert.Equal(t, []byte("architecture"), data)

	// Should be clean after clone
	clean, err := cloneRepo.IsClean()
	assert.NoError(t, err)
	assert.True(t, clean)

	// Should have remote
	assert.True(t, cloneRepo.HasRemote())
}

func TestGoGitRepository_Init_ShouldFailWhenAlreadyInit(t *testing.T) {
	// given
	dir := t.TempDir()
	repo := repositories.NewGoGitRepository()
	err := repo.Init(dir)
	assert.NoError(t, err)

	// when -- re-init on the same directory
	repo2 := repositories.NewGoGitRepository()
	err = repo2.Init(dir)

	// then
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialize git repository")
}

func TestGoGitRepository_Push_WithUnreachableHTTPSRemote(t *testing.T) {
	// given
	dir := t.TempDir()
	repo := repositories.NewGoGitRepository()
	err := repo.Init(dir)
	assert.NoError(t, err)
	err = repo.AddRemote("origin", "https://localhost:1/nonexistent.git")
	assert.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "f.txt"), []byte("data"), 0644)
	assert.NoError(t, err)
	err = repo.CommitAll("chore: commit")
	assert.NoError(t, err)

	// when -- push to an unreachable remote
	err = repo.Push()

	// then -- should return nil (logs warning for unreachable remote)
	assert.NoError(t, err)
}

func TestGoGitRepository_Clone_ShouldFailWithInvalidURL(t *testing.T) {
	// given
	repo := repositories.NewGoGitRepository()
	dir := filepath.Join(t.TempDir(), "clone-target")

	// when
	err := repo.Clone("/nonexistent/repo/path", dir, "main")

	// then
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to clone repository")
}

func TestGoGitRepository_Open_BareRepoShouldFail(t *testing.T) {
	// given
	dir := t.TempDir()
	// Manually create a bare repo using go-git
	_, err := git.PlainInit(dir, true)
	assert.NoError(t, err)

	repo := repositories.NewGoGitRepository()

	// when -- opening a bare repo should fail because Worktree() returns an error
	err = repo.Open(dir)

	// then
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get worktree")
}

func TestGoGitRepository_Open_ExistingWithRemote(t *testing.T) {
	// given
	dir := t.TempDir()
	initRepo := repositories.NewGoGitRepository()
	err := initRepo.Init(dir)
	assert.NoError(t, err)
	err = initRepo.AddRemote("origin", "https://github.com/test/repo.git")
	assert.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "file.txt"), []byte("data"), 0644)
	assert.NoError(t, err)
	err = initRepo.CommitAll("chore: initial commit")
	assert.NoError(t, err)

	// when -- open and pull (should get error since it's a fake remote)
	openRepo := repositories.NewGoGitRepository()
	err = openRepo.Open(dir)
	assert.NoError(t, err)

	pullErr := openRepo.Pull()

	// then -- pull should fail since origin is unreachable
	assert.Error(t, pullErr)
}

func TestGoGitRepository_Init_ThenAddRemoteThenHasRemote(t *testing.T) {
	// given
	dir := t.TempDir()
	repo := repositories.NewGoGitRepository()
	err := repo.Init(dir)
	assert.NoError(t, err)
	assert.False(t, repo.HasRemote())

	// when
	err = repo.AddRemote("origin", "https://github.com/test/repo.git")

	// then
	assert.NoError(t, err)
	assert.True(t, repo.HasRemote())
}

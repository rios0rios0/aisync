//go:build unit

package repositories_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainRepos "github.com/rios0rios0/aisync/internal/domain/repositories"
	"github.com/rios0rios0/aisync/internal/infrastructure/repositories"
)

// requireGit skips the test if the system `git` binary is not on PATH.
// ExecGitInspector requires `git` for `LocalRemotes` and `EmailDomain`,
// so tests that exercise those methods need a real git installation.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not on PATH, skipping ExecGitInspector test: %v", err)
	}
}

func TestExecGitInspector_DirectoryLayout(t *testing.T) {
	t.Parallel()

	t.Run("should enumerate first-level org subdirectories under each forge", func(t *testing.T) {
		t.Parallel()

		// given — synthetic dev root with one org under each forge.
		requireGit(t)
		root := t.TempDir()
		for _, forge := range []string{"github.com", "gitlab.com", "bitbucket.org"} {
			require.NoError(t, os.MkdirAll(filepath.Join(root, forge, "AcmeOrg"), 0o755))
		}

		inspector, err := repositories.NewExecGitInspector()
		require.NoError(t, err)

		// when
		terms, err := inspector.DirectoryLayout([]string{root})

		// then
		require.NoError(t, err)
		values := termValues(terms)
		assert.Contains(t, values, "AcmeOrg")
		// One AcmeOrg per public forge — no extra ADO descent for github/gitlab/bitbucket.
		count := 0
		for _, v := range values {
			if v == "AcmeOrg" {
				count++
			}
		}
		assert.Equal(t, 3, count, "AcmeOrg should appear once per public forge, with no second-level descent")
	})

	t.Run("should descend one extra level under dev.azure.com to capture <project> names", func(t *testing.T) {
		t.Parallel()

		// given — `<root>/dev.azure.com/AcmeOrg/{ProjectA,ProjectB}/`
		// is the canonical ADO layout. DirectoryLayout must enumerate
		// both the org AND each project subdirectory.
		requireGit(t)
		root := t.TempDir()
		ado := filepath.Join(root, "dev.azure.com", "AcmeOrg")
		require.NoError(t, os.MkdirAll(filepath.Join(ado, "ProjectA"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(ado, "ProjectB"), 0o755))

		inspector, err := repositories.NewExecGitInspector()
		require.NoError(t, err)

		// when
		terms, err := inspector.DirectoryLayout([]string{root})

		// then
		require.NoError(t, err)
		values := termValues(terms)
		assert.Contains(t, values, "AcmeOrg", "the org level must be included")
		assert.Contains(t, values, "ProjectA", "ADO project subdirectory must be included")
		assert.Contains(t, values, "ProjectB", "ADO project subdirectory must be included")
	})

	t.Run("should NOT descend two levels under public forges", func(t *testing.T) {
		t.Parallel()

		// given — github.com/AcmeOrg/Repo1/ exists, but the second
		// level (Repo1) must NOT be reported as a derived term: it's
		// the repo name, not an org name.
		requireGit(t)
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(
			filepath.Join(root, "github.com", "AcmeOrg", "Repo1"), 0o755))

		inspector, err := repositories.NewExecGitInspector()
		require.NoError(t, err)

		// when
		terms, err := inspector.DirectoryLayout([]string{root})

		// then
		require.NoError(t, err)
		values := termValues(terms)
		assert.Contains(t, values, "AcmeOrg")
		assert.NotContains(t, values, "Repo1",
			"public forges must not descend into <repo> level")
	})

	t.Run("should tag each derived term with its filesystem origin", func(t *testing.T) {
		t.Parallel()

		// given
		requireGit(t)
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(
			filepath.Join(root, "github.com", "AcmeOrg"), 0o755))

		inspector, err := repositories.NewExecGitInspector()
		require.NoError(t, err)

		// when
		terms, err := inspector.DirectoryLayout([]string{root})

		// then
		require.NoError(t, err)
		require.Len(t, terms, 1)
		assert.Equal(t, "AcmeOrg", terms[0].Value)
		assert.Contains(t, terms[0].Origin, "fs:")
		assert.Contains(t, terms[0].Origin, "github.com")
	})

	t.Run("should silently tolerate missing forge subdirectories", func(t *testing.T) {
		t.Parallel()

		// given — empty dev root with no forge subdirs at all.
		requireGit(t)
		root := t.TempDir()

		inspector, err := repositories.NewExecGitInspector()
		require.NoError(t, err)

		// when
		terms, err := inspector.DirectoryLayout([]string{root})

		// then
		require.NoError(t, err)
		assert.Empty(t, terms)
	})
}

func TestExecGitInspector_LocalRemotes(t *testing.T) {
	t.Parallel()

	t.Run("should parse a github SSH remote into the owner segment", func(t *testing.T) {
		t.Parallel()

		// given — a real git repo with a hand-set remote URL. This
		// exercises parseRemoteURL → parseSSHRemote → parseRemotePath
		// end-to-end via the public LocalRemotes API.
		requireGit(t)
		root := initGitRepoWithRemote(t, "git@github.com:AcmeOrg/some-repo.git")

		inspector, err := repositories.NewExecGitInspector()
		require.NoError(t, err)

		// when
		terms, err := inspector.LocalRemotes([]string{root}, 4)

		// then
		require.NoError(t, err)
		values := derivedTermValues(terms)
		assert.Contains(t, values, "AcmeOrg")
	})

	t.Run("should parse an ADO SSH remote into <org> and <project>", func(t *testing.T) {
		t.Parallel()

		// given — ADO SSH URLs use `v3/<org>/<project>/<repo>`. The
		// parser must surface BOTH the org and the project as terms.
		requireGit(t)
		root := initGitRepoWithRemote(t, "git@ssh.dev.azure.com:v3/AcmeOrg/AcmeProject/some-repo")

		inspector, err := repositories.NewExecGitInspector()
		require.NoError(t, err)

		// when
		terms, err := inspector.LocalRemotes([]string{root}, 4)

		// then
		require.NoError(t, err)
		values := derivedTermValues(terms)
		assert.Contains(t, values, "AcmeOrg", "ADO org segment must be derived")
		assert.Contains(t, values, "AcmeProject", "ADO project segment must be derived")
	})

	t.Run("should parse an ADO SSH remote with -alias suffix", func(t *testing.T) {
		t.Parallel()

		// given — `git@ssh.dev.azure.com-arancia:v3/...` is the form
		// that emerges when the user has an SSH host alias for ADO
		// (e.g. multiple ADO accounts on the same machine).
		requireGit(t)
		root := initGitRepoWithRemote(t, "git@ssh.dev.azure.com-arancia:v3/AcmeOrg/AcmeProject/some-repo")

		inspector, err := repositories.NewExecGitInspector()
		require.NoError(t, err)

		// when
		terms, err := inspector.LocalRemotes([]string{root}, 4)

		// then
		require.NoError(t, err)
		values := derivedTermValues(terms)
		assert.Contains(t, values, "AcmeOrg")
		assert.Contains(t, values, "AcmeProject")
	})

	t.Run("should parse an HTTPS gitlab remote into the owner segment", func(t *testing.T) {
		t.Parallel()

		// given
		requireGit(t)
		root := initGitRepoWithRemote(t, "https://gitlab.com/AcmeOrg/some-repo.git")

		inspector, err := repositories.NewExecGitInspector()
		require.NoError(t, err)

		// when
		terms, err := inspector.LocalRemotes([]string{root}, 4)

		// then
		require.NoError(t, err)
		values := derivedTermValues(terms)
		assert.Contains(t, values, "AcmeOrg")
	})

	t.Run("should fold a self-hosted host into both host and owner candidates", func(t *testing.T) {
		t.Parallel()

		// given — for non-public forges the parser emits BOTH the host
		// and the first path segment because either could be a sensitive
		// internal identifier.
		requireGit(t)
		root := initGitRepoWithRemote(t, "git@git.internal.corp:AcmeTeam/some-repo.git")

		inspector, err := repositories.NewExecGitInspector()
		require.NoError(t, err)

		// when
		terms, err := inspector.LocalRemotes([]string{root}, 4)

		// then
		require.NoError(t, err)
		values := derivedTermValues(terms)
		assert.Contains(t, values, "git.internal.corp")
		assert.Contains(t, values, "AcmeTeam")
	})
}

// TestExecGitInspector_SSHHostAliases must NOT call t.Parallel() at any
// level because t.Setenv panics inside parallel tests, and SSHHostAliases
// reads `~/.ssh/config` via os.UserHomeDir(), which respects $HOME.
func TestExecGitInspector_SSHHostAliases(t *testing.T) {
	t.Run("should extract <alias> segments from `Host <forge>-<alias>` entries", func(t *testing.T) {
		// given — a synthetic ~/.ssh/config with a mix of ADO and
		// github aliases, plus an unrelated host that must NOT be
		// emitted.
		requireGit(t)
		home := t.TempDir()
		t.Setenv("HOME", home)
		require.NoError(t, os.MkdirAll(filepath.Join(home, ".ssh"), 0o700))
		cfg := `# ssh config
Host dev.azure.com-arancia
    HostName ssh.dev.azure.com
    User git

Host github.com-personal
    HostName github.com
    User git

Host random.example.com
    HostName random.example.com
`
		require.NoError(t, os.WriteFile(
			filepath.Join(home, ".ssh", "config"), []byte(cfg), 0o600))

		inspector, err := repositories.NewExecGitInspector()
		require.NoError(t, err)

		// when
		terms, err := inspector.SSHHostAliases()

		// then
		require.NoError(t, err)
		values := derivedTermValues(terms)
		assert.Contains(t, values, "arancia")
		assert.Contains(t, values, "personal")
		assert.NotContains(t, values, "random.example.com",
			"unrecognized host without `<forge>-<alias>` shape must not be emitted")
	})

	t.Run("should return nil when ~/.ssh/config does not exist", func(t *testing.T) {
		// given — empty home, no .ssh/config at all.
		requireGit(t)
		home := t.TempDir()
		t.Setenv("HOME", home)

		inspector, err := repositories.NewExecGitInspector()
		require.NoError(t, err)

		// when
		terms, err := inspector.SSHHostAliases()

		// then
		require.NoError(t, err)
		assert.Empty(t, terms)
	})
}

// initGitRepoWithRemote creates a real git repo under a fresh temp dir
// (so the parent path doesn't pollute LocalRemotes) and sets `origin` to
// the given URL. Returns the dev-root path that should be passed to
// LocalRemotes — the parent of the repo, not the repo itself, because
// LocalRemotes walks DOWN from the dev root.
func initGitRepoWithRemote(t *testing.T, remoteURL string) string {
	t.Helper()
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	require.NoError(t, os.MkdirAll(repoDir, 0o755))

	// Use the real git binary to make a syntactically-valid repo. This
	// is necessary because LocalRemotes calls `git remote get-url
	// origin` and parses real git output.
	mustGit(t, repoDir, "init", "--quiet")
	mustGit(t, repoDir, "remote", "add", "origin", remoteURL)
	return root
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(out))
}

// termValues extracts just the .Value field of a slice of DerivedTerms.
// Sorted for stable assertion error messages.
func termValues(terms []domainRepos.DerivedTerm) []string {
	out := make([]string, 0, len(terms))
	for _, t := range terms {
		out = append(out, t.Value)
	}
	sort.Strings(out)
	return out
}

// derivedTermValues is a compatibility alias for [termValues] kept for
// readability inside the LocalRemotes / SSHHostAliases tests.
func derivedTermValues(terms []domainRepos.DerivedTerm) []string {
	return termValues(terms)
}

package repositories

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// repoHistoryTimeout bounds a single `git log` lookup so a pathological
// repository can never stall a conflict prompt.
const repoHistoryTimeout = 10 * time.Second

// ExecGitRepoHistory implements RepoHistory with the system git binary. It
// only reads history that is already on disk, so it works the same whether
// the sync repo is driven by go-git or by --use-system-git. When git is not
// on PATH every lookup returns a zero time, which callers treat as unknown.
type ExecGitRepoHistory struct {
	gitPath string
}

// NewExecGitRepoHistory creates an ExecGitRepoHistory, degrading to "history
// unavailable" when the git binary is missing instead of failing startup.
func NewExecGitRepoHistory() *ExecGitRepoHistory {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		gitPath = ""
	}
	return &ExecGitRepoHistory{gitPath: gitPath}
}

// LastChange returns the time and subject of the latest commit touching
// relPath in the repository at repoPath.
func (h *ExecGitRepoHistory) LastChange(repoPath, relPath string) (time.Time, string, error) {
	if h.gitPath == "" {
		return time.Time{}, "", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), repoHistoryTimeout)
	defer cancel()

	cmd := exec.CommandContext( //nolint:gosec // gitPath comes from exec.LookPath; args are fixed apart from the path
		ctx, h.gitPath, "-C", repoPath, "log", "-1", "--format=%ct%x09%s", "--", relPath,
	)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	out, err := cmd.Output()
	if err != nil {
		return time.Time{}, "", fmt.Errorf("git log for %s: %w", relPath, err)
	}
	return parseLastChange(string(out))
}

// parseLastChange decodes the "<unix seconds>\t<subject>" line printed by
// `git log -1 --format=%ct%x09%s`; empty output means the path has no history.
func parseLastChange(output string) (time.Time, string, error) {
	line := strings.TrimSpace(output)
	if line == "" {
		return time.Time{}, "", nil
	}

	seconds, subject, _ := strings.Cut(line, "\t")
	unix, err := strconv.ParseInt(seconds, 10, 64)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("unexpected git log output %q: %w", line, err)
	}
	return time.Unix(unix, 0), subject, nil
}

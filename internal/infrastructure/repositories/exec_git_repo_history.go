package repositories

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

const (
	// repoHistoryTimeout bounds a single `git log` lookup so a pathological
	// repository can never stall a conflict prompt.
	repoHistoryTimeout = 10 * time.Second
	// repoHistoryDepth bounds how many commits a lookup reads while skipping
	// encryption-only ones.
	repoHistoryDepth = 50
	// fieldSeparator and recordSeparator delimit the `git log` output, so a
	// subject containing tabs or a multi-line trailer cannot be misparsed.
	fieldSeparator  = "\x1f"
	recordSeparator = "\x1e"
)

// repoHistoryFormat prints, per commit, the commit time, the subject, and
// the values of the aisync change trailer.
//
//nolint:gochecknoglobals // derived from compile-time constants
var repoHistoryFormat = "--format=%ct%x1f%s%x1f%(trailers:key=" + entities.ChangeTrailerKey +
	",valueonly,separator=%x2C)%x1e"

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

// LastChange returns the time and subject of the latest commit touching any
// of relPaths in the repository at repoPath, skipping encryption-only commits.
func (h *ExecGitRepoHistory) LastChange(repoPath string, relPaths ...string) (time.Time, string, error) {
	if h.gitPath == "" || len(relPaths) == 0 {
		return time.Time{}, "", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), repoHistoryTimeout)
	defer cancel()

	args := []string{"-C", repoPath, "log", "--max-count=" + strconv.Itoa(repoHistoryDepth), repoHistoryFormat, "--"}
	cmd := exec.CommandContext( //nolint:gosec // gitPath comes from exec.LookPath; args are fixed apart from the paths
		ctx, h.gitPath, append(args, relPaths...)...,
	)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	out, err := cmd.Output()
	if err != nil {
		return time.Time{}, "", fmt.Errorf("git log for %s: %w", strings.Join(relPaths, ", "), err)
	}
	return parseLastChange(string(out))
}

// parseLastChange returns the time and subject of the newest commit printed
// with repoHistoryFormat that is not an encryption-only commit; output
// without such a commit means the paths have no history.
func parseLastChange(output string) (time.Time, string, error) {
	for record := range strings.SplitSeq(output, recordSeparator) {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}

		fields := strings.SplitN(record, fieldSeparator, 3) //nolint:mnd // time, subject, trailers
		if len(fields) == 3 && slices.Contains(strings.Split(fields[2], ","), entities.ChangeEncryptionOnly) {
			continue
		}
		unix, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil || len(fields) < 2 {
			return time.Time{}, "", fmt.Errorf("unexpected git log output %q", record)
		}
		return time.Unix(unix, 0), fields[1], nil
	}
	return time.Time{}, "", nil
}

package repositories

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	logger "github.com/sirupsen/logrus"
)

// ExecGitRepository implements GitRepository by shelling out to the system
// git binary. This provides a fallback for environments where go-git has
// compatibility issues (e.g., Git LFS, advanced SSH configurations).
type ExecGitRepository struct {
	dir     string
	gitPath string
}

// NewExecGitRepository creates a new ExecGitRepository. It returns an error if
// the git binary is not found on PATH.
func NewExecGitRepository() (*ExecGitRepository, error) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("git binary not found on PATH: %w", err)
	}
	return &ExecGitRepository{gitPath: gitPath}, nil
}

// Clone clones a remote repository into the given directory.
func (r *ExecGitRepository) Clone(url, dir, branch string) error {
	_, err := r.run("", "clone", "--branch", branch, "--single-branch", url, dir)
	if err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}
	r.dir = dir
	return nil
}

// Init initializes a new git repository at the given directory.
func (r *ExecGitRepository) Init(dir string) error {
	_, err := r.run("", "init", dir)
	if err != nil {
		return fmt.Errorf("git init failed: %w", err)
	}
	r.dir = dir
	return nil
}

// Open sets the working directory for subsequent git operations. It verifies
// that a .git directory exists.
func (r *ExecGitRepository) Open(dir string) error {
	gitDir := dir + "/.git"
	if info, err := os.Stat(gitDir); err != nil || !info.IsDir() {
		return fmt.Errorf("not a git repository: %s", dir)
	}
	r.dir = dir
	return nil
}

// Pull fetches and merges from the remote origin.
func (r *ExecGitRepository) Pull() error {
	_, err := r.run(r.dir, "pull", "origin")
	if err != nil {
		// "Already up to date." is not an error
		if strings.Contains(err.Error(), "Already up to date") {
			return nil
		}
		return fmt.Errorf("git pull failed: %w", err)
	}
	return nil
}

// CommitAll stages all changes and creates a commit.
func (r *ExecGitRepository) CommitAll(message string) error {
	if _, err := r.run(r.dir, "add", "."); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	_, err := r.run(r.dir, "commit", "-m", message, "--author", "aisync <aisync@local>")
	if err != nil {
		// "nothing to commit" is not an error
		if strings.Contains(err.Error(), "nothing to commit") {
			return nil
		}
		return fmt.Errorf("git commit failed: %w", err)
	}
	return nil
}

// Push pushes commits to the remote origin.
func (r *ExecGitRepository) Push() error {
	_, err := r.run(r.dir, "push", "origin")
	if err != nil {
		logger.Warnf("git push failed (offline?): %v", err)
		return fmt.Errorf("git push failed: %w", err)
	}
	return nil
}

// IsClean returns true if the working tree has no uncommitted changes.
func (r *ExecGitRepository) IsClean() (bool, error) {
	out, err := r.run(r.dir, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("git status failed: %w", err)
	}
	return strings.TrimSpace(out) == "", nil
}

// HasRemote returns true if the repository has a remote named "origin".
func (r *ExecGitRepository) HasRemote() bool {
	_, err := r.run(r.dir, "remote", "get-url", "origin")
	return err == nil
}

// SetConfig sets a git config key-value pair in the local repository config.
func (r *ExecGitRepository) SetConfig(key, value string) error {
	_, err := r.run(r.dir, "config", "--local", key, value)
	if err != nil {
		return fmt.Errorf("git config failed: %w", err)
	}
	return nil
}

// AddRemote adds a named remote to the repository.
func (r *ExecGitRepository) AddRemote(name, url string) error {
	_, err := r.run(r.dir, "remote", "add", name, url)
	if err != nil {
		return fmt.Errorf("git remote add failed: %w", err)
	}
	return nil
}

// run executes a git command and returns its combined stdout output.
func (r *ExecGitRepository) run(dir string, args ...string) (string, error) {
	cmd := exec.CommandContext( //nolint:gosec // gitPath is validated at construction time via exec.LookPath
		context.Background(),
		r.gitPath,
		args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	out, err := cmd.CombinedOutput()
	output := string(out)

	logger.Debugf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(output))

	if err != nil {
		return output, fmt.Errorf("%w: %s", err, strings.TrimSpace(output))
	}
	return output, nil
}

package repositories

import (
	"fmt"
	"os"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	logger "github.com/sirupsen/logrus"
)

// GoGitRepository implements GitRepository using go-git.
type GoGitRepository struct {
	repo     *git.Repository
	worktree *git.Worktree
}

// NewGoGitRepository creates a new GoGitRepository.
func NewGoGitRepository() *GoGitRepository {
	return &GoGitRepository{}
}

// Clone clones a remote repository into the given directory, checking out the specified branch.
func (r *GoGitRepository) Clone(url, dir, branch string) error {
	opts := &git.CloneOptions{
		URL:           url,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
		Depth:         0,
	}

	if strings.HasPrefix(url, "git@") {
		auth, err := sshAuthFromAgent()
		if err != nil {
			return fmt.Errorf("failed to configure SSH auth: %w", err)
		}
		opts.Auth = auth
	}

	repo, err := git.PlainClone(dir, false, opts)
	if err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree after clone: %w", err)
	}

	r.repo = repo
	r.worktree = worktree

	return nil
}

// Init initializes a new Git repository at the given directory.
func (r *GoGitRepository) Init(dir string) error {
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		return fmt.Errorf("failed to initialize git repository: %w", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree after init: %w", err)
	}

	r.repo = repo
	r.worktree = worktree

	return nil
}

// Open opens an existing Git repository at the given directory.
func (r *GoGitRepository) Open(dir string) error {
	repo, err := git.PlainOpen(dir)
	if err != nil {
		return fmt.Errorf("failed to open git repository at %s: %w", dir, err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	r.repo = repo
	r.worktree = worktree

	return nil
}

// Pull fetches and merges changes from the remote origin.
func (r *GoGitRepository) Pull() error {
	if r.worktree == nil {
		return fmt.Errorf("repository not opened; call Open or Clone first")
	}

	opts := &git.PullOptions{
		RemoteName: "origin",
	}

	if err := r.addSSHAuthIfNeeded(opts); err != nil {
		return err
	}

	err := r.worktree.Pull(opts)
	if err == git.NoErrAlreadyUpToDate {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to pull: %w", err)
	}

	return nil
}

// CommitAll stages all changes and creates a commit with the given message.
func (r *GoGitRepository) CommitAll(message string) error {
	if r.worktree == nil {
		return fmt.Errorf("repository not opened; call Open or Clone first")
	}

	if _, err := r.worktree.Add("."); err != nil {
		return fmt.Errorf("failed to stage changes: %w", err)
	}

	_, err := r.worktree.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "aisync",
			Email: "aisync@local",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	return nil
}

// Push pushes committed changes to the remote origin. If the remote is unreachable,
// it logs a warning and returns nil to support offline workflows.
func (r *GoGitRepository) Push() error {
	if r.repo == nil {
		return fmt.Errorf("repository not opened; call Open or Clone first")
	}

	opts := &git.PushOptions{}

	if err := r.addSSHAuthToPushIfNeeded(opts); err != nil {
		return err
	}

	err := r.repo.Push(opts)
	if err == git.NoErrAlreadyUpToDate {
		return nil
	}
	if err != nil {
		logger.Warnf("push failed (offline or unreachable remote): %v", err)
		return nil
	}

	return nil
}

// IsClean returns true if the worktree has no modifications.
func (r *GoGitRepository) IsClean() (bool, error) {
	if r.worktree == nil {
		return false, fmt.Errorf("repository not opened; call Open or Clone first")
	}

	status, err := r.worktree.Status()
	if err != nil {
		return false, fmt.Errorf("failed to get worktree status: %w", err)
	}

	return status.IsClean(), nil
}

// HasRemote returns true if the repository has a remote named "origin".
func (r *GoGitRepository) HasRemote() bool {
	if r.repo == nil {
		return false
	}

	_, err := r.repo.Remote("origin")
	return err == nil
}

// addSSHAuthIfNeeded inspects the origin remote URL and configures SSH auth on the
// PullOptions when the URL uses the git@ scheme.
func (r *GoGitRepository) addSSHAuthIfNeeded(opts *git.PullOptions) error {
	remoteURL, err := r.originURL()
	if err != nil {
		return nil // no remote configured; skip auth
	}

	if strings.HasPrefix(remoteURL, "git@") {
		auth, err := sshAuthFromAgent()
		if err != nil {
			return fmt.Errorf("failed to configure SSH auth for pull: %w", err)
		}
		opts.Auth = auth
	}

	return nil
}

// addSSHAuthToPushIfNeeded inspects the origin remote URL and configures SSH auth on
// the PushOptions when the URL uses the git@ scheme.
func (r *GoGitRepository) addSSHAuthToPushIfNeeded(opts *git.PushOptions) error {
	remoteURL, err := r.originURL()
	if err != nil {
		return nil // no remote configured; skip auth
	}

	if strings.HasPrefix(remoteURL, "git@") {
		auth, err := sshAuthFromAgent()
		if err != nil {
			return fmt.Errorf("failed to configure SSH auth for push: %w", err)
		}
		opts.Auth = auth
	}

	return nil
}

// originURL returns the first URL of the "origin" remote, or an error if none exists.
func (r *GoGitRepository) originURL() (string, error) {
	remote, err := r.repo.Remote("origin")
	if err != nil {
		return "", err
	}

	cfg := remote.Config()
	if len(cfg.URLs) == 0 {
		return "", fmt.Errorf("origin remote has no URLs")
	}

	return cfg.URLs[0], nil
}

// AddRemote adds a named remote to the repository.
func (r *GoGitRepository) AddRemote(name, url string) error {
	if r.repo == nil {
		return fmt.Errorf("repository not opened; call Open or Init first")
	}

	_, err := r.repo.CreateRemote(&config.RemoteConfig{
		Name: name,
		URLs: []string{url},
	})
	if err != nil {
		return fmt.Errorf("failed to add remote '%s': %w", name, err)
	}

	return nil
}

// SetConfig sets a git config key-value pair in the local repository config.
// The key uses dot notation (e.g., "filter.aisync-crypt.clean").
func (r *GoGitRepository) SetConfig(key, value string) error {
	if r.repo == nil {
		return fmt.Errorf("repository not opened; call Open or Init first")
	}

	cfg, err := r.repo.Config()
	if err != nil {
		return fmt.Errorf("failed to read git config: %w", err)
	}

	// Parse "section.subsection.key" or "section.key" from dot notation.
	parts := strings.SplitN(key, ".", 3)
	switch len(parts) {
	case 3:
		cfg.Raw.SetOption(parts[0], parts[1], parts[2], value)
	case 2:
		cfg.Raw.SetOption(parts[0], "", parts[1], value)
	default:
		return fmt.Errorf("invalid config key: %s", key)
	}

	if err := r.repo.SetConfig(cfg); err != nil {
		return fmt.Errorf("failed to write git config: %w", err)
	}
	return nil
}

// sshAuthFromAgent creates SSH public keys auth using the SSH agent or the default
// identity file (~/.ssh/id_rsa, ~/.ssh/id_ed25519, etc.).
func sshAuthFromAgent() (*ssh.PublicKeys, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to determine home directory: %w", err)
	}

	// Try common SSH key file names in preference order.
	keyFiles := []string{
		home + "/.ssh/id_ed25519",
		home + "/.ssh/id_rsa",
		home + "/.ssh/id_ecdsa",
	}

	for _, keyFile := range keyFiles {
		if _, statErr := os.Stat(keyFile); statErr != nil {
			continue
		}

		auth, err := ssh.NewPublicKeysFromFile("git", keyFile, "")
		if err != nil {
			continue
		}

		return auth, nil
	}

	return nil, fmt.Errorf("no SSH key found in ~/.ssh/ (tried id_ed25519, id_rsa, id_ecdsa)")
}

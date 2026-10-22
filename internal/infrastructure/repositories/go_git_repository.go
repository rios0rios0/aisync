package repositories

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	git "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/plumbing/transport/ssh"
	"golang.org/x/crypto/ssh/agent"
)

const (
	gitConfigSectionParts  = 3
	gitConfigKeyValueParts = 2
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

// Init initializes a new Git repository at the given directory and points
// HEAD at refs/heads/main. We pin the branch explicitly (instead of relying
// on go-git's internal default) so aisync's config, the assumed remote
// default, and the fresh local repo all agree from the first commit.
func (r *GoGitRepository) Init(dir string) error {
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		return fmt.Errorf("failed to initialize git repository: %w", err)
	}

	headRef := plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.ReferenceName("refs/heads/main"))
	if err = repo.Storer.SetReference(headRef); err != nil {
		return fmt.Errorf("failed to set HEAD to refs/heads/main: %w", err)
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

// Fetch downloads objects from the remote without merging.
func (r *GoGitRepository) Fetch() error {
	if r.repo == nil {
		return errors.New("repository not opened; call Open or Clone first")
	}

	opts := &git.FetchOptions{
		RemoteName: "origin",
	}

	remote, err := r.repo.Remote("origin")
	if err != nil {
		return fmt.Errorf("no origin remote: %w", err)
	}

	remoteURL := ""
	if len(remote.Config().URLs) > 0 {
		remoteURL = remote.Config().URLs[0]
	}
	if strings.HasPrefix(remoteURL, "git@") {
		auth, authErr := sshAuthFromAgent()
		if authErr != nil {
			return authErr
		}
		opts.Auth = auth
	}

	err = r.repo.Fetch(opts)
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to fetch: %w", err)
	}
	return nil
}

// Pull fetches and merges changes from the remote origin.
func (r *GoGitRepository) Pull() error {
	if r.worktree == nil {
		return errors.New("repository not opened; call Open or Clone first")
	}

	opts := &git.PullOptions{
		RemoteName: "origin",
	}

	if err := r.addSSHAuthIfNeeded(opts); err != nil {
		return err
	}

	err := r.worktree.Pull(opts)
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
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
		return errors.New("repository not opened; call Open or Clone first")
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

// Push pushes committed changes to the remote origin. A failure is returned
// rather than swallowed: the commit stays local and the caller reports that
// the next push retries it, instead of claiming the push succeeded.
func (r *GoGitRepository) Push() error {
	if r.repo == nil {
		return errors.New("repository not opened; call Open or Clone first")
	}

	opts := &git.PushOptions{}

	if err := r.addSSHAuthToPushIfNeeded(opts); err != nil {
		return err
	}

	err := r.repo.Push(opts)
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to push: %w", err)
	}

	return nil
}

// ChangedFiles lists every path with an uncommitted change in the worktree,
// untracked files included.
func (r *GoGitRepository) ChangedFiles() ([]string, error) {
	if r.worktree == nil {
		return nil, errors.New("repository not opened; call Open or Clone first")
	}

	status, err := r.worktree.Status()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree status: %w", err)
	}

	changed := make([]string, 0, len(status))
	for path, fileStatus := range status {
		if fileStatus.Staging != git.Unmodified || fileStatus.Worktree != git.Unmodified {
			changed = append(changed, filepath.ToSlash(path))
		}
	}
	sort.Strings(changed)
	return changed, nil
}

// IsClean returns true if the worktree has no modifications.
func (r *GoGitRepository) IsClean() (bool, error) {
	if r.worktree == nil {
		return false, errors.New("repository not opened; call Open or Clone first")
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
		return nil //nolint:nilerr // no remote configured; skip auth
	}

	if strings.HasPrefix(remoteURL, "git@") {
		sshAuth, sshErr := sshAuthFromAgent()
		if sshErr != nil {
			return fmt.Errorf("failed to configure SSH auth for pull: %w", sshErr)
		}
		opts.Auth = sshAuth
	}

	return nil
}

// addSSHAuthToPushIfNeeded inspects the origin remote URL and configures SSH auth on
// the PushOptions when the URL uses the git@ scheme.
func (r *GoGitRepository) addSSHAuthToPushIfNeeded(opts *git.PushOptions) error {
	remoteURL, err := r.originURL()
	if err != nil {
		return nil //nolint:nilerr // no remote configured; skip auth
	}

	if strings.HasPrefix(remoteURL, "git@") {
		sshAuth, sshErr := sshAuthFromAgent()
		if sshErr != nil {
			return fmt.Errorf("failed to configure SSH auth for push: %w", sshErr)
		}
		opts.Auth = sshAuth
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
		return "", errors.New("origin remote has no URLs")
	}

	return cfg.URLs[0], nil
}

// AddRemote adds a named remote to the repository.
func (r *GoGitRepository) AddRemote(name, url string) error {
	if r.repo == nil {
		return errors.New("repository not opened; call Open or Init first")
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
		return errors.New("repository not opened; call Open or Init first")
	}

	cfg, err := r.repo.Config()
	if err != nil {
		return fmt.Errorf("failed to read git config: %w", err)
	}

	// Parse "section.subsection.key" or "section.key" from dot notation.
	parts := strings.SplitN(key, ".", gitConfigSectionParts)
	switch len(parts) {
	case gitConfigSectionParts:
		cfg.Raw.SetOption(parts[0], parts[1], parts[2], value)
	case gitConfigKeyValueParts:
		cfg.Raw.SetOption(parts[0], "", parts[1], value)
	default:
		return fmt.Errorf("invalid config key: %s", key)
	}

	if err = r.repo.SetConfig(cfg); err != nil {
		return fmt.Errorf("failed to write git config: %w", err)
	}
	return nil
}

// trySSHAgent attempts to connect to the SSH agent via SSH_AUTH_SOCK and
// returns the auth method if successful. The socket path is read from the
// environment, which is trusted since it originates from the user's shell.
func trySSHAgent() (ssh.AuthMethod, bool) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, false
	}
	dialer := &net.Dialer{}

	conn, err := dialer.DialContext(context.Background(), "unix", sock)
	if err != nil {
		return nil, false
	}
	agentClient := agent.NewClient(conn)
	return &ssh.PublicKeysCallback{
		User:     "git",
		Callback: agentClient.Signers,
	}, true
}

// sshAuthFromAgent creates SSH auth using the SSH agent first (via
// SSH_AUTH_SOCK), falling back to common key files when the agent is
// unavailable. This lets passphrase-protected keys work without prompting.
func sshAuthFromAgent() (ssh.AuthMethod, error) {
	if auth, ok := trySSHAgent(); ok {
		return auth, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to determine home directory: %w", err)
	}

	keyFiles := []string{
		home + "/.ssh/id_ed25519",
		home + "/.ssh/id_rsa",
		home + "/.ssh/id_ecdsa",
	}

	for _, keyFile := range keyFiles {
		if _, statErr := os.Stat(keyFile); statErr != nil {
			continue
		}

		auth, keyErr := ssh.NewPublicKeysFromFile("git", keyFile, "")
		if keyErr != nil {
			continue
		}

		return auth, nil
	}

	return nil, errors.New(
		"no SSH agent (SSH_AUTH_SOCK) and no key file found in ~/.ssh/ (tried id_ed25519, id_rsa, id_ecdsa)",
	)
}

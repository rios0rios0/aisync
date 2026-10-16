package repositories

// GitRepository defines the contract for Git operations on the aifiles sync repo.
type GitRepository interface {
	Clone(url, dir, branch string) error
	Init(dir string) error
	Open(dir string) error
	// Fetch downloads objects from the remote without merging, for
	// connectivity checks and branch comparison.
	Fetch() error
	Pull() error
	CommitAll(message string) error
	Push() error
	IsClean() (bool, error)
	// ChangedFiles lists the repo-relative, slash-separated paths of every
	// uncommitted change in the working tree: modified, added, deleted, and
	// untracked files.
	ChangedFiles() ([]string, error)
	HasRemote() bool
	AddRemote(name, url string) error
	SetConfig(key, value string) error
}

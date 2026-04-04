package repositories

// GitRepository defines the contract for Git operations on the aifiles sync repo.
type GitRepository interface {
	Clone(url, dir, branch string) error
	Init(dir string) error
	Open(dir string) error
	Pull() error
	CommitAll(message string) error
	Push() error
	IsClean() (bool, error)
	HasRemote() bool
}

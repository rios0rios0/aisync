package repositories

// DerivedTerm describes a single candidate forbidden term discovered by
// inspecting machine state. The Origin field records WHERE the term was
// extracted from (`git-remote:dev.azure.com`, `gitconfig:user.email`,
// `fs:~/Development/dev.azure.com`, `ssh-config:Host`, etc.) so block
// messages can point the user at the knob that introduced the term.
type DerivedTerm struct {
	Value  string
	Origin string
}

// GitInspector extracts NDA-term candidates from the local machine without
// making network calls. Every method is read-only: the inspector never
// modifies any file, and the same machine state is expected to yield the
// same result across multiple invocations.
type GitInspector interface {
	// EmailDomain returns the domain portion of `git config --global
	// user.email` if (a) the email is parseable and (b) the domain is NOT
	// in the public-free-mail allowlist. Otherwise returns "" and no error.
	EmailDomain() (string, error)

	// SelfIdentities returns the set of owner/user identifiers that the
	// scanner should NOT treat as NDA terms (e.g. the current user's own
	// GitHub login). Used to filter out self-matches from git remotes.
	SelfIdentities() ([]string, error)

	// LocalRemotes walks every directory under the given dev roots up to
	// the configured depth, runs `git remote get-url origin` in each git
	// repo, and returns the parsed candidates. Self identities are NOT
	// filtered here — the caller applies [SelfIdentities] after the fact.
	LocalRemotes(devRoots []string, maxDepth int) ([]DerivedTerm, error)

	// DirectoryLayout enumerates the immediate subdirectories of
	// `<devRoot>/dev.azure.com/`, `<devRoot>/github.com/<non-self>/`,
	// `<devRoot>/gitlab.com/<non-self>/`, `<devRoot>/bitbucket.org/<non-self>/`
	// and returns each directory name as a candidate term tagged with its
	// filesystem origin.
	DirectoryLayout(devRoots []string) ([]DerivedTerm, error)

	// SSHHostAliases parses `~/.ssh/config` and returns `<alias>` segments
	// from `Host <forge>-<alias>` entries (e.g. the `arancia` in
	// `dev.azure.com-arancia`). Returns (nil, nil) if no ssh config exists.
	SSHHostAliases() ([]DerivedTerm, error)
}

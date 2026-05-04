package repositories

// SSHAliasRepository resolves SSH host aliases from the system SSH
// configuration. It enables aisync to retry a failing clone using per-host
// key aliases (e.g. "github.com-mine") when the bare hostname does not work.
type SSHAliasRepository interface {
	// ResolveAliases returns all SSH Host aliases whose HostName matches the
	// given hostname, in the order they appear in ~/.ssh/config. A non-nil
	// error signals that the SSH config could not be read or scanned (so
	// callers can log a warning); a missing config file is not an error and
	// is reported as (nil, nil).
	ResolveAliases(hostname string) ([]string, error)
}

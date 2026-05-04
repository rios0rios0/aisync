package repositories

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// SSHConfigAliasRepository reads ~/.ssh/config and resolves Host aliases for a
// given hostname. For example, if the file contains:
//
//	Host github.com-mine
//	  HostName github.com
//
// then ResolveAliases("github.com") returns ["github.com-mine"].
type SSHConfigAliasRepository struct{}

// NewSSHConfigAliasRepository creates a new SSHConfigAliasRepository.
func NewSSHConfigAliasRepository() *SSHConfigAliasRepository {
	return &SSHConfigAliasRepository{}
}

// ResolveAliases parses ~/.ssh/config and returns all Host entries whose
// HostName directive exactly matches hostname (case-insensitive). Wildcard
// Host patterns (containing * or ?) are skipped since they cannot be used
// directly as clone hostnames.
func (r *SSHConfigAliasRepository) ResolveAliases(hostname string) []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	f, err := os.Open(filepath.Join(home, ".ssh", "config"))
	if err != nil {
		return nil
	}
	defer f.Close()

	var aliases []string
	var currentHosts []string
	var currentHostName string

	flush := func() {
		if len(currentHosts) == 0 {
			return
		}
		if strings.EqualFold(currentHostName, hostname) {
			for _, h := range currentHosts {
				if !strings.ContainsAny(h, "*?") {
					aliases = append(aliases, h)
				}
			}
		}
		currentHosts = nil
		currentHostName = ""
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		key := strings.ToLower(parts[0])
		value := parts[1]

		switch key {
		case "host":
			flush()
			currentHosts = parts[1:]
		case "hostname":
			currentHostName = value
		}
	}
	flush()

	return aliases
}

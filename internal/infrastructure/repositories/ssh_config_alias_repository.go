package repositories

import (
	"bufio"
	"errors"
	"fmt"
	"io"
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
// directly as clone hostnames. A missing ~/.ssh/config file returns
// (nil, nil); other I/O or scan failures return a non-nil error so callers
// can log them instead of silently skipping the SSH alias fallback.
func (r *SSHConfigAliasRepository) ResolveAliases(hostname string) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}

	configPath := filepath.Join(home, ".ssh", "config")
	f, err := os.Open(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open %s: %w", configPath, err)
	}
	defer f.Close()

	aliases, scanErr := scanAliases(f, hostname)
	if scanErr != nil {
		return nil, fmt.Errorf("scan %s: %w", configPath, scanErr)
	}
	return aliases, nil
}

// scanAliases drives the line-by-line state machine over an SSH config stream
// and returns the alias list plus any scanner I/O error.
func scanAliases(r io.Reader, hostname string) ([]string, error) {
	var aliases []string
	var currentHosts []string
	var currentHostName string

	flush := func() {
		if len(currentHosts) > 0 && strings.EqualFold(currentHostName, hostname) {
			for _, h := range currentHosts {
				if !strings.ContainsAny(h, "*?") {
					aliases = append(aliases, h)
				}
			}
		}
		currentHosts = nil
		currentHostName = ""
	}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		key, value, rest := parseConfigLine(scanner.Text())
		switch key {
		case "host":
			flush()
			currentHosts = rest
		case "hostname":
			currentHostName = value
		}
	}
	flush()
	return aliases, scanner.Err()
}

// parseConfigLine returns the lowercased keyword, its first argument, and any
// trailing arguments. Comments and lines without an argument yield an empty
// key so the caller can ignore them.
func parseConfigLine(line string) (string, string, []string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", "", nil
	}
	parts := strings.Fields(trimmed)
	const minTokens = 2
	if len(parts) < minTokens {
		return "", "", nil
	}
	return strings.ToLower(parts[0]), parts[1], parts[1:]
}

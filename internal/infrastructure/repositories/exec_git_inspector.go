package repositories

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// sshRemoteParts is the expected number of segments after `git@` is
// stripped from an SSH git URL: `<host>:<path>`.
const sshRemoteParts = 2

// publicFreeMailDomains holds the email domains we deliberately DO NOT
// treat as employer-domain leaks. Anything outside this set is considered
// a potential NDA candidate when it appears in `git config --global
// user.email`.
var publicFreeMailDomains = map[string]struct{}{ //nolint:gochecknoglobals // static allowlist of public free-mail providers
	"gmail.com":      {},
	"outlook.com":    {},
	"hotmail.com":    {},
	"yahoo.com":      {},
	"ymail.com":      {},
	"icloud.com":     {},
	"me.com":         {},
	"mac.com":        {},
	"protonmail.com": {},
	"proton.me":      {},
	"pm.me":          {},
	"fastmail.com":   {},
	"duck.com":       {},
	"live.com":       {},
	"msn.com":        {},
	"aol.com":        {},
}

// knownPublicForges maps git-URL host prefixes to a normalized name used
// when reporting the origin of a derived term (so `git@github.com:...`
// and `https://github.com/...` both yield origin `github.com`).
var knownPublicForges = map[string]string{ //nolint:gochecknoglobals // static registry of public forge hosts
	"github.com":    "github.com",
	"gitlab.com":    "gitlab.com",
	"bitbucket.org": "bitbucket.org",
}

// ExecGitInspector implements [repositories.GitInspector] by shelling out
// to the system `git` binary and reading a handful of user-level config
// files. Every method is read-only and the inspector never writes to any
// filesystem location outside its cache.
type ExecGitInspector struct {
	gitPath string
}

// NewExecGitInspector locates the system `git` binary and returns an
// inspector ready to extract machine state. Returns an error if `git` is
// not found on PATH.
func NewExecGitInspector() (*ExecGitInspector, error) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, err
	}
	return &ExecGitInspector{gitPath: gitPath}, nil
}

// EmailDomain returns the domain portion of `git config --global
// user.email`. Returns ("", nil) if the email is missing, malformed, or
// in the public free-mail allowlist.
func (g *ExecGitInspector) EmailDomain() (string, error) {
	out, err := g.runGit("", "config", "--global", "user.email")
	if err != nil {
		// Treat missing config as "no domain", not an error — most CI
		// environments and fresh installs don't have user.email set.
		return "", nil //nolint:nilerr // missing config is not an error here
	}
	email := strings.TrimSpace(out)
	at := strings.LastIndex(email, "@")
	if at == -1 || at == len(email)-1 {
		return "", nil
	}
	domain := strings.ToLower(email[at+1:])
	if _, isPublic := publicFreeMailDomains[domain]; isPublic {
		return "", nil
	}
	return domain, nil
}

// SelfIdentities returns the set of owner/user identifiers that should be
// excluded from auto-derivation (so the user's own GitHub login doesn't
// become a forbidden term). Currently pulls from `gh api user` when the
// `gh` CLI is available, and falls back to an empty set otherwise.
func (g *ExecGitInspector) SelfIdentities() ([]string, error) {
	ghPath, err := exec.LookPath("gh")
	if err != nil {
		return nil, nil //nolint:nilerr // gh is optional
	}
	cmd := exec.CommandContext(context.Background(), ghPath, "api", "user", "--jq", ".login")
	out, err := cmd.Output()
	if err != nil {
		return nil, nil //nolint:nilerr // gh may be unauthenticated; not an error
	}
	login := strings.TrimSpace(string(out))
	if login == "" {
		return nil, nil
	}
	return []string{login}, nil
}

// LocalRemotes walks every git repo under the given dev roots up to the
// given depth, runs `git remote get-url origin` for each, parses the
// remote URL, and returns any org/owner candidates. The caller filters
// out self-identities afterwards.
func (g *ExecGitInspector) LocalRemotes(devRoots []string, maxDepth int) ([]repositories.DerivedTerm, error) {
	seen := make(map[string]repositories.DerivedTerm)
	for _, root := range devRoots {
		expanded := expandHome(root)
		if _, err := os.Stat(expanded); os.IsNotExist(err) {
			continue
		}
		g.walkGitRepos(expanded, maxDepth, func(repoDir string) {
			url, err := g.runGit(repoDir, "remote", "get-url", "origin")
			if err != nil {
				return
			}
			for _, term := range parseRemoteURL(strings.TrimSpace(url)) {
				seen[term.Value+"|"+term.Origin] = term
			}
		})
	}
	out := make([]repositories.DerivedTerm, 0, len(seen))
	for _, term := range seen {
		out = append(out, term)
	}
	return out, nil
}

// DirectoryLayout enumerates the immediate subdirectories of
// `<devRoot>/dev.azure.com/`, `<devRoot>/github.com/<non-self>/`,
// `<devRoot>/gitlab.com/<non-self>/`, and `<devRoot>/bitbucket.org/<non-self>/`
// and returns each directory name as a candidate term. Self-filtering is
// not applied here — the caller removes self identities after collecting
// all sources.
func (g *ExecGitInspector) DirectoryLayout(devRoots []string) ([]repositories.DerivedTerm, error) {
	var out []repositories.DerivedTerm
	forges := []string{"dev.azure.com", "github.com", "gitlab.com", "bitbucket.org"}
	for _, root := range devRoots {
		expanded := expandHome(root)
		for _, forge := range forges {
			out = append(out, scanForgeDir(expanded, root, forge)...)
		}
	}
	return out, nil
}

// scanForgeDir enumerates one forge directory under a dev root and
// returns derived terms for each first-level entry. For ADO, it also
// descends one extra level to pick up `<org>/<project>/` layouts.
func scanForgeDir(expandedRoot, displayRoot, forge string) []repositories.DerivedTerm {
	forgeRoot := filepath.Join(expandedRoot, forge)
	entries, err := os.ReadDir(forgeRoot)
	if err != nil {
		return nil
	}
	var out []repositories.DerivedTerm
	origin := "fs:" + filepath.Join(displayRoot, forge)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		out = append(out, repositories.DerivedTerm{Value: entry.Name(), Origin: origin})
		if forge != "dev.azure.com" {
			continue
		}
		out = append(out, scanADOProjects(forgeRoot, displayRoot, forge, entry.Name())...)
	}
	return out
}

// scanADOProjects descends into `<forgeRoot>/<org>/` and returns each
// project subdirectory name as a derived term. Used for ADO layouts
// where the structure is `<dev-root>/dev.azure.com/<org>/<project>/<repo>`.
func scanADOProjects(forgeRoot, displayRoot, forge, org string) []repositories.DerivedTerm {
	projectRoot := filepath.Join(forgeRoot, org)
	projectEntries, err := os.ReadDir(projectRoot)
	if err != nil {
		return nil
	}
	origin := "fs:" + filepath.Join(displayRoot, forge, org)
	out := make([]repositories.DerivedTerm, 0, len(projectEntries))
	for _, project := range projectEntries {
		if !project.IsDir() {
			continue
		}
		out = append(out, repositories.DerivedTerm{
			Value:  project.Name(),
			Origin: origin,
		})
	}
	return out
}

// SSHHostAliases parses `~/.ssh/config` and returns `<alias>` segments
// from `Host <forge>-<alias>` entries. Returns (nil, nil) if no ssh
// config exists.
func (g *ExecGitInspector) SSHHostAliases() ([]repositories.DerivedTerm, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}
	configPath := filepath.Join(home, ".ssh", "config")
	file, err := os.Open(configPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, nil
	}
	defer func() { _ = file.Close() }()

	hostLine := regexp.MustCompile(`^\s*Host\s+(.+)$`)
	aliasRE := regexp.MustCompile(`^(github\.com|gitlab\.com|bitbucket\.org|dev\.azure\.com)-([A-Za-z0-9_-]+)$`)

	var out []repositories.DerivedTerm
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		match := hostLine.FindStringSubmatch(scanner.Text())
		if match == nil {
			continue
		}
		for host := range strings.FieldsSeq(match[1]) {
			aliasMatch := aliasRE.FindStringSubmatch(host)
			if aliasMatch == nil {
				continue
			}
			out = append(out, repositories.DerivedTerm{
				Value:  aliasMatch[2],
				Origin: "ssh-config:Host:" + aliasMatch[1],
			})
		}
	}
	return out, nil
}

// runGit shells out to `git` in the given directory and returns stdout.
// An empty dir means run outside any repo (for global config operations).
func (g *ExecGitInspector) runGit(dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(context.Background(), g.gitPath, args...) //nolint:gosec // gitPath is from exec.LookPath
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// walkGitRepos walks up to maxDepth levels under root, calling onRepo for
// every directory containing a .git entry.
func (g *ExecGitInspector) walkGitRepos(root string, maxDepth int, onRepo func(string)) {
	rootDepth := strings.Count(filepath.Clean(root), string(os.PathSeparator))
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // tolerate unreadable entries
		}
		if !d.IsDir() {
			return nil
		}
		depth := strings.Count(filepath.Clean(path), string(os.PathSeparator)) - rootDepth
		if depth > maxDepth {
			return filepath.SkipDir
		}
		if _, statErr := os.Stat(filepath.Join(path, ".git")); statErr == nil {
			onRepo(path)
			return filepath.SkipDir
		}
		return nil
	})
}

// parseRemoteURL extracts candidate forbidden terms from a git remote URL.
// It handles SSH (`git@host:path` and `git@host-alias:path`), HTTPS
// (`https://host/path`), and ADO (`git@ssh.dev.azure.com:v3/org/project/repo`).
func parseRemoteURL(url string) []repositories.DerivedTerm {
	url = strings.TrimSuffix(url, ".git")
	if strings.HasPrefix(url, "git@") {
		return parseSSHRemote(url)
	}
	if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://") {
		return parseHTTPSRemote(url)
	}
	return nil
}

func parseSSHRemote(url string) []repositories.DerivedTerm {
	// git@<host-alias>:<path>
	parts := strings.SplitN(url[len("git@"):], ":", sshRemoteParts)
	if len(parts) != sshRemoteParts {
		return nil
	}
	hostWithAlias := parts[0]
	pathPart := parts[1]

	// Normalize the host: strip an optional `-<alias>` suffix so the
	// origin tag is the public forge name.
	host := hostWithAlias
	if dashIdx := strings.Index(host, "-"); dashIdx > 0 {
		possible := host[:dashIdx]
		if _, known := knownPublicForges[possible]; known || possible == "ssh.dev.azure.com" {
			host = possible
		}
	}
	return parseRemotePath(host, pathPart)
}

func parseHTTPSRemote(url string) []repositories.DerivedTerm {
	// https://<host>/<path>
	rest := strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://")
	before, after, ok := strings.Cut(rest, "/")
	if !ok {
		return nil
	}
	host := before
	pathPart := after
	return parseRemotePath(host, pathPart)
}

// parseRemotePath splits the path portion of a remote URL for the given
// host and returns candidate terms. Host-specific logic:
//
//   - github.com, gitlab.com, bitbucket.org: `<owner>/<repo>` → owner.
//   - ssh.dev.azure.com or dev.azure.com: `v3/<org>/<project>/<repo>` →
//     both org and project are candidates.
//   - Everything else (self-hosted): host plus the first path segment.
func parseRemotePath(host, path string) []repositories.DerivedTerm {
	segments := strings.Split(path, "/")
	if len(segments) == 0 {
		return nil
	}

	switch host {
	case "github.com", "gitlab.com", "bitbucket.org":
		if segments[0] == "" {
			return nil
		}
		return []repositories.DerivedTerm{
			{Value: segments[0], Origin: "git-remote:" + host},
		}
	case "ssh.dev.azure.com", "dev.azure.com":
		// Expect v3/<org>/<project>/<repo>
		if len(segments) >= 3 && segments[0] == "v3" {
			return []repositories.DerivedTerm{
				{Value: segments[1], Origin: "git-remote:dev.azure.com"},
				{Value: segments[2], Origin: "git-remote:dev.azure.com"},
			}
		}
		return nil
	default:
		// Self-hosted — treat the whole host as a candidate term plus
		// the first path segment (org/user).
		out := []repositories.DerivedTerm{
			{Value: host, Origin: "git-remote:" + host},
		}
		if segments[0] != "" {
			out = append(out, repositories.DerivedTerm{Value: segments[0], Origin: "git-remote:" + host})
		}
		return out
	}
}

// expandHome expands a leading `~/` in a path to the user's home directory.
// Falls back to the original string when home lookup fails.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		logger.Debugf("expandHome: could not resolve home: %v", err)
		return path
	}
	return filepath.Join(home, path[2:])
}

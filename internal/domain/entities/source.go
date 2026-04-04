package entities

import "regexp"

// hexSHAPattern matches a full-length Git SHA (40+ hex characters).
var hexSHAPattern = regexp.MustCompile(`^[0-9a-fA-F]{40,}$`)

// Source represents an external source repository that provides shared rules,
// agents, commands, hooks, and skills.
type Source struct {
	Name     string          `yaml:"name"`
	Repo     string          `yaml:"repo"`
	Branch   string          `yaml:"branch"`
	Ref      string          `yaml:"ref,omitempty"`
	Refresh  string          `yaml:"refresh"`
	Mappings []SourceMapping `yaml:"mappings"`
}

// SourceMapping maps a path in the external source to a path in the sync repo.
type SourceMapping struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
}

// TarballURL returns the GitHub archive URL for this source.
//   - If Ref is empty, the URL points to refs/heads/<Branch> (branch archive).
//   - If Ref is a full SHA (40+ hex chars), the URL uses the SHA directly.
//   - Otherwise Ref is treated as a tag and the URL uses refs/tags/<Ref>.
func (s *Source) TarballURL() string {
	base := "https://github.com/" + s.Repo + "/archive/"

	if s.Ref == "" {
		return base + "refs/heads/" + s.Branch + ".tar.gz"
	}

	if hexSHAPattern.MatchString(s.Ref) {
		return base + s.Ref + ".tar.gz"
	}

	return base + "refs/tags/" + s.Ref + ".tar.gz"
}

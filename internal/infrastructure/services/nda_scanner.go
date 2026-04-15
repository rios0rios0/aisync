package services

import (
	"regexp"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

// heuristicCheck is a compile-time regex the scanner always runs regardless
// of user configuration. Heuristics match content SHAPES that are almost
// always leaks (hardcoded home paths, employer email domains, ADO/GitHub
// org URLs), not specific strings.
type heuristicCheck struct {
	name string
	re   *regexp.Regexp
}

// heuristicChecks holds the compile-time heuristic regexes. Each one is
// tagged with a name that surfaces in block messages so the user can
// identify which shape fired.
var heuristicChecks = []heuristicCheck{ //nolint:gochecknoglobals // compile-time shape checks
	{
		name: "home-path",
		// /home/<user>/Development/<host>/<org>
		re: regexp.MustCompile(`/home/[^/\s]+/Development/[^/\s]+/[A-Za-z0-9_.-]{2,}`),
	},
	{
		name: "wsl-path",
		// /mnt/c/Users/<user>/OneDrive/... or Documents/...
		re: regexp.MustCompile(`/mnt/c/Users/[^/\s]+/(OneDrive|Documents)/[^\s]+`),
	},
	{
		name: "ado-org-url",
		// https://dev.azure.com/<Capitalized-or-Numeric-Org>
		// Allow alphanumeric + dash + underscore; require at least one
		// uppercase letter in the first 12 characters to skip lowercase
		// placeholder words like `<org>`.
		re: regexp.MustCompile(`https?://(?:ssh\.)?dev\.azure\.com/[A-Za-z0-9][A-Za-z0-9_-]*[A-Z][A-Za-z0-9_-]*`),
	},
	{
		name: "ssh-host-alias",
		// git@<host>.<tld>-<alias>:v3/<org>/<...>
		re: regexp.MustCompile(`git@[a-z0-9.-]+\.(com|org|io|net)-[a-z0-9][a-z0-9-]+:`),
	},
}

// HeuristicCount returns the number of compile-time heuristic shape
// checks the scanner runs when heuristics are enabled. Exposed for the
// `aisync nda list` summary so the count never drifts from
// [heuristicChecks].
func HeuristicCount() int {
	return len(heuristicChecks)
}

// ForbiddenTermsScanner implements [repositories.NDAScanner] by combining
// three sources of forbidden terms and running all of them against every
// file passed to Scan.
//
// The three sources:
//
//  1. **Explicit** — loaded from the encrypted `.aisync-forbidden.age` at
//     the repo root via [repositories.ForbiddenTermsRepository]. Shared
//     across devices via git.
//  2. **Auto-derived** — extracted from machine state (git remotes,
//     `~/.gitconfig`, `~/.ssh/config`, dev directory layout) via
//     [AutoDeriver]. Per-device, cached at `~/.cache/aisync/derived-terms.txt`.
//  3. **Heuristics** — compile-time shape checks (home paths, employer
//     email domains, ADO/GitHub org URLs, SSH host aliases) that catch
//     content-shape leaks without knowing specific strings.
//
// Any match from any source produces an [entities.NDAFinding] tagged with
// the source (`user`, `auto-derived:<origin>`, or `heuristic:<name>`) so
// the block message can tell the user which knob fixes each hit.
type ForbiddenTermsScanner struct {
	explicit     []entities.ForbiddenTerm
	autoDerived  []entities.ForbiddenTerm
	heuristicsOn bool
}

// NewForbiddenTermsScanner builds a scanner from a prepared explicit-list
// slice and an auto-derived slice. Callers assemble the slices (for
// example, loading the encrypted file in PushCommand) before constructing
// the scanner. `heuristicsOn` can be flipped via `config.yaml:
// nda.heuristics: false`.
func NewForbiddenTermsScanner(
	explicit []entities.ForbiddenTerm,
	autoDerived []entities.ForbiddenTerm,
	heuristicsOn bool,
) *ForbiddenTermsScanner {
	return &ForbiddenTermsScanner{
		explicit:     explicit,
		autoDerived:  autoDerived,
		heuristicsOn: heuristicsOn,
	}
}

// Scan runs every source against every file and returns all findings.
func (s *ForbiddenTermsScanner) Scan(files map[string][]byte) []entities.NDAFinding {
	var findings []entities.NDAFinding
	explicit := &entities.ForbiddenTerms{Terms: s.explicit}
	derived := &entities.ForbiddenTerms{Terms: s.autoDerived}

	for path, content := range files {
		findings = append(findings, explicit.Match(path, content)...)
		findings = append(findings, derived.Match(path, content)...)
		if s.heuristicsOn {
			findings = append(findings, runHeuristics(path, content)...)
		}
	}
	return findings
}

// runHeuristics applies every compile-time heuristic check to the content,
// producing a finding per hit tagged with `heuristic:<name>`.
func runHeuristics(path string, content []byte) []entities.NDAFinding {
	var findings []entities.NDAFinding
	contentStr := string(content)
	lines := splitLines(contentStr)
	for _, check := range heuristicChecks {
		for lineIdx, line := range lines {
			loc := check.re.FindStringIndex(line)
			if loc == nil {
				continue
			}
			findings = append(findings, entities.NDAFinding{
				Path:    path,
				Line:    lineIdx + 1,
				Term:    line[loc[0]:loc[1]],
				Kind:    "heuristic:" + check.name,
				Snippet: clampLine(line),
			})
		}
	}
	return findings
}

// splitLines splits on "\n" so line numbers reported to the user match
// the line count in their editor.
func splitLines(content string) []string {
	var lines []string
	start := 0
	for i := range len(content) {
		if content[i] == '\n' {
			lines = append(lines, content[start:i])
			start = i + 1
		}
	}
	lines = append(lines, content[start:])
	return lines
}

// clampLine is the heuristic-scanner equivalent of
// entities.clampSnippet (which is unexported). Keeps display lines short.
func clampLine(line string) string {
	const maxLen = 120
	if len(line) <= maxLen {
		return line
	}
	return line[:maxLen] + "..."
}

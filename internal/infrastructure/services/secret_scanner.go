package services

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

type secretPattern struct {
	regex       *regexp.Regexp
	description string
}

var patterns = []secretPattern{ //nolint:gochecknoglobals // compiled regex patterns for secret detection
	{regexp.MustCompile(`AKIA[0-9A-Z]{16}`), "AWS Access Key ID"},
	{regexp.MustCompile(`(?i)aws_secret_access_key\s*[=:]\s*[A-Za-z0-9/+=]{40}`), "AWS Secret Access Key"},
	{regexp.MustCompile(`ghp_[A-Za-z0-9]{36}`), "GitHub Personal Access Token"},
	{regexp.MustCompile(`gho_[A-Za-z0-9]{36}`), "GitHub OAuth Token"},
	{regexp.MustCompile(`ghs_[A-Za-z0-9]{36}`), "GitHub App Token"},
	{regexp.MustCompile(`github_pat_[A-Za-z0-9_]{82}`), "GitHub Fine-Grained PAT"},
	{regexp.MustCompile(`sk-ant-[A-Za-z0-9-_]{90,}`), "Anthropic API Key"},
	{regexp.MustCompile(`sk-[A-Za-z0-9]{48,}`), "OpenAI API Key"},
	{regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`), "Google API Key"},
	{regexp.MustCompile(`-----BEGIN (RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----`), "Private Key"},
	{regexp.MustCompile(`AGE-SECRET-KEY-[A-Z0-9]{59}`), "age Private Key"},
	{regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`), "JWT Token"},
	{regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[=:]\s*['"][^'"]{8,}['"]`), "Hardcoded Password"},
	{regexp.MustCompile(`(?i)connection_?string\s*[=:]\s*['"][^'"]+['"]`), "Connection String"},
	{regexp.MustCompile(`xox[bpras]-[A-Za-z0-9-]{10,}`), "Slack Token"},
}

// allowedMatches maps a pattern description to a check that recognizes
// matches with a known-safe shape, for patterns broad enough to also match
// documentation. A line is only reported when one of its matches fails
// the check.
var allowedMatches = map[string]func(match string) bool{ //nolint:gochecknoglobals // compiled-in allow rules
	"Connection String": isPlaceholderOnly,
}

// placeholderPattern matches variable references: ${NAME}, $NAME, and {{ name }}.
var placeholderPattern = regexp.MustCompile(
	`\$\{[^}]*\}|\$[A-Za-z_][A-Za-z0-9_]*|\{\{[^}]*\}\}`,
)

// isPlaceholderOnly reports whether a matched assignment's quoted value is
// made only of variable references and punctuation, such as
// "${DATABASE_HOST}:${DATABASE_PORT}": it documents where a value comes
// from instead of containing one.
func isPlaceholderOnly(match string) bool {
	start := strings.IndexAny(match, `'"`)
	if start < 0 || len(match) < start+2 {
		return false
	}
	rest := placeholderPattern.ReplaceAllString(match[start+1:len(match)-1], "")
	return !strings.ContainsFunc(rest, func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r)
	})
}

// RegexSecretScanner scans files for common secret patterns using compiled regexes.
type RegexSecretScanner struct{}

// NewRegexSecretScanner creates a new RegexSecretScanner.
func NewRegexSecretScanner() *RegexSecretScanner {
	return &RegexSecretScanner{}
}

// Scan checks all provided files against known secret patterns.
func (s *RegexSecretScanner) Scan(files map[string][]byte) []repositories.SecretFinding {
	var findings []repositories.SecretFinding

	for path, content := range files {
		lines := strings.Split(string(content), "\n")
		for lineNum, line := range lines {
			for _, p := range patterns {
				if matchesSecret(p, line) {
					findings = append(findings, repositories.SecretFinding{
						Path:        path,
						Line:        lineNum + 1,
						Pattern:     p.regex.String(),
						Description: p.description,
					})
				}
			}
		}
	}

	return findings
}

// matchesSecret reports whether line contains a match of p that is not a
// known-safe shape for that pattern.
func matchesSecret(p secretPattern, line string) bool {
	allowed, hasAllowRule := allowedMatches[p.description]
	if !hasAllowRule {
		return p.regex.MatchString(line)
	}
	for _, match := range p.regex.FindAllString(line, -1) {
		if !allowed(match) {
			return true
		}
	}
	return false
}

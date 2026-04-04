package services

import (
	"regexp"
	"strings"

	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

type secretPattern struct {
	regex       *regexp.Regexp
	description string
}

var patterns = []secretPattern{
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
				if p.regex.MatchString(line) {
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

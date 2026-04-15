//go:build unit

package entities_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

func TestCanonicalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "should strip spaces", input: "Contoso Security", expected: "contososecurity"},
		{name: "should strip hyphens", input: "contoso-security", expected: "contososecurity"},
		{name: "should strip underscores", input: "Contoso_Security", expected: "contososecurity"},
		{name: "should strip dots", input: "Contoso.Security", expected: "contososecurity"},
		{name: "should lowercase", input: "ContosoSecurity", expected: "contososecurity"},
		{name: "should uppercase fully", input: "CONTOSO SECURITY", expected: "contososecurity"},
		{name: "should strip NFKD combining marks", input: "Contosó-Sécurity", expected: "contososecurity"},
		{name: "should preserve digits", input: "1021-lab1", expected: "1021lab1"},
		{name: "should strip punctuation", input: "Acme, Corp!", expected: "acmecorp"},
		{name: "should strip whitespace-only input to empty", input: "   \t\n", expected: ""},
		{name: "should strip symbols-only input to empty", input: "!@#$%^&*()", expected: ""},
		{name: "should handle empty", input: "", expected: ""},
		{name: "should handle mixed scripts", input: "日本語test", expected: "日本語test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// given
			input := tt.input

			// when
			got := entities.Canonicalize(input)

			// then
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestForbiddenTerms_Match_Canonical(t *testing.T) {
	t.Parallel()

	term, err := entities.NewCanonicalTerm("ContosoSecurity", "user")
	require.NoError(t, err)
	terms := &entities.ForbiddenTerms{Terms: []entities.ForbiddenTerm{term}}

	// Every writing variant in the table should match the single entry
	// `ContosoSecurity`. This is the guarantee the canonical-form matcher gives
	// so users don't have to enumerate writing variants themselves.
	writingVariants := []string{
		"We deploy to the ContosoSecurity tenant",
		"contososecurity handles this",
		"See Contoso Security for details",
		"contoso-security docs",
		"Under Contoso_Security bucket",
		"CONTOSO SECURITY tenant",
		"Contoso.Security.Api namespace",
		"contosó-sécurity",
	}

	for _, line := range writingVariants {
		t.Run("should match variant "+line, func(t *testing.T) {
			t.Parallel()

			// given
			content := []byte(line)

			// when
			findings := terms.Match("test.md", content)

			// then
			require.Len(t, findings, 1, "expected exactly one finding for %q", line)
			assert.Equal(t, "ContosoSecurity", findings[0].Term)
			assert.Equal(t, "user", findings[0].Kind)
		})
	}
}

func TestForbiddenTerms_Match_Canonical_NoFalsePositives(t *testing.T) {
	t.Parallel()

	term, err := entities.NewCanonicalTerm("ContosoSecurity", "user")
	require.NoError(t, err)
	terms := &entities.ForbiddenTerms{Terms: []entities.ForbiddenTerm{term}}

	cleanLines := []string{
		"just some generic content",
		"Contosoy lemon and Securitybot are unrelated tools",
		"the security team handles audits",
		"zeal and cybersecurity are different topics",
	}

	for _, line := range cleanLines {
		t.Run("should NOT match clean "+line, func(t *testing.T) {
			t.Parallel()

			// given
			content := []byte(line)

			// when
			findings := terms.Match("test.md", content)

			// then
			assert.Empty(t, findings, "expected no findings for %q", line)
		})
	}
}

func TestForbiddenTerms_Match_ReportsLineNumbers(t *testing.T) {
	t.Parallel()

	term, err := entities.NewCanonicalTerm("ContosoSecurity", "user")
	require.NoError(t, err)
	terms := &entities.ForbiddenTerms{Terms: []entities.ForbiddenTerm{term}}

	// given
	content := []byte("line 1\nline 2 mentions ContosoSecurity directly\nline 3\nline 4 talks about Contoso Security too")

	// when
	findings := terms.Match("doc.md", content)

	// then
	require.Len(t, findings, 2)
	assert.Equal(t, 2, findings[0].Line)
	assert.Equal(t, 4, findings[1].Line)
}

func TestForbiddenTerms_Match_Word(t *testing.T) {
	t.Parallel()

	qaTerm, err := entities.NewCanonicalWordTerm("QA", "user")
	require.NoError(t, err)
	terms := &entities.ForbiddenTerms{Terms: []entities.ForbiddenTerm{qaTerm}}

	tests := []struct {
		name    string
		line    string
		matches bool
	}{
		{name: "should match QA as its own word", line: "The QA team reviews PRs", matches: true},
		{name: "should match QA with trailing punctuation", line: "Please loop in QA.", matches: true},
		{name: "should match qa (case insensitive)", line: "talk to qa about it", matches: true},
		{name: "should NOT match QA inside aquarium", line: "visit the aquarium next week", matches: false},
		{name: "should NOT match QA inside equal", line: "the two are equal in importance", matches: false},
		{name: "should NOT match qa inside cliqa", line: "is cliqa a real word?", matches: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// given
			content := []byte(tt.line)

			// when
			findings := terms.Match("t.md", content)

			// then
			if tt.matches {
				assert.NotEmpty(t, findings, "expected a match for %q", tt.line)
			} else {
				assert.Empty(t, findings, "expected no match for %q", tt.line)
			}
		})
	}
}

func TestForbiddenTerms_Match_Regex(t *testing.T) {
	t.Parallel()

	// Family catch for any Contoso-* codename.
	term, err := entities.NewRegexTerm(`\bContoso-[A-Z][A-Za-z0-9]+\b`, "user")
	require.NoError(t, err)
	terms := &entities.ForbiddenTerms{Terms: []entities.ForbiddenTerm{term}}

	tests := []struct {
		name    string
		line    string
		matches bool
	}{
		{name: "should match Contoso-App", line: "See Contoso-App for wiring", matches: true},
		{name: "should match Contoso-Helm", line: "Deploy Contoso-Helm charts", matches: true},
		{name: "should match case-insensitively", line: "see CONTOSO-HELM charts", matches: true},
		{name: "should NOT match plain ContosoSecurity", line: "ContosoSecurity tenant", matches: false},
		{name: "should NOT match Contoso-", line: "Just Contoso- hanging", matches: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// given
			content := []byte(tt.line)

			// when
			findings := terms.Match("t.md", content)

			// then
			if tt.matches {
				assert.NotEmpty(t, findings, "expected a regex match for %q", tt.line)
			} else {
				assert.Empty(t, findings, "expected no regex match for %q", tt.line)
			}
		})
	}
}

func TestNewCanonicalTerm_RejectsEmptyCanonical(t *testing.T) {
	t.Parallel()

	t.Run("should reject all-whitespace term", func(t *testing.T) {
		t.Parallel()

		// given / when
		_, err := entities.NewCanonicalTerm("   \t", "user")

		// then
		require.Error(t, err)
	})

	t.Run("should reject all-symbols term", func(t *testing.T) {
		t.Parallel()

		// given / when
		_, err := entities.NewCanonicalTerm("!@#", "user")

		// then
		require.Error(t, err)
	})
}

func TestNewRegexTerm_RejectsInvalidPattern(t *testing.T) {
	t.Parallel()

	// given / when
	_, err := entities.NewRegexTerm(`[unclosed`, "user")

	// then
	require.Error(t, err)
}

func TestParseForbiddenTermsFile(t *testing.T) {
	t.Parallel()

	t.Run("should parse all three modes plus comments and blanks", func(t *testing.T) {
		t.Parallel()

		// given
		content := []byte(`# === Header ===
ContosoSecurity

# A bare term becomes canonical
Northwind

# word: prefix for short/ambiguous terms
word:QA

# regex: prefix for power users
regex:\bContoso-[A-Z]\w+\b

# Blank lines and trailing whitespace ignored:
   `)

		// when
		terms, err := entities.ParseForbiddenTermsFile(content)

		// then
		require.NoError(t, err)
		require.Len(t, terms, 4)
		// ContosoSecurity + Northwind are canonical mode
		assert.Contains(t, []string{terms[0].Original, terms[1].Original}, "ContosoSecurity")
		assert.Contains(t, []string{terms[0].Original, terms[1].Original}, "Northwind")
		// QA is word mode
		assert.Equal(t, "QA", terms[2].Original)
		// regex term preserves the original (pattern) text
		assert.Equal(t, `\bContoso-[A-Z]\w+\b`, terms[3].Original)
	})

	t.Run("should reject invalid regex with line number in error", func(t *testing.T) {
		t.Parallel()

		// given
		content := []byte("ContosoSecurity\n\nregex:[unclosed\nNorthwind\n")

		// when
		_, err := entities.ParseForbiddenTermsFile(content)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "line 3")
	})

	t.Run("should return empty list for empty file", func(t *testing.T) {
		t.Parallel()

		// given / when
		terms, err := entities.ParseForbiddenTermsFile([]byte(""))

		// then
		require.NoError(t, err)
		assert.Empty(t, terms)
	})

	t.Run("should return empty list for comments-only file", func(t *testing.T) {
		t.Parallel()

		// given / when
		terms, err := entities.ParseForbiddenTermsFile([]byte("# just\n# comments\n\n"))

		// then
		require.NoError(t, err)
		assert.Empty(t, terms)
	})
}

func TestForbiddenTerms_Match_MultipleTermsOneLine(t *testing.T) {
	t.Parallel()

	// given
	contoso, err := entities.NewCanonicalTerm("ContosoSecurity", "user")
	require.NoError(t, err)
	northwind, err := entities.NewCanonicalTerm("Northwind", "user")
	require.NoError(t, err)
	terms := &entities.ForbiddenTerms{Terms: []entities.ForbiddenTerm{contoso, northwind}}
	content := []byte("The Northwind team manages ContosoSecurity tenancy.")

	// when
	findings := terms.Match("doc.md", content)

	// then
	require.Len(t, findings, 2)
	// Both findings should be on line 1
	assert.Equal(t, 1, findings[0].Line)
	assert.Equal(t, 1, findings[1].Line)
}

func TestForbiddenTerms_Match_PreservesKind(t *testing.T) {
	t.Parallel()

	// given
	term, err := entities.NewCanonicalTerm("ContosoSecurity", "auto-derived:git-remote:dev.azure.com")
	require.NoError(t, err)
	terms := &entities.ForbiddenTerms{Terms: []entities.ForbiddenTerm{term}}

	// when
	findings := terms.Match("t.md", []byte("ContosoSecurity leak"))

	// then
	require.Len(t, findings, 1)
	assert.Equal(t, "auto-derived:git-remote:dev.azure.com", findings[0].Kind)
}

func TestForbiddenTerms_Match_EmptyList(t *testing.T) {
	t.Parallel()

	// given
	var terms *entities.ForbiddenTerms

	// when
	findings := terms.Match("t.md", []byte("ContosoSecurity"))

	// then
	assert.Empty(t, findings)
}

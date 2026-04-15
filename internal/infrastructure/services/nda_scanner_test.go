//go:build unit

package services_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/infrastructure/services"
)

func TestForbiddenTermsScanner_Scan(t *testing.T) {
	t.Parallel()

	t.Run("should flag explicit term hit with user kind", func(t *testing.T) {
		t.Parallel()

		// given
		explicit, err := entities.NewCanonicalTerm("ZestSecurity", "user")
		require.NoError(t, err)
		scanner := services.NewForbiddenTermsScanner(
			[]entities.ForbiddenTerm{explicit},
			nil,
			false,
		)

		// when
		findings := scanner.Scan(map[string][]byte{
			"doc.md": []byte("We deploy to ZestSecurity"),
		})

		// then
		require.Len(t, findings, 1)
		assert.Equal(t, "user", findings[0].Kind)
		assert.Equal(t, "ZestSecurity", findings[0].Term)
	})

	t.Run("should flag auto-derived term with origin kind", func(t *testing.T) {
		t.Parallel()

		// given
		derived, err := entities.NewCanonicalTerm("ZestSecurity", "auto-derived:git-remote:dev.azure.com")
		require.NoError(t, err)
		scanner := services.NewForbiddenTermsScanner(
			nil,
			[]entities.ForbiddenTerm{derived},
			false,
		)

		// when
		findings := scanner.Scan(map[string][]byte{
			"doc.md": []byte("See zest-security for details"),
		})

		// then
		require.Len(t, findings, 1)
		assert.Equal(t, "auto-derived:git-remote:dev.azure.com", findings[0].Kind)
	})

	t.Run("should fire home-path heuristic with kind=heuristic:home-path", func(t *testing.T) {
		t.Parallel()

		// given
		scanner := services.NewForbiddenTermsScanner(nil, nil, true)
		content := []byte("Run: /home/alice/Development/dev.azure.com/CorporateOrg/script.sh")

		// when
		findings := scanner.Scan(map[string][]byte{"t.md": content})

		// then
		require.Len(t, findings, 1)
		assert.Equal(t, "heuristic:home-path", findings[0].Kind)
	})

	t.Run("should fire ado-org-url heuristic and report URL without leading boundary char", func(t *testing.T) {
		t.Parallel()

		// given
		scanner := services.NewForbiddenTermsScanner(nil, nil, true)
		content := []byte("Visit https://dev.azure.com/CorporateOrg/Project/_git/repo")

		// when
		findings := scanner.Scan(map[string][]byte{"t.md": content})

		// then
		var hit *entities.NDAFinding
		for i, f := range findings {
			if f.Kind == "heuristic:ado-org-url" {
				hit = &findings[i]
				break
			}
		}
		require.NotNil(t, hit, "expected ado-org-url heuristic to fire")
		// The capture group strips the leading boundary char so the
		// reported Term is the URL itself, not " https://...".
		assert.Equal(t, "https://dev.azure.com/CorporateOrg", hit.Term)
	})

	t.Run("should NOT fire ado-org-url heuristic when dev.azure.com is a substring of an attacker host", func(t *testing.T) {
		t.Parallel()

		// given — the (?:^|[^A-Za-z0-9.]) leading anchor is the part
		// CodeQL recognizes as a real URL boundary. Without it, the
		// pattern would match `dev.azure.com/Foo` as a substring of an
		// attacker-controlled host like `evil.dev.azure.com.example/`.
		scanner := services.NewForbiddenTermsScanner(nil, nil, true)
		content := []byte("Bogus URL: https://attacker.dev.azure.com.example/Foo")

		// when
		findings := scanner.Scan(map[string][]byte{"t.md": content})

		// then
		for _, f := range findings {
			assert.NotEqual(t, "heuristic:ado-org-url", f.Kind,
				"attacker host should not fire ado-org-url heuristic")
		}
	})

	t.Run("should NOT fire ado-org-url heuristic on a contrived scheme prefix", func(t *testing.T) {
		t.Parallel()

		// given — `xhttps://dev.azure.com/CorporateOrg` is a strict
		// boundary-only adversarial case: the inner pattern
		// `https?://(?:ssh\.)?dev\.azure\.com/[A-Za-z0-9][A-Za-z0-9_-]*[A-Z][A-Za-z0-9_-]*`
		// is otherwise valid (the host literal matches, the org
		// segment `CorporateOrg` satisfies the uppercase-letter shape
		// constraint), so the ONLY thing preventing a match is the
		// `(?:^|[^A-Za-z0-9.])` leading boundary: at position 0 the
		// `x` fails `https?://`, and at position 1 the preceding `x`
		// is alphanumeric which fails the `[^A-Za-z0-9.]` boundary.
		// This test locks in the boundary itself — if a future
		// simplification deletes it the attacker-substring test
		// would still pass (its inner literal already blocks the
		// match), but THIS test would fail immediately.
		scanner := services.NewForbiddenTermsScanner(nil, nil, true)
		content := []byte("xhttps://dev.azure.com/CorporateOrg")

		// when
		findings := scanner.Scan(map[string][]byte{"t.md": content})

		// then
		for _, f := range findings {
			assert.NotEqual(t, "heuristic:ado-org-url", f.Kind,
				"contrived scheme prefix `xhttps://` must be rejected by the boundary anchor")
		}
	})

	t.Run("should fire ado-org-url heuristic when the boundary char is punctuation", func(t *testing.T) {
		t.Parallel()

		// given — a URL wrapped in parentheses, a common markdown
		// shape. The `(` before `https` is non-alphanumeric-non-dot
		// so the boundary accepts, and the captured group yields the
		// URL without the `(` prefix.
		scanner := services.NewForbiddenTermsScanner(nil, nil, true)
		content := []byte("see (https://dev.azure.com/CorporateOrg) for details")

		// when
		findings := scanner.Scan(map[string][]byte{"t.md": content})

		// then
		var hit *entities.NDAFinding
		for i, f := range findings {
			if f.Kind == "heuristic:ado-org-url" {
				hit = &findings[i]
				break
			}
		}
		require.NotNil(t, hit, "expected ado-org-url heuristic to fire when URL is wrapped in parens")
		assert.Equal(t, "https://dev.azure.com/CorporateOrg", hit.Term,
			"capture group must strip the leading `(` from the reported Term")
	})

	t.Run("should fire ado-org-url heuristic at start of line", func(t *testing.T) {
		t.Parallel()

		// given — verifies the `^` half of the `(?:^|[^A-Za-z0-9.])`
		// alternation: a URL with no leading char must still fire.
		scanner := services.NewForbiddenTermsScanner(nil, nil, true)
		content := []byte("https://dev.azure.com/CorporateOrg/Project")

		// when
		findings := scanner.Scan(map[string][]byte{"t.md": content})

		// then
		var hit *entities.NDAFinding
		for i, f := range findings {
			if f.Kind == "heuristic:ado-org-url" {
				hit = &findings[i]
				break
			}
		}
		require.NotNil(t, hit, "expected ado-org-url heuristic to fire at start-of-line")
		assert.Equal(t, "https://dev.azure.com/CorporateOrg", hit.Term)
	})

	t.Run("should NOT fire ado-org-url heuristic for placeholder", func(t *testing.T) {
		t.Parallel()

		// given — the placeholder `<org>` is lowercase and bracketed, so the
		// shape regex (requires an uppercase letter somewhere) should not match.
		scanner := services.NewForbiddenTermsScanner(nil, nil, true)
		content := []byte("Example URL: https://dev.azure.com/<org>/<project>/_git/<repo>")

		// when
		findings := scanner.Scan(map[string][]byte{"t.md": content})

		// then
		for _, f := range findings {
			assert.NotEqual(t, "heuristic:ado-org-url", f.Kind,
				"placeholder URL should not fire ado-org-url heuristic")
		}
	})

	t.Run("should fire ssh-host-alias heuristic", func(t *testing.T) {
		t.Parallel()

		// given
		scanner := services.NewForbiddenTermsScanner(nil, nil, true)
		content := []byte("Clone: git@dev.azure.com-arancia:v3/CorpOrg/project/repo")

		// when
		findings := scanner.Scan(map[string][]byte{"t.md": content})

		// then
		found := false
		for _, f := range findings {
			if f.Kind == "heuristic:ssh-host-alias" {
				found = true
				break
			}
		}
		assert.True(t, found, "expected ssh-host-alias heuristic to fire")
	})

	t.Run("should NOT fire heuristics when disabled", func(t *testing.T) {
		t.Parallel()

		// given
		scanner := services.NewForbiddenTermsScanner(nil, nil, false)
		content := []byte("Run: /home/alice/Development/dev.azure.com/CorporateOrg/script.sh")

		// when
		findings := scanner.Scan(map[string][]byte{"t.md": content})

		// then
		assert.Empty(t, findings)
	})

	t.Run("should return empty for clean content", func(t *testing.T) {
		t.Parallel()

		// given
		explicit, err := entities.NewCanonicalTerm("ZestSecurity", "user")
		require.NoError(t, err)
		scanner := services.NewForbiddenTermsScanner(
			[]entities.ForbiddenTerm{explicit},
			nil,
			true,
		)

		// when
		findings := scanner.Scan(map[string][]byte{
			"doc.md": []byte("This is a totally generic markdown file with nothing sensitive."),
		})

		// then
		assert.Empty(t, findings)
	})
}

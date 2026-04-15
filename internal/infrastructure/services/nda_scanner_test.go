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

	t.Run("should fire ado-org-url heuristic", func(t *testing.T) {
		t.Parallel()

		// given
		scanner := services.NewForbiddenTermsScanner(nil, nil, true)
		content := []byte("Visit https://dev.azure.com/CorporateOrg/Project/_git/repo")

		// when
		findings := scanner.Scan(map[string][]byte{"t.md": content})

		// then
		found := false
		for _, f := range findings {
			if f.Kind == "heuristic:ado-org-url" {
				found = true
				break
			}
		}
		assert.True(t, found, "expected ado-org-url heuristic to fire")
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

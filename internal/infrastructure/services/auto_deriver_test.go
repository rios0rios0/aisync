//go:build unit

package services_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
	"github.com/rios0rios0/aisync/internal/infrastructure/services"
	"github.com/rios0rios0/aisync/test/doubles"
)

// newAutoDeriverWithTempCache wires an AutoDeriver against a fresh
// temp-dir cache file so each sub-test starts from a known state. The
// returned AutoDeriver uses a 1h TTL by default (overridable via WithTTL).
func newAutoDeriverWithTempCache(t *testing.T, inspector repositories.GitInspector) *services.AutoDeriver {
	t.Helper()
	tmp := t.TempDir()
	return services.NewAutoDeriver(inspector).
		WithCachePath(filepath.Join(tmp, "derived-terms.txt"))
}

func TestAutoDeriver_DeriveTerms(t *testing.T) {
	t.Parallel()

	t.Run("should aggregate findings from every source", func(t *testing.T) {
		t.Parallel()

		// given
		inspector := &doubles.MockGitInspector{
			EmailDomainVal: "arancia.ca",
			LocalRemotesVal: []repositories.DerivedTerm{
				{Value: "ZestSecurity", Origin: "git-remote:dev.azure.com"},
			},
			DirectoryLayoutVal: []repositories.DerivedTerm{
				{Value: "Zest-App", Origin: "fs:~/Development/dev.azure.com/ZestSecurity"},
			},
			SSHHostAliasesVal: []repositories.DerivedTerm{
				{Value: "arancia", Origin: "ssh-config:Host:dev.azure.com"},
			},
		}
		deriver := newAutoDeriverWithTempCache(t, inspector)

		// when
		terms, err := deriver.DeriveTerms(nil, nil)

		// then
		require.NoError(t, err)
		assert.Len(t, terms, 4)
		assert.Equal(t, 1, inspector.EmailDomainCalls)
		assert.Equal(t, 1, inspector.LocalRemotesCalls)
		assert.Equal(t, 1, inspector.DirectoryLayoutCalls)
		assert.Equal(t, 1, inspector.SSHHostAliasesCalls)
	})

	t.Run("should filter self-identities from derived terms", func(t *testing.T) {
		t.Parallel()

		// given — the user's own GitHub login appears as a remote owner
		// but should NOT be flagged as an NDA term.
		inspector := &doubles.MockGitInspector{
			SelfIdentitiesVal: []string{"rios0rios0"},
			LocalRemotesVal: []repositories.DerivedTerm{
				{Value: "rios0rios0", Origin: "git-remote:github.com"},
				{Value: "ZestSecurity", Origin: "git-remote:dev.azure.com"},
			},
		}
		deriver := newAutoDeriverWithTempCache(t, inspector)

		// when
		terms, err := deriver.DeriveTerms(nil, nil)

		// then
		require.NoError(t, err)
		require.Len(t, terms, 1, "self-identity must be filtered out")
		assert.Equal(t, "ZestSecurity", terms[0].Original)
	})

	t.Run("should apply user-provided excludes via canonical form", func(t *testing.T) {
		t.Parallel()

		// given — `back-end` and `BackEnd` canonicalize to the same form,
		// so the exclude must filter the derived `BackEnd` term.
		inspector := &doubles.MockGitInspector{
			DirectoryLayoutVal: []repositories.DerivedTerm{
				{Value: "BackEnd", Origin: "fs:~/Development/dev.azure.com/Org"},
				{Value: "FrontEnd", Origin: "fs:~/Development/dev.azure.com/Org"},
			},
		}
		deriver := newAutoDeriverWithTempCache(t, inspector)

		// when
		terms, err := deriver.DeriveTerms(nil, []string{"back-end"})

		// then
		require.NoError(t, err)
		require.Len(t, terms, 1)
		assert.Equal(t, "FrontEnd", terms[0].Original)
	})

	t.Run("should tolerate per-source errors and keep going", func(t *testing.T) {
		t.Parallel()

		// given — SSHHostAliases returns an error but the other three
		// sources still produce findings. AutoDeriver must NOT propagate
		// the per-source error to the caller (it's logged at debug).
		inspector := &doubles.MockGitInspector{
			EmailDomainVal:    "arancia.ca",
			SSHHostAliasesErr: errors.New("ssh config not readable"),
			LocalRemotesVal: []repositories.DerivedTerm{
				{Value: "ZestSecurity", Origin: "git-remote:dev.azure.com"},
			},
		}
		deriver := newAutoDeriverWithTempCache(t, inspector)

		// when
		terms, err := deriver.DeriveTerms(nil, nil)

		// then
		require.NoError(t, err)
		assert.Len(t, terms, 2, "the two healthy sources should still produce findings")
		assert.Equal(t, 1, inspector.SSHHostAliasesCalls)
	})

	t.Run("should short-circuit the inspector when cache is fresh", func(t *testing.T) {
		t.Parallel()

		// given — first call populates the cache, second call within
		// TTL must NOT hit the inspector again.
		inspector := &doubles.MockGitInspector{
			LocalRemotesVal: []repositories.DerivedTerm{
				{Value: "ZestSecurity", Origin: "git-remote:dev.azure.com"},
			},
		}
		deriver := newAutoDeriverWithTempCache(t, inspector)

		// when
		_, err1 := deriver.DeriveTerms(nil, nil)
		_, err2 := deriver.DeriveTerms(nil, nil)

		// then
		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.Equal(t, 1, inspector.LocalRemotesCalls,
			"second DeriveTerms call within TTL must read from cache, not re-inspect")
	})

	t.Run("should re-inspect when the cache is stale", func(t *testing.T) {
		t.Parallel()

		// given — a cache file backdated past TTL must be re-derived.
		inspector := &doubles.MockGitInspector{
			LocalRemotesVal: []repositories.DerivedTerm{
				{Value: "ZestSecurity", Origin: "git-remote:dev.azure.com"},
			},
		}
		tmp := t.TempDir()
		cachePath := filepath.Join(tmp, "derived-terms.txt")
		deriver := services.NewAutoDeriver(inspector).
			WithCachePath(cachePath).
			WithTTL(50 * time.Millisecond)

		// First call writes the cache.
		_, err := deriver.DeriveTerms(nil, nil)
		require.NoError(t, err)

		// Backdate the cache mtime so it falls outside the TTL window.
		old := time.Now().Add(-1 * time.Hour)
		require.NoError(t, os.Chtimes(cachePath, old, old))

		// when
		_, err = deriver.DeriveTerms(nil, nil)

		// then
		require.NoError(t, err)
		assert.Equal(t, 2, inspector.LocalRemotesCalls,
			"a stale cache must trigger a re-inspect")
	})

	t.Run("should still apply excludes when reading from cache", func(t *testing.T) {
		t.Parallel()

		// given — first call populates the cache with two terms, then
		// the second call (cache-hit) passes a fresh exclude list. The
		// cache-hit branch must apply the excludes too.
		inspector := &doubles.MockGitInspector{
			DirectoryLayoutVal: []repositories.DerivedTerm{
				{Value: "BackEnd", Origin: "fs:~/Development/dev.azure.com/Org"},
				{Value: "FrontEnd", Origin: "fs:~/Development/dev.azure.com/Org"},
			},
		}
		deriver := newAutoDeriverWithTempCache(t, inspector)

		// First call — no excludes, populates cache with 2 terms.
		warm, err := deriver.DeriveTerms(nil, nil)
		require.NoError(t, err)
		require.Len(t, warm, 2)

		// when — second call hits the cache, but with a new exclude.
		filtered, err := deriver.DeriveTerms(nil, []string{"back-end"})

		// then
		require.NoError(t, err)
		require.Len(t, filtered, 1)
		assert.Equal(t, "FrontEnd", filtered[0].Original)
		assert.Equal(t, 1, inspector.DirectoryLayoutCalls,
			"cache hit must skip the inspector")
	})

	t.Run("should round-trip the cache contents through ForbiddenTerm", func(t *testing.T) {
		t.Parallel()

		// given
		inspector := &doubles.MockGitInspector{
			LocalRemotesVal: []repositories.DerivedTerm{
				{Value: "ZestSecurity", Origin: "git-remote:dev.azure.com"},
			},
		}
		deriver := newAutoDeriverWithTempCache(t, inspector)

		// First call populates the cache.
		_, err := deriver.DeriveTerms(nil, nil)
		require.NoError(t, err)

		// when — second call (cache hit) must return terms with the
		// correct Kind tag and the same canonical form.
		terms, err := deriver.DeriveTerms(nil, nil)

		// then
		require.NoError(t, err)
		require.Len(t, terms, 1)
		assert.Equal(t, "ZestSecurity", terms[0].Original)
		assert.Equal(t, "auto-derived:git-remote:dev.azure.com", terms[0].Kind)
		assert.Equal(t, entities.ForbiddenModeCanonical, terms[0].Mode)
	})

	t.Run("should dedupe across sources via canonical form", func(t *testing.T) {
		t.Parallel()

		// given — the same canonical term appears in two different
		// inspector sources with different Origin tags. The seen[canon]
		// check inside addDerived must collapse them into a single
		// ForbiddenTerm; without it the user would see two findings
		// per matching line, one per source.
		inspector := &doubles.MockGitInspector{
			LocalRemotesVal: []repositories.DerivedTerm{
				{Value: "ZestSecurity", Origin: "git-remote:dev.azure.com"},
			},
			DirectoryLayoutVal: []repositories.DerivedTerm{
				// Same canonical form (`zestsecurity`) as the git remote
				// owner above — dedupe must collapse them.
				{Value: "Zest-Security", Origin: "fs:~/Development/dev.azure.com"},
			},
		}
		deriver := newAutoDeriverWithTempCache(t, inspector)

		// when
		terms, err := deriver.DeriveTerms(nil, nil)

		// then
		require.NoError(t, err)
		require.Len(t, terms, 1, "cross-source canonical-form dedupe must collapse to one entry")
		// The first source seen wins (deterministic ordering depends on
		// runInspector's source iteration), so don't pin the exact
		// Original spelling — just confirm the canonical form is
		// preserved and the entry survived.
		assert.Equal(t, "zestsecurity", entities.Canonicalize(terms[0].Original))
	})
}

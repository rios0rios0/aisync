//go:build unit

package commands_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/domain/commands"
	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/test/doubles"
)

// newNDATestCommand wires NDACommand with the two stub repositories the
// tests need. The heuristicCount is fixed at 4 to match the shipping
// services.HeuristicCount() so the List() summary assertions stay
// independent from the infrastructure layer.
func newNDATestCommand(
	configRepo *doubles.MockConfigRepository,
	forbiddenRepo *doubles.MockForbiddenTermsRepository,
) *commands.NDACommand {
	return commands.NewNDACommand(configRepo, forbiddenRepo, 4)
}

func TestNDACommand_Add(t *testing.T) {
	t.Parallel()

	t.Run("should append a new canonical term and persist", func(t *testing.T) {
		t.Parallel()

		// given
		forbiddenRepo := &doubles.MockForbiddenTermsRepository{}
		cmd := newNDATestCommand(&doubles.MockConfigRepository{}, forbiddenRepo)

		// when
		count, added, err := cmd.Add("/tmp/repo", "ZestSecurity", commands.AddModeCanonical)

		// then
		require.NoError(t, err)
		assert.True(t, added)
		assert.Equal(t, 1, count)
		assert.Equal(t, 1, forbiddenRepo.SaveCalls)
		require.Len(t, forbiddenRepo.SavedTerms, 1)
		assert.Equal(t, "ZestSecurity", forbiddenRepo.SavedTerms[0].Original)
		assert.Equal(t, entities.ForbiddenModeCanonical, forbiddenRepo.SavedTerms[0].Mode)
	})

	t.Run("should route --word flag through canonical-word constructor", func(t *testing.T) {
		t.Parallel()

		// given
		forbiddenRepo := &doubles.MockForbiddenTermsRepository{}
		cmd := newNDATestCommand(&doubles.MockConfigRepository{}, forbiddenRepo)

		// when
		_, added, err := cmd.Add("/tmp/repo", "QA", commands.AddModeWord)

		// then
		require.NoError(t, err)
		assert.True(t, added)
		require.Len(t, forbiddenRepo.SavedTerms, 1)
		assert.Equal(t, entities.ForbiddenModeCanonicalWord, forbiddenRepo.SavedTerms[0].Mode)
	})

	t.Run("should route --regex flag through regex constructor", func(t *testing.T) {
		t.Parallel()

		// given
		forbiddenRepo := &doubles.MockForbiddenTermsRepository{}
		cmd := newNDATestCommand(&doubles.MockConfigRepository{}, forbiddenRepo)

		// when
		_, added, err := cmd.Add("/tmp/repo", `\bZest-[A-Z]\w+`, commands.AddModeRegex)

		// then
		require.NoError(t, err)
		assert.True(t, added)
		require.Len(t, forbiddenRepo.SavedTerms, 1)
		assert.Equal(t, entities.ForbiddenModeRegex, forbiddenRepo.SavedTerms[0].Mode)
	})

	t.Run("should reject an invalid regex without persisting", func(t *testing.T) {
		t.Parallel()

		// given
		forbiddenRepo := &doubles.MockForbiddenTermsRepository{}
		cmd := newNDATestCommand(&doubles.MockConfigRepository{}, forbiddenRepo)

		// when
		_, added, err := cmd.Add("/tmp/repo", `[unclosed`, commands.AddModeRegex)

		// then
		require.Error(t, err)
		assert.False(t, added)
		assert.Equal(t, 0, forbiddenRepo.SaveCalls)
	})

	t.Run("should dedupe a canonical-form duplicate without re-saving", func(t *testing.T) {
		t.Parallel()

		// given
		existing, err := entities.NewCanonicalTerm("ZestSecurity", "user")
		require.NoError(t, err)
		forbiddenRepo := &doubles.MockForbiddenTermsRepository{
			Terms: []entities.ForbiddenTerm{existing},
		}
		cmd := newNDATestCommand(&doubles.MockConfigRepository{}, forbiddenRepo)

		// when
		// "Zest Security" canonicalizes to the same form as the existing entry.
		count, added, err := cmd.Add("/tmp/repo", "Zest Security", commands.AddModeCanonical)

		// then
		require.NoError(t, err)
		assert.False(t, added)
		assert.Equal(t, 1, count)
		assert.Equal(t, 0, forbiddenRepo.SaveCalls)
	})

	t.Run("should reject empty term", func(t *testing.T) {
		t.Parallel()

		// given
		forbiddenRepo := &doubles.MockForbiddenTermsRepository{}
		cmd := newNDATestCommand(&doubles.MockConfigRepository{}, forbiddenRepo)

		// when
		_, added, err := cmd.Add("/tmp/repo", "", commands.AddModeCanonical)

		// then
		require.Error(t, err)
		assert.False(t, added)
		assert.Equal(t, 0, forbiddenRepo.LoadCalls)
		assert.Equal(t, 0, forbiddenRepo.SaveCalls)
	})

	t.Run("should propagate Load errors", func(t *testing.T) {
		t.Parallel()

		// given
		forbiddenRepo := &doubles.MockForbiddenTermsRepository{
			LoadErr: errors.New("decrypt failed"),
		}
		cmd := newNDATestCommand(&doubles.MockConfigRepository{}, forbiddenRepo)

		// when
		_, _, err := cmd.Add("/tmp/repo", "ZestSecurity", commands.AddModeCanonical)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "decrypt failed")
	})
}

func TestNDACommand_Remove(t *testing.T) {
	t.Parallel()

	t.Run("should remove a term that matches by canonical form", func(t *testing.T) {
		t.Parallel()

		// given
		existing, err := entities.NewCanonicalTerm("ZestSecurity", "user")
		require.NoError(t, err)
		forbiddenRepo := &doubles.MockForbiddenTermsRepository{
			Terms: []entities.ForbiddenTerm{existing},
		}
		cmd := newNDATestCommand(&doubles.MockConfigRepository{}, forbiddenRepo)

		// when
		// Lower-case-and-spaced variant must still match for removal.
		count, removed, err := cmd.Remove("/tmp/repo", "zest security")

		// then
		require.NoError(t, err)
		assert.True(t, removed)
		assert.Equal(t, 0, count)
		assert.Equal(t, 1, forbiddenRepo.SaveCalls)
		assert.Empty(t, forbiddenRepo.SavedTerms)
	})

	t.Run("should be a no-op when the term is not present", func(t *testing.T) {
		t.Parallel()

		// given
		existing, err := entities.NewCanonicalTerm("ZestSecurity", "user")
		require.NoError(t, err)
		forbiddenRepo := &doubles.MockForbiddenTermsRepository{
			Terms: []entities.ForbiddenTerm{existing},
		}
		cmd := newNDATestCommand(&doubles.MockConfigRepository{}, forbiddenRepo)

		// when
		count, removed, err := cmd.Remove("/tmp/repo", "Unrelated")

		// then
		require.NoError(t, err)
		assert.False(t, removed)
		assert.Equal(t, 1, count)
		assert.Equal(t, 0, forbiddenRepo.SaveCalls)
	})

	t.Run("should reject empty term", func(t *testing.T) {
		t.Parallel()

		// given
		forbiddenRepo := &doubles.MockForbiddenTermsRepository{}
		cmd := newNDATestCommand(&doubles.MockConfigRepository{}, forbiddenRepo)

		// when
		_, removed, err := cmd.Remove("/tmp/repo", "")

		// then
		require.Error(t, err)
		assert.False(t, removed)
	})
}

func TestNDACommand_List(t *testing.T) {
	t.Parallel()

	t.Run("should report the explicit count and heuristic count when heuristics enabled", func(t *testing.T) {
		t.Parallel()

		// given
		t1, err := entities.NewCanonicalTerm("ZestSecurity", "user")
		require.NoError(t, err)
		t2, err := entities.NewCanonicalWordTerm("QA", "user")
		require.NoError(t, err)
		forbiddenRepo := &doubles.MockForbiddenTermsRepository{
			Terms: []entities.ForbiddenTerm{t1, t2},
		}
		// Empty config → AutoDeriveEnabled() and HeuristicsEnabled() both
		// return true via the pointer-nil-default semantics in NDAConfig.
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{},
		}
		cmd := newNDATestCommand(configRepo, forbiddenRepo)

		// when
		summary, err := cmd.List("/tmp/repo", false)

		// then
		require.NoError(t, err)
		assert.Equal(t, 2, summary.Explicit)
		assert.Equal(t, 4, summary.Heuristics)
		assert.Empty(t, summary.ExplicitAll, "showDetailed=false must hide the term list")
	})

	t.Run("should populate ExplicitAll when showDetailed is true", func(t *testing.T) {
		t.Parallel()

		// given
		term, err := entities.NewCanonicalTerm("ZestSecurity", "user")
		require.NoError(t, err)
		forbiddenRepo := &doubles.MockForbiddenTermsRepository{
			Terms: []entities.ForbiddenTerm{term},
		}
		cmd := newNDATestCommand(&doubles.MockConfigRepository{Config: &entities.Config{}}, forbiddenRepo)

		// when
		summary, err := cmd.List("/tmp/repo", true)

		// then
		require.NoError(t, err)
		require.Len(t, summary.ExplicitAll, 1)
		assert.Equal(t, "ZestSecurity", summary.ExplicitAll[0].Original)
	})

	t.Run("should report zero heuristics when nda.heuristics is false", func(t *testing.T) {
		t.Parallel()

		// given
		off := false
		forbiddenRepo := &doubles.MockForbiddenTermsRepository{}
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				NDA: entities.NDAConfig{Heuristics: &off},
			},
		}
		cmd := newNDATestCommand(configRepo, forbiddenRepo)

		// when
		summary, err := cmd.List("/tmp/repo", false)

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, summary.Heuristics)
	})

	t.Run("should propagate Load errors", func(t *testing.T) {
		t.Parallel()

		// given
		forbiddenRepo := &doubles.MockForbiddenTermsRepository{
			LoadErr: errors.New("decrypt failed"),
		}
		cmd := newNDATestCommand(&doubles.MockConfigRepository{}, forbiddenRepo)

		// when
		_, err := cmd.List("/tmp/repo", false)

		// then
		require.Error(t, err)
	})
}

func TestNDACommand_Ignore(t *testing.T) {
	t.Parallel()

	t.Run("should append the term and save the config", func(t *testing.T) {
		t.Parallel()

		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{},
		}
		cmd := newNDATestCommand(configRepo, &doubles.MockForbiddenTermsRepository{})

		// when
		err := cmd.Ignore("/tmp/repo", "backend")

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, configRepo.SaveCalls)
		require.NotNil(t, configRepo.SavedConfig)
		assert.Equal(t, []string{"backend"}, configRepo.SavedConfig.NDA.AutoDeriveExclude)
	})

	t.Run("should dedupe canonical-form duplicate without saving", func(t *testing.T) {
		t.Parallel()

		// given
		configRepo := &doubles.MockConfigRepository{
			Config: &entities.Config{
				NDA: entities.NDAConfig{
					AutoDeriveExclude: []string{"BackEnd"},
				},
			},
		}
		cmd := newNDATestCommand(configRepo, &doubles.MockForbiddenTermsRepository{})

		// when
		// `back-end` canonicalizes to the same form as the existing
		// `BackEnd`, so the Ignore should be a no-op (no save call).
		err := cmd.Ignore("/tmp/repo", "back-end")

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, configRepo.SaveCalls)
	})

	t.Run("should reject empty term", func(t *testing.T) {
		t.Parallel()

		// given
		configRepo := &doubles.MockConfigRepository{Config: &entities.Config{}}
		cmd := newNDATestCommand(configRepo, &doubles.MockForbiddenTermsRepository{})

		// when
		err := cmd.Ignore("/tmp/repo", "")

		// then
		require.Error(t, err)
		assert.Equal(t, 0, configRepo.LoadCalls)
		assert.Equal(t, 0, configRepo.SaveCalls)
	})

	t.Run("should reject term that canonicalizes to empty", func(t *testing.T) {
		t.Parallel()

		// given
		configRepo := &doubles.MockConfigRepository{Config: &entities.Config{}}
		cmd := newNDATestCommand(configRepo, &doubles.MockForbiddenTermsRepository{})

		// when
		// Pure punctuation canonicalizes to empty.
		err := cmd.Ignore("/tmp/repo", "---")

		// then
		require.Error(t, err)
		assert.Equal(t, 0, configRepo.SaveCalls)
	})

	t.Run("should propagate config Load errors", func(t *testing.T) {
		t.Parallel()

		// given
		configRepo := &doubles.MockConfigRepository{
			LoadErr: errors.New("yaml parse failed"),
		}
		cmd := newNDATestCommand(configRepo, &doubles.MockForbiddenTermsRepository{})

		// when
		err := cmd.Ignore("/tmp/repo", "backend")

		// then
		require.Error(t, err)
		assert.Equal(t, 0, configRepo.SaveCalls)
	})
}

package commands

import (
	"errors"
	"fmt"
	"os"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// NDACommand manages the user's explicit forbidden-terms list (stored as
// `<repo>/.aisync-forbidden.age`) and the `nda.auto_derive_exclude` entry
// in `config.yaml`. It is a lightweight façade over
// [repositories.ForbiddenTermsRepository] and
// [repositories.ConfigRepository] with minimal business logic — mostly
// dedupe, validation, and pretty-printing.
//
// The v1 subcommand surface is deliberately small (Add, Remove, List,
// Ignore). Richer operations (Import, Export, Discover, Check) are
// planned follow-ups and are intentionally omitted here so the initial
// PR stays focused.
type NDACommand struct {
	configRepo     repositories.ConfigRepository
	forbiddenRepo  repositories.ForbiddenTermsRepository
	heuristicCount int
}

// NewNDACommand wires the command to its two repositories. The
// heuristicCount is the number of compile-time heuristic shape checks the
// content scanner runs — injected here so the domain layer doesn't have
// to import the infrastructure services package just to count rules.
func NewNDACommand(
	configRepo repositories.ConfigRepository,
	forbiddenRepo repositories.ForbiddenTermsRepository,
	heuristicCount int,
) *NDACommand {
	return &NDACommand{
		configRepo:     configRepo,
		forbiddenRepo:  forbiddenRepo,
		heuristicCount: heuristicCount,
	}
}

// AddMode selects which matching semantics an added term should use.
type AddMode int

const (
	// AddModeCanonical is the default: canonical-form substring match.
	// Catches writing variants like casing, spacing, hyphens, underscores,
	// and accents without the user typing them all out.
	AddModeCanonical AddMode = iota
	// AddModeWord adds word-boundary enforcement on top of canonical
	// matching. Use for short or ambiguous terms like `QA`.
	AddModeWord
	// AddModeRegex registers a raw Go regex (case-insensitive by default).
	AddModeRegex
)

// Add appends a new term to the encrypted forbidden-terms list. Duplicate
// terms (compared via canonical form) are silently skipped. Returns the
// total count of terms in the list after the add, and a flag indicating
// whether the term was actually added vs. already present.
func (c *NDACommand) Add(repoPath, term string, mode AddMode) (int, bool, error) {
	if term == "" {
		return 0, false, errors.New("nda add: term cannot be empty")
	}

	built, err := buildForbiddenTerm(term, mode)
	if err != nil {
		return 0, false, err
	}

	existing, err := c.forbiddenRepo.Load(repoPath)
	if err != nil {
		return 0, false, fmt.Errorf("nda add: failed to load existing list: %w", err)
	}

	if containsTerm(existing, built) {
		return len(existing), false, nil
	}

	existing = append(existing, built)
	if saveErr := c.forbiddenRepo.Save(repoPath, existing); saveErr != nil {
		return 0, false, fmt.Errorf("nda add: failed to save list: %w", saveErr)
	}
	logger.Infof("nda: added 1 term (%d total)", len(existing))
	return len(existing), true, nil
}

// Remove deletes a term from the encrypted forbidden-terms list. Matching
// is canonical-form so variants of the same term dedupe cleanly on
// removal. Returns the count after removal and whether anything was
// actually removed.
func (c *NDACommand) Remove(repoPath, term string) (int, bool, error) {
	if term == "" {
		return 0, false, errors.New("nda remove: term cannot be empty")
	}

	existing, err := c.forbiddenRepo.Load(repoPath)
	if err != nil {
		return 0, false, fmt.Errorf("nda remove: failed to load existing list: %w", err)
	}

	canonical := entities.Canonicalize(term)
	kept := existing[:0:len(existing)]
	removed := false
	for _, t := range existing {
		if entities.Canonicalize(t.Original) == canonical {
			removed = true
			continue
		}
		kept = append(kept, t)
	}
	if !removed {
		return len(existing), false, nil
	}
	if saveErr := c.forbiddenRepo.Save(repoPath, kept); saveErr != nil {
		return 0, false, fmt.Errorf("nda remove: failed to save list: %w", saveErr)
	}
	logger.Infof("nda: removed a term (%d total)", len(kept))
	return len(kept), true, nil
}

// ListSummary captures the counts of each term source. The count-only
// default exists so accidental terminal scrollback doesn't leak the terms
// themselves.
type ListSummary struct {
	Explicit    int
	AutoDerive  int // populated only when showDetailed is true AND auto-derive is configured
	Heuristics  int // always equals the compile-time count; 0 if heuristics are disabled
	ExplicitAll []entities.ForbiddenTerm
}

// List returns a summary of the current forbidden-term list. When
// showDetailed is false, only the counts are populated. When true, the
// full explicit list is returned as well (but still only the explicit
// one — auto-derive and heuristics are never echoed).
func (c *NDACommand) List(repoPath string, showDetailed bool) (ListSummary, error) {
	terms, err := c.forbiddenRepo.Load(repoPath)
	if err != nil {
		return ListSummary{}, fmt.Errorf("nda list: failed to load list: %w", err)
	}

	summary := ListSummary{
		Explicit: len(terms),
	}
	if showDetailed {
		summary.ExplicitAll = terms
	}

	config, err := c.loadConfig(repoPath)
	if err == nil {
		if config.NDA.HeuristicsEnabled() {
			summary.Heuristics = c.heuristicCount
		}
	}
	return summary, nil
}

// Ignore appends the given term to `nda.auto_derive_exclude` in
// config.yaml so push-time auto-derivation stops emitting findings for
// it on this device. The encrypted forbidden list is untouched — this
// command exists specifically for false positives from auto-derivation.
func (c *NDACommand) Ignore(repoPath, term string) error {
	if term == "" {
		return errors.New("nda ignore: term cannot be empty")
	}

	configPath := repoPath + "/config.yaml"
	config, err := c.configRepo.Load(configPath)
	if err != nil {
		return fmt.Errorf("nda ignore: failed to load config: %w", err)
	}

	canonical := entities.Canonicalize(term)
	if canonical == "" {
		return fmt.Errorf("nda ignore: term %q canonicalizes to empty", term)
	}

	for _, existing := range config.NDA.AutoDeriveExclude {
		if entities.Canonicalize(existing) == canonical {
			logger.Infof("nda: %q already in auto_derive_exclude", term)
			return nil
		}
	}

	config.NDA.AutoDeriveExclude = append(config.NDA.AutoDeriveExclude, term)
	if saveErr := c.configRepo.Save(configPath, config); saveErr != nil {
		return fmt.Errorf("nda ignore: failed to save config: %w", saveErr)
	}
	logger.Infof("nda: added %q to auto_derive_exclude (%d total)", term, len(config.NDA.AutoDeriveExclude))
	return nil
}

// buildForbiddenTerm constructs a term from user input based on the add mode.
func buildForbiddenTerm(term string, mode AddMode) (entities.ForbiddenTerm, error) {
	switch mode {
	case AddModeRegex:
		return entities.NewRegexTerm(term, "user")
	case AddModeWord:
		return entities.NewCanonicalWordTerm(term, "user")
	case AddModeCanonical:
		return entities.NewCanonicalTerm(term, "user")
	default:
		return entities.NewCanonicalTerm(term, "user")
	}
}

// containsTerm reports whether a term with the same canonical form (or,
// for regex terms, the same pattern text) already exists in the list.
func containsTerm(existing []entities.ForbiddenTerm, candidate entities.ForbiddenTerm) bool {
	candCanon := entities.Canonicalize(candidate.Original)
	for _, t := range existing {
		if t.Mode != candidate.Mode {
			continue
		}
		if candidate.Mode == entities.ForbiddenModeRegex {
			if t.Original == candidate.Original {
				return true
			}
			continue
		}
		if entities.Canonicalize(t.Original) == candCanon {
			return true
		}
	}
	return false
}

// loadConfig wraps configRepo.Load with a tolerant fallback: if the
// config file is missing, returns a zero-valued Config so callers can
// still display a sensible result (e.g. List on a fresh repo).
func (c *NDACommand) loadConfig(repoPath string) (*entities.Config, error) {
	configPath := repoPath + "/config.yaml"
	config, err := c.configRepo.Load(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &entities.Config{}, nil
		}
		return nil, err
	}
	return config, nil
}

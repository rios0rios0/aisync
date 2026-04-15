package services

import (
	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// CompositeNDAChecker composes the three NDA term sources (explicit list,
// auto-derivation, compile-time heuristics) behind the single-method
// [repositories.NDAContentChecker] facade. Push uses this checker
// alongside the existing secret scanner in its content-scan pipeline.
type CompositeNDAChecker struct {
	forbiddenRepo repositories.ForbiddenTermsRepository
	autoDeriver   *AutoDeriver
}

// NewCompositeNDAChecker wires an encrypted forbidden-terms repository and
// an auto-deriver into a CompositeNDAChecker. Push is expected to receive
// this via dependency injection in controllers/root.go.
func NewCompositeNDAChecker(
	forbiddenRepo repositories.ForbiddenTermsRepository,
	autoDeriver *AutoDeriver,
) *CompositeNDAChecker {
	return &CompositeNDAChecker{
		forbiddenRepo: forbiddenRepo,
		autoDeriver:   autoDeriver,
	}
}

// Check runs the full NDA pipeline. It never treats a missing forbidden
// file, an empty auto-derive result, or an empty `files` map as an
// error — those are all legitimate zero-finding states.
func (c *CompositeNDAChecker) Check(
	repoPath string,
	config *entities.Config,
	files map[string][]byte,
) ([]entities.NDAFinding, error) {
	explicit, err := c.forbiddenRepo.Load(repoPath)
	if err != nil {
		return nil, err
	}

	var derived []entities.ForbiddenTerm
	if config.NDA.AutoDeriveEnabled() && c.autoDeriver != nil {
		derived, err = c.autoDeriver.DeriveTerms(config.NDA.DevRoots, config.NDA.AutoDeriveExclude)
		if err != nil {
			logger.Warnf("nda: auto-derive failed, continuing without machine-state terms: %v", err)
			derived = nil
		}
	}

	scanner := NewForbiddenTermsScanner(explicit, derived, config.NDA.HeuristicsEnabled())
	return scanner.Scan(files), nil
}

package repositories

import "github.com/rios0rios0/aisync/internal/domain/entities"

// NDAContentChecker is the one-call facade the push command uses to run
// the full NDA detection pipeline (explicit list + auto-derive + compile-time
// heuristics) against a set of already-collected files. Implementations
// live in the infrastructure layer and compose [ForbiddenTermsRepository],
// an auto-derivation service, and a [NDAScanner] behind a single method.
//
// Keeping this as one domain interface prevents the push command from
// importing infrastructure directly while still letting infrastructure
// wire the three sources together per push.
type NDAContentChecker interface {
	// Check runs the full pipeline for the given sync repo and config,
	// returning every finding across every source. Returns an empty slice
	// when the content is clean. Only returns an error when the pipeline
	// itself fails (e.g., the encrypted forbidden file exists but cannot
	// be decrypted). Missing/empty sources are NOT errors.
	Check(
		repoPath string,
		config *entities.Config,
		files map[string][]byte,
	) ([]entities.NDAFinding, error)
}

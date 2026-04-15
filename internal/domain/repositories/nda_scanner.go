package repositories

import "github.com/rios0rios0/aisync/internal/domain/entities"

// NDAScanner checks file content against a combined set of forbidden terms
// sourced from the user's explicit list, per-device auto-derivation, and
// compile-time heuristics. It is separate from [SecretScanner] because the
// threat classes, error messages, and remediation paths differ.
type NDAScanner interface {
	// Scan runs the full NDA detection pipeline (explicit list + auto-derive
	// + heuristics) against the given files and returns every match. Files
	// that are already encrypted (`.age`) or otherwise out of scope must be
	// filtered by the caller before invoking Scan.
	Scan(files map[string][]byte) []entities.NDAFinding
}

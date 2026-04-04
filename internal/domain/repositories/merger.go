package repositories

import "github.com/rios0rios0/aisync/internal/domain/entities"

// Merger defines the contract for merging single-file configs from multiple sources.
type Merger interface {
	// Merge takes content from multiple shared sources (in config order) plus
	// optional personal content, and returns the merged result.
	Merge(sharedSources [][]byte, personal []byte) ([]byte, error)
}

// ExcludeAware is implemented by mergers that support dynamic exclude
// configuration. It allows the caller to set excludes after construction,
// which is necessary because excludes come from config that is loaded at
// pull time, not at application startup.
type ExcludeAware interface {
	SetExcludes(excludes []entities.HooksExcludeEntry)
}

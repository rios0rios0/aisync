package repositories

import "github.com/rios0rios0/aisync/internal/domain/entities"

// FetchResult holds the result of fetching files from an external source.
type FetchResult struct {
	// Files maps relative paths (within the sync repo) to their content.
	Files map[string][]byte
	// ETag is the HTTP ETag from the tarball response for cache validation.
	ETag string
}

// SourceRepository defines the contract for fetching files from external sources.
type SourceRepository interface {
	// Fetch downloads the tarball for the given source, extracts the mapped
	// files, and returns them. It uses the cached ETag to avoid re-downloading
	// unchanged content (returns nil FetchResult when content has not changed).
	Fetch(source *entities.Source, cachedETag string) (*FetchResult, error)
}

package repositories

import "github.com/rios0rios0/aisync/internal/domain/entities"

// CacheHints holds cached HTTP headers for conditional requests.
type CacheHints struct {
	// ETag is the HTTP ETag from a previous response.
	ETag string
	// LastModified is the HTTP Last-Modified value from a previous response.
	LastModified string
}

// FetchResult holds the result of fetching files from an external source.
type FetchResult struct {
	// Files maps relative paths (within the sync repo) to their content.
	Files map[string][]byte
	// ETag is the HTTP ETag from the tarball response for cache validation.
	ETag string
	// LastModified is the HTTP Last-Modified header from the response.
	LastModified string
}

// SourceRepository defines the contract for fetching files from external sources.
type SourceRepository interface {
	// Fetch downloads the tarball for the given source, extracts the mapped
	// files, and returns them. It uses the cached hints to avoid re-downloading
	// unchanged content (returns nil FetchResult when content has not changed).
	Fetch(source *entities.Source, hints CacheHints) (*FetchResult, error)
}

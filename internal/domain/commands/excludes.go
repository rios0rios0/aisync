package commands

import (
	"strings"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// loadExcludes returns the device's exclusions from repo. A nil repo -- a
// command built without one -- excludes nothing.
func loadExcludes(repo repositories.ExcludeRepository) (*entities.ExcludePatterns, error) {
	if repo == nil {
		return &entities.ExcludePatterns{}, nil
	}
	return repo.Load()
}

// dropExcludedShared removes from files, keyed shared/<tool>/<path> as the
// external sources deliver them, every file this device excludes.
func dropExcludedShared[V any](files map[string]V, excludes *entities.ExcludePatterns) {
	if excludes.Len() == 0 {
		return
	}
	for key := range files {
		rest, shared := strings.CutPrefix(key, namespaceShared+"/")
		if !shared {
			continue
		}
		if tool, rel, ok := strings.Cut(rest, "/"); ok && excludes.Matches(tool, rel) {
			delete(files, key)
		}
	}
}

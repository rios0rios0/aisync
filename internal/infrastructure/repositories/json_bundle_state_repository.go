package repositories

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

const (
	bundleStateFileName = "bundle-state.json"
	// bundleStateFileMode keeps the on-disk cache readable only by its
	// owner so a shared-host scenario cannot leak project names.
	bundleStateFileMode = 0o600
)

// JSONBundleStateRepository stores the per-device bundle cache as JSON
// at <basePath>/bundle-state.json. The file is intentionally per-device
// — it remembers what *this* machine last saw — and must never be
// committed to the sync repo.
type JSONBundleStateRepository struct {
	basePath string
}

// NewJSONBundleStateRepository builds a repository rooted at basePath
// (typically ~/.cache/aisync/).
func NewJSONBundleStateRepository(basePath string) *JSONBundleStateRepository {
	return &JSONBundleStateRepository{basePath: basePath}
}

// Load reads the cache. When the file does not yet exist (first-run
// pull, or after a manual cache wipe) it returns an empty state instead
// of an error so callers can treat absence as "nothing previously seen".
func (r *JSONBundleStateRepository) Load() (*entities.BundleState, error) {
	path := filepath.Join(r.basePath, bundleStateFileName)
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		if errors.Is(readErr, os.ErrNotExist) {
			return entities.NewBundleState(), nil
		}
		return nil, fmt.Errorf("read bundle state: %w", readErr)
	}

	state := entities.NewBundleState()
	if len(data) == 0 {
		return state, nil
	}
	if parseErr := json.Unmarshal(data, state); parseErr != nil {
		return nil, fmt.Errorf("parse bundle state: %w", parseErr)
	}
	if state.Bundles == nil {
		state.Bundles = map[string]entities.BundleStateEntry{}
	}
	return state, nil
}

// Save writes the cache atomically (temp file + rename) so a crash
// halfway through cannot leave a truncated cache that would later be
// misread as "all bundles deleted upstream".
func (r *JSONBundleStateRepository) Save(state *entities.BundleState) error {
	if mkErr := os.MkdirAll(r.basePath, 0o700); mkErr != nil {
		return fmt.Errorf("create bundle state dir: %w", mkErr)
	}
	data, marshalErr := json.MarshalIndent(state, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("marshal bundle state: %w", marshalErr)
	}
	path := filepath.Join(r.basePath, bundleStateFileName)
	tmp, createErr := os.CreateTemp(r.basePath, bundleStateFileName+".tmp-*")
	if createErr != nil {
		return fmt.Errorf("create temp bundle state: %w", createErr)
	}
	tmpName := tmp.Name()
	if _, writeErr := tmp.Write(data); writeErr != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write temp bundle state: %w", writeErr)
	}
	if chmodErr := tmp.Chmod(bundleStateFileMode); chmodErr != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("chmod bundle state: %w", chmodErr)
	}
	if closeErr := tmp.Close(); closeErr != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close bundle state: %w", closeErr)
	}
	if renameErr := os.Rename(tmpName, path); renameErr != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename bundle state: %w", renameErr)
	}
	return nil
}

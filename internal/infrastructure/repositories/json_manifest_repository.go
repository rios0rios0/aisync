package repositories

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

const manifestFileName = ".aisync-manifest.json"

// JSONManifestRepository reads and writes manifest files as JSON.
type JSONManifestRepository struct{}

// NewJSONManifestRepository creates a new JSONManifestRepository.
func NewJSONManifestRepository() *JSONManifestRepository {
	return &JSONManifestRepository{}
}

// Load reads and parses a manifest file from the given tool directory.
func (r *JSONManifestRepository) Load(toolDir string) (*entities.Manifest, error) {
	path := filepath.Join(toolDir, manifestFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest entities.Manifest
	if err = json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	return &manifest, nil
}

// Save writes a manifest file to the given tool directory.
func (r *JSONManifestRepository) Save(toolDir string, manifest *entities.Manifest) error {
	path := filepath.Join(toolDir, manifestFileName)
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	if err = os.MkdirAll(toolDir, 0700); err != nil {
		return fmt.Errorf("failed to create tool directory: %w", err)
	}

	if err = os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	return nil
}

// Exists checks if a manifest file exists in the given tool directory.
func (r *JSONManifestRepository) Exists(toolDir string) bool {
	path := filepath.Join(toolDir, manifestFileName)
	_, err := os.Stat(path)
	return err == nil
}

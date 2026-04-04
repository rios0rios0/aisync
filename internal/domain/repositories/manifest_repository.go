package repositories

import "github.com/rios0rios0/aisync/internal/domain/entities"

// ManifestRepository defines the contract for reading and writing manifest files.
type ManifestRepository interface {
	Load(toolDir string) (*entities.Manifest, error)
	Save(toolDir string, manifest *entities.Manifest) error
	Exists(toolDir string) bool
}

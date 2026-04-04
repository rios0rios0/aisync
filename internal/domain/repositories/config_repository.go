package repositories

import "github.com/rios0rios0/aisync/internal/domain/entities"

// ConfigRepository defines the contract for reading and writing the aisync config.
type ConfigRepository interface {
	Load(path string) (*entities.Config, error)
	Save(path string, config *entities.Config) error
	Exists(path string) bool
}

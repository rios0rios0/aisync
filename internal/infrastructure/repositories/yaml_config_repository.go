package repositories

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

// YAMLConfigRepository reads and writes config.yaml files.
type YAMLConfigRepository struct{}

// NewYAMLConfigRepository creates a new YAMLConfigRepository.
func NewYAMLConfigRepository() *YAMLConfigRepository {
	return &YAMLConfigRepository{}
}

// Load reads and parses a config.yaml file.
func (r *YAMLConfigRepository) Load(path string) (*entities.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config entities.Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// Save writes a Config to a YAML file.
func (r *YAMLConfigRepository) Save(path string, config *entities.Config) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Exists checks if a config file exists at the given path.
func (r *YAMLConfigRepository) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

package repositories

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

const stateFileName = "state.json"

// JSONStateRepository reads and writes the sync state as JSON inside .aisync/.
type JSONStateRepository struct{}

// NewJSONStateRepository creates a new JSONStateRepository.
func NewJSONStateRepository() *JSONStateRepository {
	return &JSONStateRepository{}
}

// Load reads and parses the state file from the given repo path.
func (r *JSONStateRepository) Load(repoPath string) (*entities.State, error) {
	path := filepath.Join(repoPath, ".aisync", stateFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var state entities.State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	return &state, nil
}

// Save writes the state to the .aisync/ directory inside the given repo path.
func (r *JSONStateRepository) Save(repoPath string, state *entities.State) error {
	dir := filepath.Join(repoPath, ".aisync")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create .aisync directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	path := filepath.Join(dir, stateFileName)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

// Exists checks if a state file exists in the given repo path.
func (r *JSONStateRepository) Exists(repoPath string) bool {
	path := filepath.Join(repoPath, ".aisync", stateFileName)
	_, err := os.Stat(path)
	return err == nil
}

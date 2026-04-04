package repositories

import "github.com/rios0rios0/aisync/internal/domain/entities"

// ToolDetector defines the contract for detecting which AI tools are installed.
type ToolDetector interface {
	// DetectInstalled checks which tools from the defaults have their config
	// directory present on the filesystem and returns them with Enabled set
	// appropriately.
	DetectInstalled(defaults map[string]entities.Tool) map[string]entities.Tool
}

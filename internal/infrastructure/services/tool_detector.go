package services

import (
	"os"
	"path/filepath"
	"strings"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

// FSToolDetector detects which AI tools are installed by checking for their
// configuration directories on the filesystem.
type FSToolDetector struct{}

// NewFSToolDetector creates a new FSToolDetector.
func NewFSToolDetector() *FSToolDetector {
	return &FSToolDetector{}
}

// DetectInstalled checks which tools have their configuration directory present
// and returns the updated tool map with Enabled set based on detection.
func (d *FSToolDetector) DetectInstalled(defaults map[string]entities.Tool) map[string]entities.Tool {
	result := make(map[string]entities.Tool, len(defaults))

	for name, tool := range defaults {
		expanded := expandHome(tool.Path)
		_, err := os.Stat(expanded)
		installed := err == nil

		result[name] = entities.Tool{
			Path:    tool.Path,
			Enabled: installed,
		}

		if installed {
			logger.Debugf("detected AI tool: %s at %s", name, expanded)
		}
	}

	return result
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	if strings.Contains(path, "%") {
		return os.ExpandEnv(path)
	}
	return path
}
